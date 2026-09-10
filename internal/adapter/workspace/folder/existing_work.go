package folder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"multiharness-core/internal/store"
)

// Keep recovery files outside inspected files, and under persistent personal
// configuration rather than /tmp. Docker puts this home on its state volume.
func (w *Workspace) recoveryParent(root string) (string, error) {
	parent := w.config.RecoveryDir
	if parent == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("cannot locate recovery storage: %w", err)
		}
		parent = filepath.Join(base, "magent", "recovery")
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	// Resolve the nearest existing ancestor before creating any directories.
	// A symlink into the workspace must not create even an empty backup folder there.
	pending := []string{}
	ancestor := parent
	for {
		_, err := os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		pending = append(pending, filepath.Base(ancestor))
		next := filepath.Dir(ancestor)
		if next == ancestor {
			return "", fmt.Errorf("cannot resolve recovery folder")
		}
		ancestor = next
	}
	ancestor, err = filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	parent = ancestor
	for i := len(pending) - 1; i >= 0; i-- {
		parent = filepath.Join(parent, pending[i])
	}
	if inside(root, parent) {
		return "", fmt.Errorf("choose a recovery folder outside the selected workspace")
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", fmt.Errorf("cannot create recovery folder: %w", err)
	}
	return parent, nil
}
func inside(root, name string) bool {
	rel, err := filepath.Rel(root, name)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (s *session) prepareExistingWork(ctx context.Context) error {
	// Write a complete recovery baseline before granting any agent write access.
	var err error
	s.recovery, err = s.workspace.saveRecovery(ctx, s.baseline, s.root)
	if err != nil {
		return err
	}
	mode := s.workspace.config.ExistingWork
	if len(s.baseline.existingFiles) > 0 && mode == "prompt" {
		if s.workspace.approver == nil {
			return fmt.Errorf("your folder contains existing work; no files were edited. Open the interactive app to allow updates, or explicitly select --existing-work snapshot. Backup: %s", s.recovery)
		}
		approved, err := s.workspace.approver.ConfirmExistingWork(ctx, store.ExistingWork{WorkingDir: s.root, Files: append([]string{}, s.baseline.existingFiles...), RecoveryDirectory: s.recovery})
		if err != nil {
			return fmt.Errorf("existing-work confirmation failed; backup: %s: %w", s.recovery, err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !approved {
			return fmt.Errorf("stopped before editing your existing files. Backup: %s. Start another task when you are ready to allow updates", s.recovery)
		}
		s.editExisting = true
	} else if mode == "snapshot" {
		s.editExisting = true
	}
	// Consent and backup I/O can take time. Never let approval cover newer work.
	confirmed, err := s.workspace.stableCapture(ctx, s.root, s.baseline.files)
	if err != nil {
		return fmt.Errorf("cannot verify the backed-up files; backup: %s: %w", s.recovery, err)
	}
	if confirmed.state.Fingerprint != s.baseline.state.Fingerprint {
		return fmt.Errorf("your files changed while preparing the task; no agent edits started. Please start again. Backup: %s", s.recovery)
	}
	return nil
}
