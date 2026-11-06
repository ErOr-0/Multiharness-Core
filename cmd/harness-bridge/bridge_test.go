package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeEnv makes the test binary act as a harness. The bridge runs it through
// --bin, and runs it again as the detached worker for start.
const fakeEnv = "HARNESS_BRIDGE_FAKE"

// exeSuffix is what fake CLIs need to be found on PATH: .exe on Windows,
// nothing elsewhere (where the test binary's own ".test" must not be copied).
var exeSuffix = map[bool]string{true: ".exe", false: ""}[runtime.GOOS == "windows"]

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == workerCommand {
		os.Exit(cli(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	if mode := os.Getenv(fakeEnv); mode != "" {
		os.Exit(fakeHarness(mode))
	}
	home, err := os.MkdirTemp("", "harness-bridge-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HARNESS_BRIDGE_HOME", home)
	os.Setenv("HARNESS_BRIDGE_CONFIG", filepath.Join(home, "config.json"))
	os.Setenv("HARNESS_BRIDGE_ROUTER", "")
	code := m.Run()
	for range 20 { // detached workers may still be exiting on Windows
		if os.RemoveAll(home) == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	os.Exit(code)
}

func fakeHarness(mode string) int {
	if mode == "cli" {
		return fakeCLI()
	}
	var prompt []byte
	if mode != "npm-installer" { // installers inherit the caller's stdin
		prompt, _ = io.ReadAll(os.Stdin)
	}
	cwd, _ := os.Getwd()
	if delay, err := time.ParseDuration(os.Getenv(fakeEnv + "_DELAY")); err == nil {
		time.Sleep(delay) // lets a manual check outlive the shell call that started it
	}
	emit := func(v any) {
		data, _ := json.Marshal(v)
		fmt.Printf("%s\n", data)
	}
	item := func(kind string, it map[string]any) map[string]any {
		return map[string]any{"type": kind, "item": it}
	}
	switch mode {
	case "codex-ok":
		path := filepath.Join(cwd, "hello.txt")
		_ = os.WriteFile(path, []byte("hi\n"), 0o600)
		fmt.Println("Reading prompt from stdin...") // non-JSON noise is ignored
		emit(map[string]any{"type": "thread.started", "thread_id": "thread-123"})
		emit(map[string]any{"type": "turn.started"})
		emit(item("item.started", map[string]any{"type": "command_execution", "command": "go test ./...", "status": "in_progress"}))
		emit(item("item.completed", map[string]any{"type": "command_execution", "command": "go test ./...", "exit_code": 0, "status": "completed"}))
		emit(item("item.completed", map[string]any{"type": "file_change", "status": "completed", "changes": []any{map[string]any{"path": path, "kind": "add"}}}))
		emit(item("item.completed", map[string]any{"type": "agent_message", "text": "Created hello.txt for: " + strings.TrimSpace(string(prompt))}))
		emit(map[string]any{"type": "turn.completed", "usage": map[string]any{"input_tokens": 10, "output_tokens": 5}})
	case "codex-fail":
		emit(map[string]any{"type": "thread.started", "thread_id": "thread-123"})
		emit(map[string]any{"type": "turn.failed", "error": map[string]any{"message": "model overloaded"}})
		return 1
	case "codex-slow":
		emit(map[string]any{"type": "thread.started", "thread_id": "thread-123"})
		time.Sleep(time.Minute)
	case "codex-crash":
		fmt.Fprintln(os.Stderr, "error: unexpected argument '--bogus'")
		return 2
	case "claude-ok":
		path := filepath.Join(cwd, "notes.md")
		_ = os.WriteFile(path, []byte("x\n"), 0o600)
		emit(map[string]any{"type": "system", "subtype": "init", "session_id": "sess-9"})
		emit(map[string]any{"type": "assistant", "session_id": "sess-9", "message": map[string]any{"content": []any{
			map[string]any{"type": "tool_use", "name": "Write", "input": map[string]any{"file_path": path, "content": "x"}},
		}}})
		emit(map[string]any{"type": "assistant", "session_id": "sess-9", "message": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "Wrote notes.md"},
		}}})
		emit(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": "Wrote notes.md", "session_id": "sess-9",
			"total_cost_usd": 0.01, "permission_denials": []any{map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "rm -rf x"}}}})
	case "claude-denied":
		emit(map[string]any{"type": "system", "subtype": "init", "session_id": "sess-9"})
		emit(map[string]any{"type": "result", "subtype": "error_during_execution", "is_error": true, "session_id": "sess-9",
			"permission_denials": []any{map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "go test ./..."}}}})
		return 1
	case "muse-ok", "muse-fail":
		if i := slices.Index(os.Args, "--prompt-file"); i > 0 {
			prompt, _ = os.ReadFile(os.Args[i+1])
		}
		envelope := func(kind, stream string, payload map[string]any) {
			emit(map[string]any{"schema_version": 1, "payload_type": kind, "payload_schema_version": 1, "stream": map[string]any{"kind": "session", "id": stream}, "payload": payload})
		}
		envelope("run.lifecycle.started", "s-1", map[string]any{"command_id": "root"})
		envelope("run.lifecycle.started", "s-1", map[string]any{"command_id": "nested"})
		envelope("run.terminal.completed", "s-1", map[string]any{"command_id": "nested", "terminal": "completed", "text": "nested answer"})
		if mode == "muse-fail" {
			envelope("run.terminal.failed", "s-1", map[string]any{"command_id": "root", "terminal": "failed", "reason": "quota exhausted"})
			return 1
		}
		envelope("run.terminal.completed", "s-1", map[string]any{"command_id": "root", "terminal": "completed", "text": "Muse read: " + strings.TrimSpace(string(prompt))})
	case "opencode-ok":
		path := filepath.Join(cwd, "app.go")
		_ = os.WriteFile(path, []byte("package app\n"), 0o600)
		ev := func(kind string, part map[string]any) {
			emit(map[string]any{"type": kind, "sessionID": "ses_7", "part": part})
		}
		ev("step_start", map[string]any{})
		ev("text", map[string]any{"text": "Let me look."})
		ev("tool_use", map[string]any{"tool": "edit", "state": map[string]any{"status": "completed", "input": map[string]any{"filePath": path}}})
		ev("tool_use", map[string]any{"tool": "bash", "state": map[string]any{"status": "completed", "input": map[string]any{"command": "go build ./..."}, "metadata": map[string]any{"exit": 0}}})
		ev("step_finish", map[string]any{"reason": "tool-calls", "cost": 0.02})
		ev("step_start", map[string]any{})
		ev("text", map[string]any{"text": "Edited app.go."})
		ev("text", map[string]any{"text": "The build passes."})
		ev("step_finish", map[string]any{"reason": "stop", "cost": 0.01})
	case "opencode-denied":
		emit(map[string]any{"type": "step_start", "sessionID": "ses_7", "part": map[string]any{}})
		emit(map[string]any{"type": "tool_use", "sessionID": "ses_7", "part": map[string]any{"tool": "bash", "state": map[string]any{"status": "error",
			"input": map[string]any{"command": "go test ./..."}, "error": "The user rejected permission to use this specific tool call."}}})
		emit(map[string]any{"type": "step_finish", "sessionID": "ses_7", "part": map[string]any{"reason": "tool-calls"}})
	case "commandcode-ok":
		emit(map[string]any{"type": "event", "event": map[string]any{"type": "tool_start", "toolCallId": "t1", "toolName": "edit_file", "description": "README.md"}})
		emit(map[string]any{"type": "event", "event": map[string]any{"type": "something_new"}})
		emit(map[string]any{"type": "result", "subtype": "success", "sessionId": "cc-42", "finalText": "Updated the README.", "usage": map[string]any{"inputTokens": 3}})
	case "commandcode-yolo":
		emit(map[string]any{"type": "event", "event": map[string]any{"type": "tool_result", "toolName": "edit_file", "description": "README.md", "error": "edit_file requires permissions. Use --yolo"}})
		emit(map[string]any{"type": "result", "subtype": "success", "sessionId": "cc-42", "finalText": "I couldn't edit README.md."})
	case "commandcode-auth":
		emit(map[string]any{"type": "result", "subtype": "error", "error": "invalid token"})
		return 3
	case "commandcode-denied":
		emit(map[string]any{"type": "result", "subtype": "error", "sessionId": "cc-42"})
		return 4
	case "npm-installer":
		// Acts as npm: "installs" codex next to itself.
		self, err := os.Executable()
		if err != nil {
			return 4
		}
		dir := filepath.Dir(self)
		name, body := "codex", "#!/bin/sh\nexit 0\n"
		if runtime.GOOS == "windows" {
			name, body = "codex.bat", "@exit /b 0\r\n"
		}
		_ = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755)
		fmt.Println("added 1 package:", strings.Join(os.Args[1:], " "))
	default:
		fmt.Fprintf(os.Stderr, "unknown fake mode %q\n", mode)
		return 3
	}
	return 0
}

