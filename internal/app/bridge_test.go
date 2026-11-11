package app

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/config"
	"multiharness-core/internal/job"
	"multiharness-core/internal/testkit"
)

func TestRunCodexReportsChangesAndGitEvidence(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "codex-ok")
	dir := testkit.GitRepo(t)
	r, code := testkit.Bridge(t, Run, "run", "--to", "codex", "--bin", testkit.TestExe(t), "--dir", dir, "--prompt", "add hello")
	if code != cli.ExitOK || r.Status != agent.StatusCompleted {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	// The implementer gets the brief first, then the plan.
	if r.SessionID != "thread-123" || !strings.HasPrefix(r.Message, "Created hello.txt for: You are the implementer of this harness-bridge job.") || !strings.HasSuffix(r.Message, "add hello") {
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
	if again, _ := testkit.Bridge(t, Run, "wait", r.Job, "--max", "0"); again.Status != agent.StatusCompleted || again.SessionID != r.SessionID {
		t.Fatalf("a finished job reads back as %+v", again)
	}
}

func TestRunClaudeReportsDenials(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "claude-ok")
	r, code := testkit.Bridge(t, Run, "run", "--to", "claude", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "write notes")
	if code != cli.ExitOK || r.Status != agent.StatusCompleted || r.SessionID != "sess-9" || r.Message != "Wrote notes.md" {
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
		{"codex-fail", "codex", agent.StatusFailed, "model overloaded"},
		{"codex-crash", "codex", agent.StatusFailed, "the inner agent exited with status 2: error: unexpected argument '--bogus'"},
		{"claude-denied", "claude", agent.StatusNeedsPermission, "Claude Code ended with error_during_execution"},
		{"opencode-denied", "opencode", agent.StatusNeedsPermission, "stopped after an action was denied"},
		{"commandcode-auth", "commandcode", agent.StatusFailed, "Command Code is not signed in; run `command-code login` or set COMMAND_CODE_API_KEY: invalid token"},
		{"commandcode-denied", "commandcode", agent.StatusNeedsPermission, "Command Code stopped because an action was not permitted"},
		{"muse-fail", "muse", agent.StatusFailed, "quota exhausted"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv(testkit.FakeEnv, tc.mode)
			r, code := testkit.Bridge(t, Run, "run", "--to", tc.to, "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "x")
			if code != cli.ExitFailed || r.Status != tc.status || r.Error != tc.error {
				t.Fatalf("exit %d, status %q, error %q", code, r.Status, r.Error)
			}
		})
	}
}

func TestRunMissingHarness(t *testing.T) {
	r, code := testkit.Bridge(t, Run, "run", "--to", "codex", "--bin", "harness-bridge-no-such-cli", "--dir", t.TempDir(), "--prompt", "x")
	if code != cli.ExitFailed || r.Status != agent.StatusFailed || !strings.Contains(r.Error, "is not installed; run `harness-bridge setup --implementer codex`") {
		t.Fatalf("exit %d, report %+v", code, r)
	}
}

func TestRunTimeout(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "codex-slow")
	started := time.Now()
	r, code := testkit.Bridge(t, Run, "run", "--to", "codex", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "x", "--timeout", "1s")
	if code != cli.ExitFailed || r.Status != agent.StatusTimedOut || r.SessionID != "thread-123" {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Fatalf("the timeout took %s to stop the harness", elapsed)
	}
}

func TestStartWaitCompletes(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "codex-ok")
	dir := testkit.GitRepo(t)
	r, code := testkit.Bridge(t, Run, "start", "--to", "codex", "--bin", testkit.TestExe(t), "--dir", dir, "--prompt", "add hello")
	if code != cli.ExitOK || r.Status != agent.StatusRunning || !job.IDPattern.MatchString(r.Job) || r.Next != "harness-bridge wait "+r.Job {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	w, code := testkit.Bridge(t, Run, "wait", r.Job, "--max", "30s")
	if code != cli.ExitOK || w.Status != agent.StatusCompleted || !slices.Equal(w.FilesChanged, []string{"hello.txt"}) {
		t.Fatalf("exit %d, report %+v", code, w)
	}
}

func TestStartThenCancel(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "codex-slow")
	r, _ := testkit.Bridge(t, Run, "start", "--to", "codex", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "x")
	deadline := time.Now().Add(20 * time.Second)
	for {
		w, code := testkit.Bridge(t, Run, "wait", "--max", "1s", r.Job)
		if code != cli.ExitOK || w.Status != agent.StatusRunning {
			t.Fatalf("exit %d, report %+v", code, w)
		}
		if w.SessionID == "thread-123" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("progress never showed the session: %+v", w)
		}
	}
	c, code := testkit.Bridge(t, Run, "cancel", r.Job)
	if code != cli.ExitFailed || c.Status != agent.StatusCancelled || c.SessionID != "thread-123" {
		t.Fatalf("exit %d, report %+v", code, c)
	}
}

func TestUnknownCommandAndJobID(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"bogus"}, nil, &out, &errOut); code != cli.ExitUsage || !strings.Contains(errOut.String(), `unknown command "bogus"`) {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	errOut.Reset()
	if code := Run([]string{"wait", "../../etc"}, nil, &out, &errOut); code != cli.ExitUsage || !strings.Contains(errOut.String(), "is not a job ID") {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
}

func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if cli.IsTerminal(f) {
		t.Fatal("a file from os.DevNull counts as a terminal")
	}
	config, _ := config.Path()
	var out, errOut bytes.Buffer
	if code := Run([]string{}, f, &out, &errOut); code != cli.ExitUsage || !strings.Contains(errOut.String(), "--planner") {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if _, err := os.Stat(config); err == nil {
		t.Fatal("the wizard ran and wrote files without a terminal")
	}
}
