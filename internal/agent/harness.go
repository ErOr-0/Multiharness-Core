// Package agent knows the agent CLIs harness-bridge can run. Each CLI is one
// adapter: its command line, its event parser, its installer and its sign-in
// check. The package also holds the request, the tracker that folds a CLI's
// event stream into a report, and the code that runs one headless turn.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxRecent   = 8
	maxCommands = 30
	maxFiles    = 300
	maxMessage  = 16 << 10
	maxNote     = 160
	MaxLine     = 8 << 20
	stderrTail  = 4 << 10
	promptFile  = "prompt.txt"
	EventsFile  = "events.jsonl"
	stderrFile  = "stderr.log"
)

// Adapter is everything the bridge knows about one agent CLI: how to run it
// headless, how to read its events and how to install it. Adding a CLI means
// adding one Adapter to All.
type Adapter struct {
	Name  string // value of --to, --planner and --implementer
	Title string // display name
	Bin   string // default executable on PATH
	// Resume reports whether --session can continue an earlier session.
	Resume bool
	// PromptFile makes the CLI read the prompt from a file instead of stdin.
	PromptFile bool
	// Instructions is the project file the CLI reads when it is the planner.
	Instructions string
	Login        string // how to sign in after installing
	// LoginCheck, if set, are arguments that exit 0 only when signed in.
	LoginCheck []string
	// Planner, if set, can connect harness-bridge to this agent as a planner.
	Planner *PlannerHooks
	Docs    string
	Install []Installer
	// Notes are caveats for a planner that delegates to this CLI.
	Notes []string
	args  func(req Request, promptPath string) []string
	parse func(t *Tracker, line []byte)
	// finish, if set, adjusts the outcome from the exit status (-1 when the
	// process did not exit normally).
	finish  func(t *Tracker, code int)
	resolve func(bin string) (string, error) // optional executable lookup
}

var All = []*Adapter{Codex, Claude, Muse, OpenCode, CommandCode}

func Named(name string) *Adapter {
	for _, a := range All {
		if a.Name == name {
			return a
		}
	}
	return nil
}

