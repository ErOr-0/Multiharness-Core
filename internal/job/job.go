// Package job runs a request as a job: a folder with the request, a status
// file refreshed while the agent works, the raw output and the final result.
// It implements the start, run, wait and cancel commands and the detached
// worker.
package job

import (
	"bytes"
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

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/jsonfile"
)

const (
	WorkerCommand  = "__worker"
	requestFile    = "request.json"
	StatusFile     = "status.json"
	ResultFile     = "result.json"
	workerLogFile  = "worker.log"
	CancelFile     = "cancel"
	heartbeatEvery = time.Second
	lostAfter      = 30 * time.Second
	// lostConfirm is how long a stale status must stay unchanged, measured
	// with this process's own clock, before wait gives up on it.
	lostConfirm = 5 * time.Second
	keepJobs    = 7 * 24 * time.Hour
)

var IDPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{6}$`)

// A Job is a directory holding the request, a status file the runner refreshes
// every second, the raw harness output and, at the end, the result.
type Job struct {
	ID       string
	dir      string
	started  time.Time
	detached bool
}

func (j *Job) Path(name string) string { return filepath.Join(j.dir, name) }

func CmdStart(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	req, err := agent.ParseRequest("start", args, stdin)
	if err != nil {
		return cli.UsageError(stdout, stderr, err)
	}
	r, err := Start(req)
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return cli.ExitFailed
	}
	return printReport(stdout, r)
}

// Start creates a job and starts its detached worker. A worker that cannot
// start is reported as a failed job, not an error.
func Start(req agent.Request) (agent.Report, error) {
	j, err := createJob(req)
	if err != nil {
		return agent.Report{}, err
	}
	j.writeStatus(req, nil)
	if err := j.spawnWorker(); err != nil {
		r := j.final(req, agent.Report{Status: agent.StatusFailed, Error: "could not start the background worker: " + err.Error()})
		j.writeResult(r)
		return r, nil
	}
	r, err := ReadReport(j.Path(StatusFile))
	if err != nil {
		r = agent.Report{Job: j.ID, Harness: req.Harness, Status: agent.StatusRunning, StartedAt: j.started, UpdatedAt: j.started}
	}
	return r, nil
}

func CmdRun(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	req, err := agent.ParseRequest("run", args, stdin)
	if err != nil {
		return cli.UsageError(stdout, stderr, err)
	}
	j, err := createJob(req)
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return cli.ExitFailed
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return printReport(stdout, j.runJob(ctx, req))
}

func CmdWait(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	limit := fs.Duration("max", 9*time.Minute, "")
	j, err := jobFromArgs(fs, args)
	if err != nil {
		return cli.UsageError(stdout, stderr, err)
	}
	return printReport(stdout, j.Wait(context.Background(), max(*limit, 0)))
}

func CmdCancel(args []string, stdout, stderr io.Writer) int {
	j, err := jobFromArgs(flag.NewFlagSet("cancel", flag.ContinueOnError), args)
	if err != nil {
		return cli.UsageError(stdout, stderr, err)
	}
	if r, err := ReadReport(j.Path(ResultFile)); err == nil {
		return printReport(stdout, r)
	}
	if err := os.WriteFile(j.Path(CancelFile), nil, 0o600); err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return cli.ExitFailed
	}
	return printReport(stdout, j.Wait(context.Background(), 30*time.Second))
}

// CmdWorker runs a job that start created, detached from the caller.
func CmdWorker(args []string, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "harness-bridge: the worker needs a job ID")
		return cli.ExitUsage
	}
	j, err := Open(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return cli.ExitFailed
	}
	var req agent.Request
	if err := jsonfile.Read(j.Path(requestFile), &req); err != nil {
		fmt.Fprintf(stderr, "harness-bridge: read the request: %v\n", err)
		return cli.ExitFailed
	}
	j.started, j.detached = time.Now(), true
	if st, err := ReadReport(j.Path(StatusFile)); err == nil && !st.StartedAt.IsZero() {
		j.started = st.StartedAt
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if r := j.runJob(ctx, req); r.Status != agent.StatusCompleted {
		return cli.ExitFailed
	}
	return cli.ExitOK
}

func jobFromArgs(fs *flag.FlagSet, args []string) (*Job, error) {
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
	return Open(id)
}

// runJob executes the request, keeps the status file fresh while it runs,
// stops it when a cancel marker appears and writes the result at the end.
func (j *Job) runJob(parent context.Context, req agent.Request) agent.Report {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	t := agent.NewTracker(req)
	watching := make(chan struct{})
	stopWatch := make(chan struct{})
	go func() {
		defer close(watching)
		j.watch(ctx, cancel, req, t, stopWatch)
	}()
	r := agent.Execute(ctx, agent.Turn{Dir: j.dir, Detached: j.detached}, req, t)
	cancel()
	// Keep the heartbeat until the result is written: stopping the child and
	// collecting Git evidence can take a while, and a stale status would make
	// wait report the job lost.
	close(stopWatch)
	<-watching
	r = j.final(req, r)
	j.writeResult(r)
	return r
}

func (j *Job) watch(ctx context.Context, cancel context.CancelFunc, req agent.Request, t *agent.Tracker, stop <-chan struct{}) {
	tick := time.NewTicker(heartbeatEvery)
	defer tick.Stop()
	for {
		j.writeStatus(req, t)
		if _, err := os.Stat(j.Path(CancelFile)); err == nil {
			cancel()
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

// Wait returns the result once it exists, or the latest progress when limit
// passes or ctx ends first. A status file that stops changing means the runner
// died — but only once it has stayed unchanged for lostConfirm of this
// process's own time: after a laptop sleeps, the first poll sees a stale wall
// clock timestamp that the worker's next heartbeat fixes.
func (j *Job) Wait(ctx context.Context, limit time.Duration) agent.Report {
	deadline := time.Now().Add(limit)
	var last agent.Report
	var staleSince, staleValue time.Time
	for {
		if ctx.Err() != nil {
			deadline = time.Now()
		}
		if r, err := ReadReport(j.Path(ResultFile)); err == nil {
			return r
		}
		if st, err := ReadReport(j.Path(StatusFile)); err == nil {
			last = st
		}
		if !last.UpdatedAt.IsZero() && time.Since(last.UpdatedAt) > lostAfter {
			// Remember, with the local monotonic clock, when this UpdatedAt
			// was first seen; any change resets the wait.
			if staleSince.IsZero() || !last.UpdatedAt.Equal(staleValue) {
				staleSince, staleValue = time.Now(), last.UpdatedAt
			} else if time.Since(staleSince) > lostConfirm {
				last.Status = agent.StatusLost
				last.Error = "the job stopped without writing a result; see " + j.Path(workerLogFile)
				return last
			}
		} else {
			staleSince, staleValue = time.Time{}, time.Time{}
		}
		if !time.Now().Before(deadline) {
			if last.Job == "" {
				last = agent.Report{Job: j.ID, Status: agent.StatusLost, Error: "the job status cannot be read from " + j.dir}
			}
			return last
		}
		time.Sleep(min(250*time.Millisecond, time.Until(deadline)))
	}
}

func (j *Job) final(req agent.Request, r agent.Report) agent.Report {
	r.Job, r.Harness = j.ID, req.Harness
	r.StartedAt, r.UpdatedAt = j.started, time.Now()
	r.Log = j.Path(agent.EventsFile)
	return r
}

func (j *Job) writeStatus(req agent.Request, t *agent.Tracker) {
	var r agent.Report
	if t != nil {
		r = t.Snapshot()
	}
	r = j.final(req, r)
	r.Status = agent.StatusRunning
	// A failed replace (Windows refuses while a reader has the file open) is
	// retried on the next heartbeat.
	_ = jsonfile.Write(j.Path(StatusFile), r)
}

func (j *Job) writeResult(r agent.Report) {
	var err error
	for range 50 {
		if err = jsonfile.Write(j.Path(ResultFile), r); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "harness-bridge: write the result: %v\n", err)
}

func (j *Job) spawnWorker() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(j.Path(workerLogFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd, err := startDetached(func() *exec.Cmd {
		cmd := exec.Command(exe, WorkerCommand, j.ID)
		cmd.Stdout, cmd.Stderr = logFile, logFile
		return cmd
	})
	if err != nil {
		return err
	}
	return cmd.Process.Release()
}

func Root() (string, error) {
	if home := os.Getenv("HARNESS_BRIDGE_HOME"); home != "" {
		return filepath.Abs(filepath.Join(home, "jobs"))
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("no user cache directory; set HARNESS_BRIDGE_HOME: %w", err)
	}
	return filepath.Join(cache, "harness-bridge", "jobs"), nil
}

func createJob(req agent.Request) (*Job, error) {
	root, err := Root()
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
	j := &Job{ID: now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(suffix[:]), started: now}
	j.dir = filepath.Join(root, j.ID)
	if err := os.Mkdir(j.dir, 0o700); err != nil {
		return nil, err
	}
	if err := jsonfile.Write(j.Path(requestFile), req); err != nil {
		return nil, err
	}
	return j, nil
}

func Open(id string) (*Job, error) {
	if !IDPattern.MatchString(id) {
		return nil, fmt.Errorf("%q is not a job ID", id)
	}
	root, err := Root()
	if err != nil {
		return nil, err
	}
	j := &Job{ID: id, dir: filepath.Join(root, id)}
	if _, err := os.Stat(j.Path(requestFile)); err != nil {
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
		if !e.IsDir() || !IDPattern.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		var newest time.Time
		for _, name := range []string{ResultFile, StatusFile, requestFile} {
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		if !newest.IsZero() && newest.Before(cutoff) {
			_ = os.RemoveAll(dir)
		}
	}
}

func ReadReport(path string) (agent.Report, error) {
	var r agent.Report
	err := jsonfile.Read(path, &r)
	return r, err
}

func printReport(w io.Writer, r agent.Report) int {
	r.Decorate()
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false) // agents read this; keep & < > in commands readable
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		fmt.Fprintf(w, "{\"status\":%q,\"error\":%q}\n", agent.StatusFailed, "cannot encode the report: "+err.Error())
		return cli.ExitFailed
	}
	_, _ = w.Write(out.Bytes())
	if r.Status == agent.StatusRunning || r.Status == agent.StatusCompleted {
		return cli.ExitOK
	}
	return cli.ExitFailed
}