// fakeCLI stands in for an agent CLI's own commands (mcp add, auth status,
// login): it logs "<name> <args>" to HARNESS_BRIDGE_FAKE_LOG and succeeds,
// unless its name is in HARNESS_BRIDGE_FAKE_SIGNED_OUT and it is asked for its
// sign-in status. As npm, it installs itself as codex.
func fakeCLI() int {
	self, err := os.Executable()
	if err != nil {
		return 4
	}
	name := strings.TrimSuffix(filepath.Base(self), exeSuffix)
	if path := os.Getenv(fakeEnv + "_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			fmt.Fprintf(f, "%s %s\n", name, strings.Join(os.Args[1:], " "))
			f.Close()
		}
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(firstNonEmpty(os.Getenv(fakeEnv+"_VERSION"), "1.0.0"))
		return 0
	}
	signedOut := slices.Contains(strings.Split(os.Getenv(fakeEnv+"_SIGNED_OUT"), ","), name)
	for _, a := range adapters {
		if a.Bin == name && signedOut && len(a.LoginCheck) > 0 && slices.Equal(os.Args[1:], a.LoginCheck) {
			return 1
		}
	}
	if name == "npm" {
		data, err := os.ReadFile(self)
		if err != nil {
			return 4
		}
		target := filepath.Join(filepath.Dir(self), "codex"+exeSuffix)
		if err := os.WriteFile(target, data, 0o755); err != nil {
			return 4
		}
		fmt.Println("added 1 package")
	}
	return 0
}

