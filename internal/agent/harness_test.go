package agent

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// ErrWaitDelay means the process exited successfully and only a child still
// held its output, so it is not a failure.
func TestExitCodeTreatsWaitDelayAsSuccess(t *testing.T) {
	if code := exitCode(exec.ErrWaitDelay); code != 0 {
		t.Fatalf("exitCode(ErrWaitDelay) = %d", code)
	}
	if code := exitCode(fmt.Errorf("copying output: %w", exec.ErrWaitDelay)); code != 0 {
		t.Fatalf("exitCode(wrapped ErrWaitDelay) = %d", code)
	}
}

// On Windows, .cmd/.bat shims run through cmd.exe, whose quoting rules can
// split or inject arguments; unsafe values must be refused before the run.
func TestCmdShimArgsChecked(t *testing.T) {
	shim := func(path string, req Request) error {
		return checkCmdShimArgs(path, req)
	}
	safe := Request{Harness: "codex", ExtraArgs: []string{"--yolo"}, Allow: []string{"Bash(go test:*)"}}
	if err := shim("C:\\npm\\codex.cmd", safe); err != nil {
		t.Fatalf("safe values for a .cmd: %v", err)
	}
	if err := shim("C:\\npm\\codex.CMD", safe); err != nil {
		t.Fatalf("safe values for a .CMD: %v", err)
	}
	if err := shim("C:\\npm\\codex.bat", safe); err != nil {
		t.Fatalf("safe values for a .bat: %v", err)
	}
	for _, v := range []string{"a&b", `x"y`, "Bash(make|tee)"} {
		req := Request{Harness: "codex", ExtraArgs: []string{v}}
		if err := shim("C:\\npm\\codex.cmd", req); err == nil || !strings.Contains(err.Error(), "cmd.exe") {
			t.Fatalf("%q for a .cmd: %v", v, err)
		}
	}
	if err := shim("C:\\npm\\codex.cmd", Request{Harness: "codex", Allow: []string{"Bash(make|tee)"}}); err == nil || !strings.Contains(err.Error(), "--allow") {
		t.Fatalf("an unsafe allow rule: %v", err)
	}
	unsafe := Request{Harness: "codex", ExtraArgs: []string{"a&b", `x"y`, "Bash(make|tee)"}}
	if err := shim("C:\\go\\codex.exe", unsafe); err != nil {
		t.Fatalf("a .exe is not run by cmd.exe: %v", err)
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
			if bin != Named(tc.req.Harness).Bin || !slices.Equal(args, tc.want) {
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
		if _, err := ParseRequest("run", append([]string{"--dir", dir}, tc.args...), strings.NewReader("")); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want an error containing %q", tc.args, err, tc.want)
		}
	}
	req, err := ParseRequest("run", []string{"--to", "codex", "--dir", dir, "--", "-c", "x=1"}, strings.NewReader("plan from stdin"))
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
	tr := NewTracker(Request{Harness: "codex", Dir: t.TempDir()})
	for i := range len(stream) {
		_, _ = tr.Write([]byte{stream[i]})
	}
	tr.flush()
	r := tr.Snapshot()
	if status, problem := tr.outcome(); status != StatusCompleted || problem != "" {
		t.Fatalf("outcome %q %q", status, problem)
	}
	if r.SessionID != "t-1" || r.Message != "done ✓" || len(r.Recent) != 3 {
		t.Fatalf("report %+v", r)
	}
}
