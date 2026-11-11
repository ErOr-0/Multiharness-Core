package app

import (
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/testkit"
)

func TestRunEachAgentCompletes(t *testing.T) {
	for _, tc := range []struct {
		mode, to, session, message string
		files                      []string
	}{
		{"muse-ok", "muse", "s-1", "Muse read: tidy up", nil},
		{"opencode-ok", "opencode", "ses_7", "Edited app.go.\n\nThe build passes.", []string{"app.go"}},
		{"commandcode-ok", "commandcode", "cc-42", "Updated the README.", nil},
	} {
		t.Run(tc.to, func(t *testing.T) {
			t.Setenv(testkit.FakeEnv, tc.mode)
			r, code := testkit.Bridge(t, Run, "run", "--to", tc.to, "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "tidy up")
			if code != cli.ExitOK || r.Status != agent.StatusCompleted {
				t.Fatalf("exit %d, report %+v", code, r)
			}
			message := r.Message
			if tc.to == "muse" {
				// The prompt-file implementer gets the brief first, then the plan.
				if !strings.HasPrefix(message, "Muse read: You are the implementer of this harness-bridge job.") || !strings.HasSuffix(message, "tidy up") {
					t.Fatalf("message %q", message)
				}
				message = tc.message
			}
			if r.SessionID != tc.session || message != tc.message || !slices.Equal(r.FilesChanged, tc.files) {
				t.Fatalf("session %q, message %q, files %v", r.SessionID, r.Message, r.FilesChanged)
			}
		})
	}
}

// A job's implementer process gets the brief first and runs marked as an
// implementer, so a same-CLI implementer does the work instead of handing it
// back.
func TestRunMarksTheImplementerProcess(t *testing.T) {
	for _, tc := range []struct{ mode, to string }{
		{"claude-echo-brief", "claude"},
		{"muse-echo-brief", "muse"},
	} {
		t.Run(tc.to, func(t *testing.T) {
			t.Setenv(testkit.FakeEnv, tc.mode)
			r, code := testkit.Bridge(t, Run, "run", "--to", tc.to, "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "tidy up")
			if code != cli.ExitOK || r.Status != agent.StatusCompleted {
				t.Fatalf("exit %d, report %+v", code, r)
			}
			if r.Message != "brief=true job=1" {
				t.Fatalf("message %q", r.Message)
			}
		})
	}
}

func TestOpenCodeReportsCommandsAndCost(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "opencode-ok")
	r, _ := testkit.Bridge(t, Run, "run", "--to", "opencode", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "x")
	if len(r.Commands) != 1 || r.Commands[0].Command != "go build ./..." || r.Commands[0].ExitCode == nil || r.CostUSD < 0.029 {
		t.Fatalf("commands %+v, cost %v", r.Commands, r.CostUSD)
	}
	if !strings.Contains(r.Next, "--to opencode --session ses_7") {
		t.Fatalf("next %q", r.Next)
	}
}

func TestDeniedActionsAreListed(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "opencode-denied")
	r, _ := testkit.Bridge(t, Run, "run", "--to", "opencode", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "x")
	if !slices.Equal(r.Denied, []string{"bash(go test ./...)"}) || !strings.HasPrefix(r.Next, "allow what was denied") {
		t.Fatalf("denied %v, next %q", r.Denied, r.Next)
	}
}

// files_changed must list only edits that actually happened, not attempts
// that were denied or failed.
func TestFilesChangedListsOnlySuccessfulEdits(t *testing.T) {
	for _, tc := range []struct {
		mode, to, want string
	}{
		{"claude-mixed", "claude", "done.txt"},
		{"opencode-mixed", "opencode", "done.go"},
		{"codex-mixed", "codex", "done.txt"},
	} {
		t.Run(tc.to, func(t *testing.T) {
			t.Setenv(testkit.FakeEnv, tc.mode)
			r, code := testkit.Bridge(t, Run, "run", "--to", tc.to, "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "x")
			if code != cli.ExitOK || r.Status != agent.StatusCompleted {
				t.Fatalf("exit %d, report %+v", code, r)
			}
			if !slices.Equal(r.FilesChanged, []string{tc.want}) {
				t.Fatalf("files_changed %v", r.FilesChanged)
			}
		})
	}
}

func TestMuseCannotResume(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "muse-fail")
	r, _ := testkit.Bridge(t, Run, "run", "--to", "muse", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "x")
	if !strings.Contains(r.Next, "cannot continue a session") {
		t.Fatalf("next %q", r.Next)
	}
}

func TestCommandCodeRefusedEditsNeedPermission(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "commandcode-yolo")
	r, code := testkit.Bridge(t, Run, "run", "--to", "commandcode", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "edit the readme")
	if code != cli.ExitFailed || r.Status != agent.StatusNeedsPermission || !slices.Equal(r.Denied, []string{"edit_file (needs --yolo)"}) || !strings.Contains(r.Error, "Ask the user before") {
		t.Fatalf("exit %d, report %+v", code, r)
	}
}

// A turn that quit because an action was refused is not a completed job, even
// though Command Code calls the result a success.
func TestCommandCodeStoppedOnRefusalNeedsPermission(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "commandcode-stopped")
	r, code := testkit.Bridge(t, Run, "run", "--to", "commandcode", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "edit the readme")
	if code != cli.ExitFailed || r.Status != agent.StatusNeedsPermission || !strings.Contains(r.Error, "not permitted") || r.SessionID != "cc-42" {
		t.Fatalf("exit %d, report %+v", code, r)
	}
}

func TestCommandCodeRepeatedRefusalIsNotOne(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "commandcode-resumed")
	r, code := testkit.Bridge(t, Run, "run", "--to", "commandcode", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "edit the readme")
	if code != cli.ExitOK || r.Status != agent.StatusCompleted || len(r.Denied) != 0 {
		t.Fatalf("exit %d, report %+v", code, r)
	}
}