// fakeCLIs puts copies of the test binary, acting as fakeCLI, on PATH under
// the given names, and points the home and app-data folders at an empty
// directory so no real installation is found.
func fakeCLIs(t *testing.T, names ...string) (logPath string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	bin, home := t.TempDir(), t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(bin, name+exeSuffix), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, v := range []string{"HOME", "USERPROFILE", "APPDATA"} {
		t.Setenv(v, home)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv(fakeEnv, "cli")
	logPath = filepath.Join(home, "calls.log")
	t.Setenv(fakeEnv+"_LOG", logPath)
	return logPath
}

// useConfig saves cfg as the setup choices for one test.
func useConfig(t *testing.T, cfg Config) {
	t.Helper()
	path, err := saveConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })
}

func bridge(t *testing.T, args ...string) (Report, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli(args, strings.NewReader(""), &out, &errOut)
	var r Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("harness-bridge %v: exit %d\nstdout: %s\nstderr: %s", args, code, out.String(), errOut.String())
	}
	return r, code
}

func testExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Skipf("git is not available: %v %s", err, out)
	}
	return dir
}

func TestRunCodexReportsChangesAndGitEvidence(t *testing.T) {
	t.Setenv(fakeEnv, "codex-ok")
	dir := gitRepo(t)
	r, code := bridge(t, "run", "--to", "codex", "--bin", testExe(t), "--dir", dir, "--prompt", "add hello")
	if code != exitOK || r.Status != statusCompleted {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	if r.SessionID != "thread-123" || r.Message != "Created hello.txt for: add hello" {
		t.Fatalf("session %q, message %q", r.SessionID, r.Message)
	}
	if !slices.Equal(r.FilesChanged, []string{"hello.txt"}) {
		t.Fatalf("files_changed %v", r.FilesChanged)
	}
	if len(r.Commands) != 1 || r.Commands[0].ExitCode == nil || *r.Commands[0].ExitCode != 0 {
		t.Fatalf("commands %+v", r.Commands)
	}
	if r.Git == nil || !slices.Contains(r.Git.Status, "?? hello.txt") || r.Git.DirtyBefore != 0 {
		t.Fatalf("git %+v", r.Git)
	}
	if !strings.Contains(r.Next, "--session thread-123") || len(r.Usage) == 0 || r.Recent != nil {
		t.Fatalf("next %q, usage %s, recent %v", r.Next, r.Usage, r.Recent)
	}
	if again, _ := bridge(t, "wait", r.Job, "--max", "0"); again.Status != statusCompleted || again.SessionID != r.SessionID {
		t.Fatalf("a finished job reads back as %+v", again)
	}
}

func TestRunClaudeReportsDenials(t *testing.T) {
	t.Setenv(fakeEnv, "claude-ok")
	r, code := bridge(t, "run", "--to", "claude", "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "write notes")
	if code != exitOK || r.Status != statusCompleted || r.SessionID != "sess-9" || r.Message != "Wrote notes.md" {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	if !slices.Equal(r.FilesChanged, []string{"notes.md"}) || !slices.Equal(r.Denied, []string{"Bash(rm -rf x)"}) || r.CostUSD != 0.01 {
		t.Fatalf("files %v, denied %v, cost %v", r.FilesChanged, r.Denied, r.CostUSD)
	}
	if !strings.HasPrefix(r.Next, "allow what was denied") {
		t.Fatalf("next %q", r.Next)
	}
	if r.Git == nil || r.Git.Status != nil || !strings.HasPrefix(r.Git.Note, "no Git evidence") {
		t.Fatalf("a folder without Git reports %+v", r.Git)
	}
}

func TestRunFailures(t *testing.T) {
	for _, tc := range []struct {
		mode, to, status, error string
	}{
		{"codex-fail", "codex", statusFailed, "model overloaded"},
		{"codex-crash", "codex", statusFailed, "the inner agent exited with status 2: error: unexpected argument '--bogus'"},
		{"claude-denied", "claude", statusNeedsPermission, "Claude Code ended with error_during_execution"},
		{"opencode-denied", "opencode", statusNeedsPermission, "stopped after an action was denied"},
		{"commandcode-auth", "commandcode", statusFailed, "Command Code is not signed in; run `command-code login` or set COMMAND_CODE_API_KEY: invalid token"},
		{"commandcode-denied", "commandcode", statusNeedsPermission, "Command Code stopped because an action was not permitted"},
		{"muse-fail", "muse", statusFailed, "quota exhausted"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv(fakeEnv, tc.mode)
			r, code := bridge(t, "run", "--to", tc.to, "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "x")
			if code != exitFailed || r.Status != tc.status || r.Error != tc.error {
				t.Fatalf("exit %d, status %q, error %q", code, r.Status, r.Error)
			}
		})
	}
}

func TestRunMissingHarness(t *testing.T) {
	r, code := bridge(t, "run", "--to", "codex", "--bin", "harness-bridge-no-such-cli", "--dir", t.TempDir(), "--prompt", "x")
	if code != exitFailed || r.Status != statusFailed || !strings.Contains(r.Error, "is not installed; run `harness-bridge setup --implementer codex`") {
		t.Fatalf("exit %d, report %+v", code, r)
	}
}

func TestRunTimeout(t *testing.T) {
	t.Setenv(fakeEnv, "codex-slow")
	started := time.Now()
	r, code := bridge(t, "run", "--to", "codex", "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "x", "--timeout", "1s")
	if code != exitFailed || r.Status != statusTimedOut || r.SessionID != "thread-123" {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Fatalf("the timeout took %s to stop the harness", elapsed)
	}
}

func TestStartWaitCompletes(t *testing.T) {
	t.Setenv(fakeEnv, "codex-ok")
	dir := gitRepo(t)
	r, code := bridge(t, "start", "--to", "codex", "--bin", testExe(t), "--dir", dir, "--prompt", "add hello")
	if code != exitOK || r.Status != statusRunning || !jobIDPattern.MatchString(r.Job) || r.Next != "harness-bridge wait "+r.Job {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	w, code := bridge(t, "wait", r.Job, "--max", "30s")
	if code != exitOK || w.Status != statusCompleted || !slices.Equal(w.FilesChanged, []string{"hello.txt"}) {
		t.Fatalf("exit %d, report %+v", code, w)
	}
}

func TestStartThenCancel(t *testing.T) {
	t.Setenv(fakeEnv, "codex-slow")
	r, _ := bridge(t, "start", "--to", "codex", "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "x")
	deadline := time.Now().Add(20 * time.Second)
	for {
		w, code := bridge(t, "wait", "--max", "1s", r.Job)
		if code != exitOK || w.Status != statusRunning {
			t.Fatalf("exit %d, report %+v", code, w)
		}
		if w.SessionID == "thread-123" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("progress never showed the session: %+v", w)
		}
	}
	c, code := bridge(t, "cancel", r.Job)
	if code != exitFailed || c.Status != statusCancelled || c.SessionID != "thread-123" {
		t.Fatalf("exit %d, report %+v", code, c)
	}
}

func TestWaitReportsLostJob(t *testing.T) {
	req := Request{Harness: "codex", Dir: t.TempDir(), Prompt: "x", Sandbox: "workspace-write", Timeout: time.Minute}
	j, err := createJob(req)
	if err != nil {
		t.Fatal(err)
	}
	stale := j.final(req, Report{Status: statusRunning})
	stale.UpdatedAt = time.Now().Add(-time.Minute)
	if err := writeJSON(j.path(statusFile), stale); err != nil {
		t.Fatal(err)
	}
	r, code := bridge(t, "wait", j.id, "--max", "5s")
	if code != exitFailed || r.Status != statusLost || !strings.Contains(r.Error, workerLogFile) {
		t.Fatalf("exit %d, report %+v", code, r)
	}
}

func TestHarnessCommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  Request
		want []string
	}{
		{"codex", Request{Harness: "codex", Sandbox: "workspace-write", Model: "gpt-5.6", Reasoning: "high"},
			[]string{"exec", "--json", "--skip-git-repo-check", "-c", `sandbox_mode="workspace-write"`, "-c", `approval_policy="never"`, "--model", "gpt-5.6", "-c", `model_reasoning_effort="high"`, "-"}},
		{"codex resume", Request{Harness: "codex", Sandbox: "read-only", Session: "abc", ExtraArgs: []string{"-c", "x=1"}},
			[]string{"exec", "resume", "--json", "--skip-git-repo-check", "-c", `sandbox_mode="read-only"`, "-c", `approval_policy="never"`, "-c", "x=1", "abc", "-"}},
		{"claude", Request{Harness: "claude", Sandbox: "workspace-write", Allow: []string{"Bash(go test:*)"}, Model: "sonnet"},
			[]string{"--print", "--output-format", "stream-json", "--verbose", "--permission-mode", "acceptEdits", "--allowedTools", "Read,Glob,Grep,Edit,Write,Bash(go test:*)", "--model", "sonnet"}},
		{"claude read-only resume", Request{Harness: "claude", Sandbox: "read-only", Session: "sess-9", Reasoning: "high"},
			[]string{"--print", "--output-format", "stream-json", "--verbose", "--permission-mode", "default", "--disallowedTools", "Edit,Write,NotebookEdit", "--allowedTools", "Read,Glob,Grep", "--effort", "high", "--resume", "sess-9"}},
		{"muse", Request{Harness: "muse", Sandbox: "workspace-write", Dir: "/work"},
			[]string{"exec", "--json", "--provider", "meta", "--workspace", "/work", "--model", "muse-spark-1.3", "--reasoning-effort", "medium", "--permission-profile", ":ask-me", "--disable-shell", "--no-foreign-personal-context", "--no-session-log", "--prompt-file", "/jobs/prompt.txt"}},
		{"muse read-only", Request{Harness: "muse", Sandbox: "read-only", Dir: "/work", Model: "muse-x", Reasoning: "high"},
			[]string{"exec", "--json", "--provider", "meta", "--workspace", "/work", "--model", "muse-x", "--reasoning-effort", "high", "--permission-profile", ":read-only", "--disable-shell", "--no-foreign-personal-context", "--no-session-log", "--prompt-file", "/jobs/prompt.txt", "--disable-write"}},
		{"opencode", Request{Harness: "opencode", Sandbox: "read-only", Model: "anthropic/claude-sonnet#high", Session: "ses_1", ExtraArgs: []string{"--auto"}},
			[]string{"run", "--format", "json", "--agent", "plan", "--model", "anthropic/claude-sonnet#high", "--session", "ses_1", "--auto"}},
		{"commandcode", Request{Harness: "commandcode", Sandbox: "workspace-write", Model: "m1", Reasoning: "high", Session: "cc-1"},
			[]string{"--print", "--output-format", "json", "--permission-mode", "accept-edits", "--model", "m1", "--effort", "high", "--resume", "cc-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, args := harnessCommand(tc.req, "/jobs/prompt.txt")
			if bin != adapterNamed(tc.req.Harness).Bin || !slices.Equal(args, tc.want) {
				t.Fatalf("got %s %q\nwant %q", bin, args, tc.want)
			}
		})
	}
}

func TestParseRequestRejects(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--prompt", "x"}, "--to is required"},
		{[]string{"--to", "aider", "--prompt", "x"}, "use one of codex, claude, muse, opencode, commandcode"},
		{[]string{"--to", "muse", "--session", "s1", "--prompt", "x"}, "Muse Code cannot continue a session"},
		{[]string{"--to", "opencode", "--reasoning", "high", "--prompt", "x"}, "provider/model#variant"},
		{[]string{"--to", "codex", "--sandbox", "danger", "--prompt", "x"}, "--sandbox"},
		{[]string{"--to", "codex", "--allow", "Bash(ls)", "--prompt", "x"}, "--allow applies to Claude"},
		{[]string{"--to", "claude", "--allow", "Read,Write", "--prompt", "x"}, "one tool rule per flag"},
		{[]string{"--to", "codex", "--model", "gpt&calc", "--prompt", "x"}, "unsupported characters"},
		{[]string{"--to", "codex", "do the thing"}, "harness arguments go after --"},
		{[]string{"--to", "codex", "--dir", filepath.Join(dir, "missing"), "--prompt", "x"}, "is not a directory"},
		{[]string{"--to", "codex", "--prompt", "  "}, "the prompt is empty"},
		{[]string{"--to", "codex", "--prompt", "x", "--prompt-file", "f"}, "not both"},
		{[]string{"--to", "codex", "--timeout", "0s", "--prompt", "x"}, "--timeout"},
	} {
		if _, err := parseRequest("run", append([]string{"--dir", dir}, tc.args...), strings.NewReader("")); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want an error containing %q", tc.args, err, tc.want)
		}
	}
	req, err := parseRequest("run", []string{"--to", "codex", "--dir", dir, "--", "-c", "x=1"}, strings.NewReader("plan from stdin"))
	if err != nil || req.Prompt != "plan from stdin" || !slices.Equal(req.ExtraArgs, []string{"-c", "x=1"}) || !filepath.IsAbs(req.Dir) {
		t.Fatalf("got %+v, %v", req, err)
	}
}

func TestTrackerHandlesSplitLinesAndNoise(t *testing.T) {
	stream := "warning: not JSON\n" +
		`{"type":"thread.started","thread_id":"t-1"}` + "\n" +
		`{"type":"error","message":"reconnecting 1/5"}` + "\n" +
		`{"type":"item.completed","item":{"type":"agent_message","text":"halfway"}}` + "\n" +
		`{"type":"item.completed","item":{"type":"agent_message","text":"done ✓"}}` + "\n" +
		`{"type":"turn.completed","usage":{"output_tokens":1}}`
	tr := newTracker(Request{Harness: "codex", Dir: t.TempDir()})
	for i := range len(stream) {
		_, _ = tr.Write([]byte{stream[i]})
	}
	tr.flush()
	r := tr.snapshot()
	if status, problem := tr.outcome(); status != statusCompleted || problem != "" {
		t.Fatalf("outcome %q %q", status, problem)
	}
	if r.SessionID != "t-1" || r.Message != "done ✓" || len(r.Recent) != 3 {
		t.Fatalf("report %+v", r)
	}
}

func TestUnknownCommandAndJobID(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cli([]string{"bogus"}, nil, &out, &errOut); code != exitUsage || !strings.Contains(errOut.String(), `unknown command "bogus"`) {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	errOut.Reset()
	if code := cli([]string{"wait", "../../etc"}, nil, &out, &errOut); code != exitUsage || !strings.Contains(errOut.String(), "is not a job ID") {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
}
