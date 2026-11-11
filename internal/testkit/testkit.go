// Package testkit holds what the tests of several packages share: a TestMain
// that lets the test binary stand in for an agent CLI and for the detached
// worker, and helpers for fake CLIs, saved choices and temporary files. Only
// tests import it.
package testkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/config"
	"multiharness-core/internal/job"
)

// FakeEnv makes the test binary act as a harness. The bridge runs it through
// --bin, and runs it again as the detached worker for start.
const FakeEnv = "HARNESS_BRIDGE_FAKE"

// exeSuffix is what fake CLIs need to be found on PATH: .exe on Windows,
// nothing elsewhere (where the test binary's own ".test" must not be copied).
var exeSuffix = map[bool]string{true: ".exe", false: ""}[runtime.GOOS == "windows"]

func Main(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == job.WorkerCommand {
		os.Exit(job.CmdWorker(os.Args[2:], os.Stderr))
	}
	if mode := os.Getenv(FakeEnv); mode != "" {
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
	if delay, err := time.ParseDuration(os.Getenv(FakeEnv + "_DELAY")); err == nil {
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
	case "codex-tree":
		// Starts a child that would survive a signal to the harness alone;
		// HARNESS_BRIDGE_TREE_PID says where to record the child's pid.
		if runtime.GOOS != "windows" {
			child := exec.Command("sleep", "60")
			if child.Start() == nil {
				_ = os.WriteFile(os.Getenv("HARNESS_BRIDGE_TREE_PID"), []byte(strconv.Itoa(child.Process.Pid)), 0o600)
			}
		}
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
			map[string]any{"type": "tool_use", "id": "w1", "name": "Write", "input": map[string]any{"file_path": path, "content": "x"}},
		}}})
		emit(map[string]any{"type": "user", "session_id": "sess-9", "message": map[string]any{"content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "w1", "is_error": false},
		}}})
		emit(map[string]any{"type": "assistant", "session_id": "sess-9", "message": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "Wrote notes.md"},
		}}})
		emit(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": "Wrote notes.md", "session_id": "sess-9",
			"total_cost_usd": 0.01, "permission_denials": []any{map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "rm -rf x"}}}})
	case "claude-mixed":
		// One edit is denied (an error tool_result), one succeeds.
		gone, kept := filepath.Join(cwd, "denied.txt"), filepath.Join(cwd, "done.txt")
		write := func(id, path string) {
			emit(map[string]any{"type": "assistant", "session_id": "sess-9", "message": map[string]any{"content": []any{
				map[string]any{"type": "tool_use", "id": id, "name": "Write", "input": map[string]any{"file_path": path, "content": "x"}},
			}}})
		}
		result := func(id string, isError bool) {
			emit(map[string]any{"type": "user", "session_id": "sess-9", "message": map[string]any{"content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": id, "is_error": isError},
			}}})
		}
		write("w1", gone)
		result("w1", true)
		write("w2", kept)
		result("w2", false)
		emit(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": "One edit was denied.", "session_id": "sess-9"})
	case "claude-hangs":
		// Prints its final event, then lingers like an MCP server it started.
		emit(map[string]any{"type": "system", "subtype": "init", "session_id": "sess-9"})
		emit(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": "Done", "session_id": "sess-9"})
		time.Sleep(time.Minute)
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
	case "opencode-mixed":
		// One edit fails, one completes.
		ok, failed := filepath.Join(cwd, "done.go"), filepath.Join(cwd, "skipped.go")
		ev := func(kind string, part map[string]any) {
			emit(map[string]any{"type": kind, "sessionID": "ses_7", "part": part})
		}
		ev("step_start", map[string]any{})
		ev("tool_use", map[string]any{"tool": "edit", "state": map[string]any{"status": "error", "input": map[string]any{"filePath": failed}, "error": "the file vanished"}})
		ev("tool_use", map[string]any{"tool": "edit", "state": map[string]any{"status": "completed", "input": map[string]any{"filePath": ok}}})
		ev("step_finish", map[string]any{"reason": "stop", "cost": 0.01})
	case "codex-mixed":
		// One file change fails, one completes.
		emit(item("item.completed", map[string]any{"type": "file_change", "status": "failed", "changes": []any{map[string]any{"path": filepath.Join(cwd, "skipped.txt"), "kind": "add"}}}))
		emit(item("item.completed", map[string]any{"type": "file_change", "status": "completed", "changes": []any{map[string]any{"path": filepath.Join(cwd, "done.txt"), "kind": "add"}}}))
		emit(map[string]any{"type": "turn.completed"})
	case "commandcode-ok":
		emit(map[string]any{"type": "event", "event": map[string]any{"type": "tool_start", "toolCallId": "t1", "toolName": "edit_file", "description": "README.md"}})
		emit(map[string]any{"type": "event", "event": map[string]any{"type": "something_new"}})
		emit(map[string]any{"type": "result", "subtype": "success", "sessionId": "cc-42", "finalText": "Updated the README.", "usage": map[string]any{"inputTokens": 3}})
	case "commandcode-yolo":
		emit(map[string]any{"type": "event", "event": map[string]any{"type": "tool_hook_blocked", "toolCallId": "call_300dbd4aa67143808a48244d", "toolName": "edit_file",
			"hookOutput": `Error: Tool "edit_file" requires permissions. Use --yolo (or --dangerously-skip-permissions) to enable file edits.`}})
		emit(map[string]any{"type": "result", "subtype": "success", "sessionId": "cc-42", "finalText": `I couldn't edit README.md: Tool "edit_file" requires permissions. Use --yolo.`})
	case "commandcode-resumed":
		// A resumed session repeats the earlier refusal in its transcript
		// events; only a real tool_hook_blocked event is a refusal.
		refusal := `Tool "edit_file" requires permissions. Use --yolo to enable file edits.`
		emit(map[string]any{"type": "event", "event": map[string]any{"type": "run_end", "text": refusal}})
		emit(map[string]any{"type": "event", "event": map[string]any{"type": "message_update", "text": refusal}})
		emit(map[string]any{"type": "result", "subtype": "success", "sessionId": "cc-42", "finalText": "Done."})
	case "commandcode-stopped":
		// The turn ended because an action was refused, yet reports success.
		emit(map[string]any{"type": "result", "subtype": "success", "stopReason": "permission_denied", "sessionId": "cc-42", "finalText": "Let me read the files first."})
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
	if path := os.Getenv(FakeEnv + "_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			fmt.Fprintf(f, "%s %s\n", name, strings.Join(os.Args[1:], " "))
			f.Close()
		}
	}
	if name == "command-code" && len(os.Args) > 1 && os.Args[1] == "--list-models" {
		fmt.Print("Available models  ·  3 models\n" +
			"\n" +
			"Open Source\n" +
			"\n" +
			"moonshotai/kimi-k2.5                    multimodal frontend coding\n" +
			"\n" +
			"Anthropic\n" +
			"\n" +
			"claude-sonnet-5-5                       best combo of speed & intelligence (recommended)\n" +
			"claude-haiku-5-5                        fastest & most compact\n" +
			"\n" +
			"Pass the full id, or just the short name after the last \"/\":\n" +
			"cmdc --model moonshotai/kimi-k2.5\n" +
			"\n" +
			"Decision models (headless only)\n" +
			"typesafe/jev  typed questions in, probabilities out\n")
		return 0
	}
	if name == "opencode" && len(os.Args) > 1 && os.Args[1] == "models" {
		fmt.Print("anthropic/claude-sonnet-5-5\nopenai/gpt-5.5\nopencode/big-pickle\n")
		return 0
	}
	if os.Getenv(FakeEnv+"_FAIL") == "mcp-remove" && len(os.Args) > 2 && os.Args[1] == "mcp" && os.Args[2] == "remove" {
		fmt.Fprintln(os.Stderr, name+" mcp remove: connection refused")
		return 1
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(agent.FirstNonEmpty(os.Getenv(FakeEnv+"_VERSION"), "1.0.0"))
		return 0
	}
	signedOut := slices.Contains(strings.Split(os.Getenv(FakeEnv+"_SIGNED_OUT"), ","), name)
	for _, a := range agent.All {
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

// FakeCLIs puts copies of the test binary, acting as fakeCLI, on PATH under
// the given names, and points the home and app-data folders at an empty
// directory so no real installation is found.
func FakeCLIs(t *testing.T, names ...string) (logPath string) {
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
	t.Setenv(FakeEnv, "cli")
	logPath = filepath.Join(home, "calls.log")
	t.Setenv(FakeEnv+"_LOG", logPath)
	return logPath
}

// UseConfig saves cfg as the setup choices for one test.
func UseConfig(t *testing.T, cfg config.Config) {
	t.Helper()
	path, err := config.Save(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })
}

// Bridge runs one command through run, the command dispatcher, and decodes the
// report it prints.
func Bridge(t *testing.T, run func(args []string, stdin io.Reader, stdout, stderr io.Writer) int, args ...string) (agent.Report, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errOut)
	var r agent.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("harness-bridge %v: exit %d\nstdout: %s\nstderr: %s", args, code, out.String(), errOut.String())
	}
	return r, code
}

func TestExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func GitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Skipf("git is not available: %v %s", err, out)
	}
	return dir
}

func Write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// EmptyPath leaves only dir on PATH and points the home and app-data folders
// at it, so no real CLI or installer is found.
func EmptyPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	for _, v := range []string{"HOME", "USERPROFILE", "APPDATA"} {
		t.Setenv(v, dir)
	}
	return dir
}

func Read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func Calls(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

type seenRequest struct {
	Auth, Title string
	Body        map[string]any
}

func DecisionServer(t *testing.T, status int, reply string) (*httptest.Server, *seenRequest) {
	t.Helper()
	seen := &seenRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &seen.Body)
		seen.Auth, seen.Title = r.Header.Get("Authorization"), r.Header.Get("X-Title")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}