func Names() string {
	names := make([]string, len(All))
	for i, a := range All {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

// Report is what harness-bridge prints. While a job runs it carries progress
// (recent); once it ends it carries the outcome.
type Report struct {
	Job          string          `json:"job"`
	Harness      string          `json:"harness"`
	Status       string          `json:"status"`
	SessionID    string          `json:"session_id,omitempty"`
	Message      string          `json:"message,omitempty"`
	FilesChanged []string        `json:"files_changed,omitempty"`
	Commands     []CommandRun    `json:"commands,omitempty"`
	Denied       []string        `json:"permission_denied,omitempty"`
	Git          *GitEvidence    `json:"git,omitempty"`
	Error        string          `json:"error,omitempty"`
	Recent       []string        `json:"recent,omitempty"`
	Usage        json.RawMessage `json:"usage,omitempty"`
	CostUSD      float64         `json:"cost_usd,omitempty"`
	StartedAt    time.Time       `json:"started_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	Elapsed      string          `json:"elapsed"`
	Log          string          `json:"log"`
	Next         string          `json:"next,omitempty"`
}

// CommandRun is a shell command the inner agent ran.
type CommandRun struct {
	Command  string `json:"command"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

func (r *Report) Decorate() {
	end := r.UpdatedAt
	if r.Status == StatusRunning {
		end = time.Now()
	}
	if !r.StartedAt.IsZero() {
		r.Elapsed = end.Sub(r.StartedAt).Round(time.Second).String()
	}
	r.Next = NextHint(*r, false)
}

// NextHint says what to do after this report, as a CLI command or, for an MCP
// client, as the tool call to make.
func NextHint(r Report, mcp bool) string {
	a := Named(r.Harness)
	cont := fmt.Sprintf("harness-bridge start --to %s --session %s", r.Harness, r.SessionID)
	if mcp {
		cont = fmt.Sprintf("call delegate with session_id %q", r.SessionID)
	}
	switch {
	case r.Status == StatusRunning && mcp:
		return fmt.Sprintf("call wait with job %q", r.Job)
	case r.Status == StatusRunning:
		return "harness-bridge wait " + r.Job
	case r.Status == StatusLost:
		return ""
	case a == nil || !a.Resume || r.SessionID == "":
		if a != nil && !a.Resume && r.Status != StatusCompleted {
			return a.Title + " cannot continue a session; send fixes as a new job with the full context"
		}
		return ""
	case len(r.Denied) > 0:
		return "allow what was denied, then continue with: " + cont
	}
	return "to send fixes: " + cont
}

// harnessCommand returns the executable and arguments for one headless turn.
func harnessCommand(req Request, promptPath string) (string, []string) {
	a := Named(req.Harness)
	bin := req.Bin
	if bin == "" {
		bin = a.Bin
	}
	return bin, a.args(req, promptPath)
}

// exitGrace is how long a process whose turn has already finished
// successfully gets to exit on its own before it is stopped.
var exitGrace = 30 * time.Second

// cmdShimUnsafe is what cmd.exe treats as a separator, redirect or variable
// in a command line.
const cmdShimUnsafe = "\"&|<>^%!\r\n"

// checkCmdShimArgs refuses values that cmd.exe could split or inject when the
// harness resolves to a .cmd/.bat shim, where Go's argument quoting is not
// safe. Other binaries are unaffected.
func checkCmdShimArgs(bin string, req Request) error {
	if !strings.EqualFold(filepath.Ext(bin), ".cmd") && !strings.EqualFold(filepath.Ext(bin), ".bat") {
		return nil
	}
	for _, v := range req.ExtraArgs {
		if i := strings.IndexAny(v, cmdShimUnsafe); i >= 0 {
			return fmt.Errorf("extra_args value %q can't be passed safely to %s through cmd.exe; it contains %q", v, filepath.Base(bin), string(v[i]))
		}
	}
	for _, v := range req.Allow {
		if i := strings.IndexAny(v, cmdShimUnsafe); i >= 0 {
			return fmt.Errorf("--allow value %q can't be passed safely to %s through cmd.exe; it contains %q", v, filepath.Base(bin), string(v[i]))
		}
	}
	return nil
}

// Execute runs one headless turn of the inner harness and folds its output
// into a report. Cancelling ctx means the caller cancelled the job.
func Execute(ctx context.Context, turn Turn, req Request, t *Tracker) Report {
	a := Named(req.Harness)
	before, beforeErr := gitStatus(ctx, req.Dir)
	fail := func(msg string) Report {
		r := t.Snapshot()
		r.Recent, r.Status, r.Error = nil, StatusFailed, msg
		r.Git = gitEvidence(req.Dir, before, beforeErr)
		return r
	}
	var promptPath string
	if a.PromptFile {
		promptPath = filepath.Join(turn.Dir, promptFile)
		if err := os.WriteFile(promptPath, []byte(req.Prompt), 0o600); err != nil {
			return fail("write the prompt file: " + err.Error())
		}
	}
	name, args := harnessCommand(req, promptPath)
	if path, ok := FindBin(name); ok && req.Bin == "" {
		name = path
	}
	if a.resolve != nil && req.Bin == "" {
		resolved, err := a.resolve(name)
		if err != nil {
			return fail(err.Error())
		}
		name = resolved
	}
	if err := checkCmdShimArgs(name, req); err != nil {
		return fail(err.Error())
	}
	runCtx, stop := context.WithTimeout(ctx, req.Timeout)
	defer stop()
	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Dir = req.Dir
	if !a.PromptFile {
		cmd.Stdin = strings.NewReader(req.Prompt)
	}
	events := appendFile(filepath.Join(turn.Dir, EventsFile))
	defer events.Close()
	errLog := appendFile(filepath.Join(turn.Dir, stderrFile))
	defer errLog.Close()
	tail := &tailBuffer{limit: stderrTail}
	cmd.Stdout = io.MultiWriter(t, events)
	cmd.Stderr = io.MultiWriter(tail, errLog)
	prepareHarness(cmd, turn.Detached)
	cmd.WaitDelay = 10 * time.Second
	// Wait in a goroutine so the turn's final event can end the wait early:
	// a harness whose own children (an MCP server, a background shell) keep
	// running must not hold the job until --timeout.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Run() }()
	var runErr error
	select {
	case runErr = <-exited:
	case <-t.finished:
		select {
		case runErr = <-exited:
		case <-time.After(exitGrace):
			// The turn is done; kill the process tree the usual way.
			stop()
			runErr = <-exited
		}
	}
	t.flush()
	if a.finish != nil && ctx.Err() == nil && runCtx.Err() == nil {
		t.mu.Lock()
		a.finish(t, exitCode(runErr))
		t.mu.Unlock()
	}

	r := t.Snapshot()
	r.Recent = nil
	turnOver := t.turnFinishedSuccessfully()
	switch {
	case turnOver:
		// The turn finished successfully, so a later stop, cancel or timeout
		// only cleaned up the process; report the turn's own outcome.
		r.Status, r.Error = t.outcome()
	case ctx.Err() != nil:
		r.Status, r.Error = StatusCancelled, "cancelled before the inner agent finished"
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		r.Status, r.Error = StatusTimedOut, fmt.Sprintf("stopped after --timeout %s", req.Timeout)
	case errors.Is(runErr, exec.ErrNotFound):
		r.Status = StatusFailed
		r.Error = fmt.Sprintf("%s (%s) is not installed; run `harness-bridge setup --implementer %s`, or pass --bin", a.Title, name, a.Name)
	default:
		r.Status, r.Error = t.outcome()
		if r.Status == StatusFailed && r.Error == "" {
			exitErr := runErr
			if errors.Is(exitErr, exec.ErrWaitDelay) {
				// ErrWaitDelay means the process exited successfully and only
				// its output lingered, so it is not a failure reason.
				exitErr = nil
			}
			r.Error = describeExit(exitErr, tail.String())
		}
	}
	r.Git = gitEvidence(req.Dir, before, beforeErr)
	return r
}

func exitCode(err error) int {
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.Is(err, exec.ErrWaitDelay):
		// The process exited successfully; only a child still held its output
		// when WaitDelay passed.
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	}
	return -1
}

func describeExit(err error, stderr string) string {
	msg := "the inner agent ended without finishing its turn"
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		msg = fmt.Sprintf("the inner agent exited with status %d", exit.ExitCode())
	} else if err != nil {
		msg = err.Error()
	}
	if lines := strings.Split(strings.TrimSpace(stderr), "\n"); lines[0] != "" {
		msg += ": " + strings.Join(lines[max(0, len(lines)-5):], " | ")
	}
	return msg
}

// Tracker folds the harness's event stream into a report. The process output
// copier writes to it while the heartbeat reads snapshots. Adapters' parse
// functions run with mu held.
type Tracker struct {
	mu        sync.Mutex
	adapter   *Adapter
	dir       string
	session   string
	rootRun   string // Muse: the top-level run, to ignore nested ones
	afterTool bool   // OpenCode: the next text starts a new answer
	blocked   bool   // OpenCode: the last step ended on a denied tool call
	message   string
	files     []string
	commands  []CommandRun
	denied    []string
	usage     json.RawMessage
	cost      float64
	recent    []string
	done      bool
	// finished is closed the first time the turn ends successfully, so
	// Execute can stop waiting for a process that lingers after its answer.
	finished chan struct{}
	// pendingEdits remembers a file an edit tool call targets by the call's
	// ID, until its result shows whether the edit actually happened.
	pendingEdits map[string]string
	turnFailed   string
	lastError    string
	// needsPermission marks a failure the CLI itself attributes to a denied
	// action, even when no denial event names it.
	needsPermission bool
	line            []byte
	overflow        bool
}

func NewTracker(req Request) *Tracker {
	return &Tracker{adapter: Named(req.Harness), dir: req.Dir, session: req.Session, finished: make(chan struct{})}
}

// markDone records that the turn ended successfully. Adapters' parse
// functions run with mu held.
func (t *Tracker) markDone() {
	t.done = true
	if t.turnFailed == "" {
		select {
		case <-t.finished:
		default:
			close(t.finished)
		}
	}
}

// turnFinishedSuccessfully reports whether a success event was seen.
func (t *Tracker) turnFinishedSuccessfully() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-t.finished:
		return true
	default:
		return false
	}
}

