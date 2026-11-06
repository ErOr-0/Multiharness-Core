package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

const (
	workerCommand  = "__worker"
	requestFile    = "request.json"
	statusFile     = "status.json"
	resultFile     = "result.json"
	eventsFile     = "events.jsonl"
	stderrFile     = "stderr.log"
	workerLogFile  = "worker.log"
	cancelFile     = "cancel"
	heartbeatEvery = time.Second
	lostAfter      = 30 * time.Second
	keepJobs       = 7 * 24 * time.Hour
)

var jobIDPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{6}$`)

// A job is a directory holding the request, a status file the runner refreshes
// every second, the raw harness output and, at the end, the result.
type job struct {
	id       string
	dir      string
	started  time.Time
	detached bool
}

func (j *job) path(name string) string { return filepath.Join(j.dir, name) }

func cmdStart(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	req, err := parseRequest("start", args, stdin)
	if err != nil {
		return usageError(stdout, stderr, err)
	}
	r, err := startJob(req)
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return exitFailed
	}
	return printReport(stdout, r)
}

// startJob creates a job and starts its detached worker. A worker that cannot
// start is reported as a failed job, not an error.
func startJob(req Request) (Report, error) {
	j, err := createJob(req)
	if err != nil {
		return Report{}, err
	}
	j.writeStatus(req, nil)
	if err := j.spawnWorker(); err != nil {
		r := j.final(req, Report{Status: statusFailed, Error: "could not start the background worker: " + err.Error()})
		j.writeResult(r)
		return r, nil
	}
	r, err := readReport(j.path(statusFile))
	if err != nil {
		r = Report{Job: j.id, Harness: req.Harness, Status: statusRunning, StartedAt: j.started, UpdatedAt: j.started}
	}
	return r, nil
}

func cmdRun(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	req, err := parseRequest("run", args, stdin)
	if err != nil {
		return usageError(stdout, stderr, err)
	}
	j, err := createJob(req)
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return exitFailed
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return printReport(stdout, j.runJob(ctx, req))
}

func cmdWait(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	limit := fs.Duration("max", 9*time.Minute, "")
	j, err := jobFromArgs(fs, args)
	if err != nil {
		return usageError(stdout, stderr, err)
	}
	return printReport(stdout, j.wait(context.Background(), max(*limit, 0)))
}

func cmdCancel(args []string, stdout, stderr io.Writer) int {
	j, err := jobFromArgs(flag.NewFlagSet("cancel", flag.ContinueOnError), args)
	if err != nil {
		return usageError(stdout, stderr, err)
	}
	if r, err := readReport(j.path(resultFile)); err == nil {
		return printReport(stdout, r)
	}
	if err := os.WriteFile(j.path(cancelFile), nil, 0o600); err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return exitFailed
	}
	return printReport(stdout, j.wait(context.Background(), 30*time.Second))
}

// cmdWorker runs a job that start created, detached from the caller.
func cmdWorker(args []string, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "harness-bridge: the worker needs a job ID")
		return exitUsage
	}
	j, err := openJob(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return exitFailed
	}
	var req Request
	if err := readJSON(j.path(requestFile), &req); err != nil {
		fmt.Fprintf(stderr, "harness-bridge: read the request: %v\n", err)
		return exitFailed
	}
	j.started, j.detached = time.Now(), true
	if st, err := readReport(j.path(statusFile)); err == nil && !st.StartedAt.IsZero() {
		j.started = st.StartedAt
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if r := j.runJob(ctx, req); r.Status != statusCompleted {
		return exitFailed
	}
	return exitOK
}

func jobFromArgs(fs *flag.FlagSet, args []string) (*job, error) {
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() == 0 {
		return nil, errors.New("missing job ID")
	}
	id := fs.Arg(0)
	if err := fs.Parse(fs.Args()[1:]); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return openJob(id)
}

// runJob executes the request, keeps the status file fresh while it runs,
// stops it when a cancel marker appears and writes the result at the end.
func (j *job) runJob(parent context.Context, req Request) Report {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	t := newTracker(req)
	watching := make(chan struct{})
	go func() {
		defer close(watching)
		j.watch(ctx, cancel, req, t)
	}()
	r := execute(ctx, j, req, t)
	cancel()
	<-watching
	r = j.final(req, r)
	j.writeResult(r)
	return r
}

func (j *job) watch(ctx context.Context, cancel context.CancelFunc, req Request, t *tracker) {
	tick := time.NewTicker(heartbeatEvery)
	defer tick.Stop()
	for {
		j.writeStatus(req, t)
		if _, err := os.Stat(j.path(cancelFile)); err == nil {
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// wait returns the result once it exists, or the latest progress when limit
// passes or ctx ends first. A status file that stops changing means the runner
// died.
func (j *job) wait(ctx context.Context, limit time.Duration) Report {
	deadline := time.Now().Add(limit)
	var last Report
	for {
		if ctx.Err() != nil {
			deadline = time.Now()
		}
		if r, err := readReport(j.path(resultFile)); err == nil {
			return r
		}
		if st, err := readReport(j.path(statusFile)); err == nil {
			last = st
		}
		if !last.UpdatedAt.IsZero() && time.Since(last.UpdatedAt) > lostAfter {
			last.Status = statusLost
			last.Error = "the job stopped without writing a result; see " + j.path(workerLogFile)
			return last
		}
		if !time.Now().Before(deadline) {
			if last.Job == "" {
				last = Report{Job: j.id, Status: statusLost, Error: "the job status cannot be read from " + j.dir}
			}
			return last
		}
		time.Sleep(min(250*time.Millisecond, time.Until(deadline)))
	}
}

func (j *job) final(req Request, r Report) Report {
	r.Job, r.Harness = j.id, req.Harness
	r.StartedAt, r.UpdatedAt = j.started, time.Now()
	r.Log = j.path(eventsFile)
	return r
}

func (j *job) writeStatus(req Request, t *tracker) {
	var r Report
	if t != nil {
		r = t.snapshot()
	}
	r = j.final(req, r)
	r.Status = statusRunning
	// A failed replace (Windows refuses while a reader has the file open) is
	// retried on the next heartbeat.
	_ = writeJSON(j.path(statusFile), r)
}

func (j *job) writeResult(r Report) {
	var err error
	for range 50 {
		if err = writeJSON(j.path(resultFile), r); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "harness-bridge: write the result: %v\n", err)
}

func (j *job) spawnWorker() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(j.path(workerLogFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd, err := startDetached(func() *exec.Cmd {
		cmd := exec.Command(exe, workerCommand, j.id)
		cmd.Stdout, cmd.Stderr = logFile, logFile
		return cmd
	})
	if err != nil {
		return err
	}
	return cmd.Process.Release()
}

func (j *job) appendFile(name string) quietFile {
	f, err := os.OpenFile(j.path(name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return quietFile{}
	}
	return quietFile{f}
}

// quietFile is a best-effort log: a failed write must not stop the harness.
type quietFile struct{ f *os.File }

func (q quietFile) Write(p []byte) (int, error) {
	if q.f != nil {
		_, _ = q.f.Write(p)
	}
	return len(p), nil
}

func (q quietFile) Close() error {
	if q.f == nil {
		return nil
	}
	return q.f.Close()
}

func jobsRoot() (string, error) {
	if home := os.Getenv("HARNESS_BRIDGE_HOME"); home != "" {
		return filepath.Abs(filepath.Join(home, "jobs"))
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("no user cache directory; set HARNESS_BRIDGE_HOME: %w", err)
	}
	return filepath.Join(cache, "harness-bridge", "jobs"), nil
}

func createJob(req Request) (*job, error) {
	root, err := jobsRoot()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	pruneJobs(root)
	var suffix [3]byte
	_, _ = rand.Read(suffix[:])
	now := time.Now()
	j := &job{id: now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(suffix[:]), started: now}
	j.dir = filepath.Join(root, j.id)
	if err := os.Mkdir(j.dir, 0o700); err != nil {
		return nil, err
	}
	if err := writeJSON(j.path(requestFile), req); err != nil {
		return nil, err
	}
	return j, nil
}

func openJob(id string) (*job, error) {
	if !jobIDPattern.MatchString(id) {
		return nil, fmt.Errorf("%q is not a job ID", id)
	}
	root, err := jobsRoot()
	if err != nil {
		return nil, err
	}
	j := &job{id: id, dir: filepath.Join(root, id)}
	if _, err := os.Stat(j.path(requestFile)); err != nil {
		return nil, fmt.Errorf("job %s not found in %s", id, root)
	}
	return j, nil
}

// pruneJobs removes jobs whose files have not changed for keepJobs.
func pruneJobs(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-keepJobs)
	for _, e := range entries {
		if !e.IsDir() || !jobIDPattern.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		var newest time.Time
		for _, name := range []string{resultFile, statusFile, requestFile} {
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		if !newest.IsZero() && newest.Before(cutoff) {
			_ = os.RemoveAll(dir)
		}
	}
}

// writeJSON replaces path in one step so readers never see a partial file.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func readReport(path string) (Report, error) {
	var r Report
	err := readJSON(path, &r)
	return r, err
}
