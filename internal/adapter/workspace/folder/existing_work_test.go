//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package folder

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/store"
)

type existingApproval func(context.Context, store.ExistingWork) (bool, error)

func (f existingApproval) ConfirmExistingWork(ctx context.Context, r store.ExistingWork) (bool, error) {
	return f(ctx, r)
}

func TestExistingFolderCanContinueWithBackupAndExplicitConsent(t *testing.T) {
	dir, backups := t.TempDir(), t.TempDir()
	put(t, dir, "app.txt", "original work")
	put(t, dir, "notes.txt", "personal notes")
	var saved string
	w, err := NewWorkspaceWithApproval(Config{ExistingWork: "prompt", RecoveryDir: backups}, existingApproval(func(ctx context.Context, r store.ExistingWork) (bool, error) {
		saved = r.RecoveryDirectory
		content, err := os.ReadFile(filepath.Join(saved, "files", "app.txt"))
		if err != nil || string(content) != "original work" {
			t.Fatal("consent requested before backup", err)
		}
		if len(r.Files) != 2 {
			t.Fatal("wrong consent scope", r.Files)
		}
		return true, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	s := acquire(t, w, dir)
	if !s.Baseline().ExistingWorkAuthorized {
		t.Fatal("authorization not delivered to agents")
	}
	put(t, dir, "app.txt", "implemented")
	evidence, err := s.Inspect(t.Context())
	if err != nil || len(evidence.PreservationViolations) != 0 || !strings.Contains(evidence.Diff, "+implemented") {
		t.Fatal("authorized edits rejected", err, evidence)
	}
	put(t, dir, "app.txt", "repaired")
	evidence, err = s.Inspect(t.Context())
	if err != nil || !strings.Contains(evidence.Diff, "-original work") || !strings.Contains(evidence.Diff, "+repaired") {
		t.Fatal("repair lost original baseline", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(saved, "files", "app.txt"))
	if err != nil || string(content) != "original work" {
		t.Fatal("backup lost after close", err)
	}
	if entries, err := os.ReadDir(backups); err != nil || len(entries) != 1 {
		t.Fatal("backup location lost", err)
	}
}

func TestExistingWorkRefusalCancellationAndConcurrentChangesStopBeforeEdits(t *testing.T) {
	for _, mode := range []string{"refuse", "cancel", "changed", "unattended"} {
		t.Run(mode, func(t *testing.T) {
			dir := repository(t)
			put(t, dir, "notes.txt", "existing work")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			w, err := NewWorkspaceWithApproval(Config{ExistingWork: "prompt", RecoveryDir: t.TempDir()}, existingApproval(func(context.Context, store.ExistingWork) (bool, error) {
				switch mode {
				case "refuse":
					return false, nil
				case "cancel":
					cancel()
				case "changed":
					put(t, dir, "notes.txt", "new user edit")
				}
				return true, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			if mode == "unattended" {
				w.approver = nil
			}
			if s, err := w.Acquire(ctx, dir); err == nil || s != nil {
				t.Fatal("unsafe acquisition accepted")
			} else if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			// All failures release the lease without rolling back the newer file.
			s := acquire(t, newWorkspace(t, Config{}), dir)
			_ = s
			content, _ := os.ReadFile(filepath.Join(dir, "notes.txt"))
			want := "existing work"
			if mode == "changed" {
				want = "new user edit"
			}
			if string(content) != want {
				t.Fatal("user work overwritten")
			}
		})
	}
}

func TestRecoveryStorageFailureAndWorkspaceSymlinkFailBeforeConsent(t *testing.T) {
	for _, mode := range []string{"inside", "symlink", "unwritable"} {
		t.Run(mode, func(t *testing.T) {
			dir := repository(t)
			put(t, dir, "notes.txt", "existing work")
			parent := t.TempDir()
			dest := filepath.Join(parent, "backups")
			switch mode {
			case "inside":
				dest = filepath.Join(dir, "backup")
			case "symlink":
				if err := os.Symlink(dir, filepath.Join(parent, "link")); err != nil {
					t.Fatal(err)
				}
				dest = filepath.Join(parent, "link", "backup")
			case "unwritable":
				put(t, parent, "backups", "a regular file")
			}
			w, err := NewWorkspaceWithApproval(Config{ExistingWork: "prompt", RecoveryDir: dest}, existingApproval(func(context.Context, store.ExistingWork) (bool, error) {
				t.Fatal("consent before valid storage")
				return true, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			if s, err := w.Acquire(t.Context(), dir); err == nil || s != nil {
				t.Fatal("unsafe backup accepted")
			}
			if _, err := os.Stat(filepath.Join(dir, "backup")); !os.IsNotExist(err) {
				t.Fatal("backup wrote into workspace", err)
			}
		})
	}
}