// deferEdit remembers the file an edit tool call targets until its result
// arrives, so files_changed only lists edits that happened.
func (t *Tracker) deferEdit(id, path string) {
	if id == "" || path == "" {
		return
	}
	if t.pendingEdits == nil {
		t.pendingEdits = map[string]string{}
	}
	t.pendingEdits[id] = path
}

// settleEdit adds a remembered file to files_changed unless its tool result
// was an error.
func (t *Tracker) settleEdit(id string, failed bool) {
	path := t.pendingEdits[id]
	delete(t.pendingEdits, id)
	if !failed {
		t.file(path)
	}
}

// Write receives the harness's stdout: one JSON event per line.
func (t *Tracker) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(p)
	for {
		i := bytes.IndexByte(p, '\n')
		chunk := p
		if i >= 0 {
			chunk = p[:i]
		}
		if !t.overflow && len(t.line)+len(chunk) <= MaxLine {
			t.line = append(t.line, chunk...)
		} else {
			t.overflow = true
		}
		if i < 0 {
			return n, nil
		}
		if !t.overflow {
			t.event(t.line)
		}
		t.line, t.overflow = t.line[:0], false
		p = p[i+1:]
	}
}

func (t *Tracker) flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.overflow {
		t.event(t.line)
	}
	t.line, t.overflow = nil, false
}

func (t *Tracker) event(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return
	}
	t.adapter.parse(t, line)
}

type toolInput struct {
	FilePath     string `json:"file_path"`
	FilePathCC   string `json:"filePath"`
	NotebookPath string `json:"notebook_path"`
	Command      string `json:"command"`
	Pattern      string `json:"pattern"`
	Path         string `json:"path"`
}

func (in toolInput) target() string {
	return FirstNonEmpty(in.Command, in.FilePath, in.FilePathCC, in.NotebookPath, in.Path, in.Pattern)
}

