package main

import (
	"slices"
	"strings"
	"testing"
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
			t.Setenv(fakeEnv, tc.mode)
			r, code := bridge(t, "run", "--to", tc.to, "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "tidy up")
			if code != exitOK || r.Status != statusCompleted {
				t.Fatalf("exit %d, report %+v", code, r)
			}
			if r.SessionID != tc.session || r.Message != tc.message || !slices.Equal(r.FilesChanged, tc.files) {
				t.Fatalf("session %q, message %q, files %v", r.SessionID, r.Message, r.FilesChanged)
			}
		})
	}
}

func TestOpenCodeReportsCommandsAndCost(t *testing.T) {
	t.Setenv(fakeEnv, "opencode-ok")
	r, _ := bridge(t, "run", "--to", "opencode", "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "x")
	if len(r.Commands) != 1 || r.Commands[0].Command != "go build ./..." || r.Commands[0].ExitCode == nil || r.CostUSD < 0.029 {
		t.Fatalf("commands %+v, cost %v", r.Commands, r.CostUSD)
	}
	if !strings.Contains(r.Next, "--to opencode --session ses_7") {
		t.Fatalf("next %q", r.Next)
	}
}

func TestDeniedActionsAreListed(t *testing.T) {
	t.Setenv(fakeEnv, "opencode-denied")
	r, _ := bridge(t, "run", "--to", "opencode", "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "x")
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
			t.Setenv(fakeEnv, tc.mode)
			r, code := bridge(t, "run", "--to", tc.to, "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "x")
			if code != exitOK || r.Status != statusCompleted {
				t.Fatalf("exit %d, report %+v", code, r)
			}
			if !slices.Equal(r.FilesChanged, []string{tc.want}) {
				t.Fatalf("files_changed %v", r.FilesChanged)
			}
		})
	}
}

func TestMuseCannotResume(t *testing.T) {
	t.Setenv(fakeEnv, "muse-fail")
	r, _ := bridge(t, "run", "--to", "muse", "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "x")
	if !strings.Contains(r.Next, "cannot continue a session") {
		t.Fatalf("next %q", r.Next)
	}
}