// editTarget returns the file a Claude-style edit tool call would change, or
// "" for any other tool.
func editTarget(name string, input json.RawMessage) string {
	switch strings.ToLower(name) {
	case "edit", "multiedit", "write", "notebookedit":
		var in toolInput
		_ = json.Unmarshal(input, &in)
		return FirstNonEmpty(in.FilePath, in.FilePathCC, in.NotebookPath, in.Path)
	}
	return ""
}

// tool records one tool call reported as a name and its JSON input.
func (t *Tracker) tool(name string, input json.RawMessage) {
	var in toolInput
	_ = json.Unmarshal(input, &in)
	switch strings.ToLower(name) {
	case "edit", "multiedit", "write", "notebookedit", "patch", "apply_patch":
		path := FirstNonEmpty(in.FilePath, in.FilePathCC, in.NotebookPath, in.Path)
		t.file(path)
		t.note(strings.ToLower(name) + " " + t.rel(path))
	case "bash", "shell":
		t.command(in.Command, nil)
	default:
		t.note(name + " " + t.rel(in.target()))
	}
}

func describeTool(name string, input json.RawMessage) string {
	var in toolInput
	_ = json.Unmarshal(input, &in)
	if detail := in.target(); detail != "" {
		return name + "(" + Clip(detail, 200) + ")"
	}
	return name
}

func (t *Tracker) note(s string) {
	if s = Clip(strings.Join(strings.Fields(s), " "), maxNote); s == "" {
		return
	}
	t.recent = append(t.recent, s)
	if len(t.recent) > maxRecent {
		t.recent = t.recent[len(t.recent)-maxRecent:]
	}
}

func (t *Tracker) say(text string) {
	if strings.TrimSpace(text) != "" {
		t.message = text
		t.note("said: " + text)
	}
}

func (t *Tracker) command(command string, exit *int) {
	if command = Clip(strings.TrimSpace(command), 300); command == "" {
		return
	}
	t.commands = append(t.commands, CommandRun{Command: command, ExitCode: exit})
	if len(t.commands) > maxCommands {
		t.commands = t.commands[len(t.commands)-maxCommands:]
	}
	note := "$ " + command
	if exit != nil {
		note += fmt.Sprintf(" (exit %d)", *exit)
	}
	t.note(note)
}

func (t *Tracker) file(path string) {
	if path == "" {
		return
	}
	if rel := t.rel(path); !slices.Contains(t.files, rel) && len(t.files) < maxFiles {
		t.files = append(t.files, rel)
	}
}

// rel shows paths inside the workspace relative to it, with forward slashes.
func (t *Tracker) rel(path string) string {
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	if r, err := filepath.Rel(t.dir, path); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(r)
	}
	return path
}

func (t *Tracker) Snapshot() Report {
	t.mu.Lock()
	defer t.mu.Unlock()
	message := t.message
	if len(message) > maxMessage {
		message = Clip(message, maxMessage) + " [truncated; the full text is in the log]"
	}
	return Report{
		SessionID:    t.session,
		Message:      message,
		FilesChanged: slices.Clone(t.files),
		Commands:     slices.Clone(t.commands),
		Denied:       slices.Clone(t.denied),
		Usage:        slices.Clone(t.usage),
		CostUSD:      t.cost,
		Recent:       slices.Clone(t.recent),
	}
}

// outcome classifies a run that was neither cancelled nor timed out.
func (t *Tracker) outcome() (status, problem string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.needsPermission, len(t.denied) > 0 && (t.turnFailed != "" || !t.done):
		return StatusNeedsPermission, FirstNonEmpty(t.turnFailed, "stopped after an action was denied")
	case t.turnFailed != "":
		return StatusFailed, t.turnFailed
	case t.done:
		return StatusCompleted, ""
	}
	return StatusFailed, t.lastError
}

// textOf reads a field that is either a string or an object with a message.
func textOf(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Message string `json:"message"`
		Data    struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return FirstNonEmpty(o.Message, o.Data.Message)
	}
	return ""
}

func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// Clip shortens s to at most n bytes without splitting a character.
func Clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.limit; over > 0 {
		b.buf = append(b.buf[:0:0], b.buf[over:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

const (
	StatusRunning         = "running"
	StatusCompleted       = "completed"
	StatusFailed          = "failed"
	StatusNeedsPermission = "needs_permission"
	StatusTimedOut        = "timed_out"
	StatusCancelled       = "cancelled"
	StatusLost            = "lost"
)

// Turn is where one headless turn keeps its files, and how it was started.
type Turn struct {
	// Dir is the job's folder: the prompt file, the raw event stream and the
	// raw stderr are written there.
	Dir string
	// Detached reports whether the turn runs in a background worker.
	Detached bool
}

func appendFile(path string) quietFile {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
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
