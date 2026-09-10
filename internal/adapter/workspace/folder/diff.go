package folder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"multiharness-core/internal/store"
)

func writeSnapshot(ctx context.Context, directory string, files map[string]*fileState, names []string) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		file := files[name]
		if file == nil {
			continue
		}
		if err := validPath(name); err != nil {
			return err
		}
		target := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if file.mode&os.ModeSymlink != 0 {
			if err := os.Symlink(string(file.data), target); err != nil {
				return err
			}
		} else {
			if err := os.WriteFile(target, file.data, file.mode.Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Workspace) saveRecovery(ctx context.Context, baseline snapshot, root string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	parent, err := w.recoveryParent(root)
	if err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp(parent, "multiharness-recovery-")
	if err != nil {
		return "", fmt.Errorf("save recovery snapshot: %w", err)
	}
	if err := writeSnapshot(ctx, filepath.Join(directory, "files"), baseline.files, sortedNames(baseline.files)); err != nil {
		return directory, fmt.Errorf("partial recovery snapshot at %s: %w", directory, err)
	}
	missing := []string{}
	for _, name := range sortedNames(baseline.files) {
		if baseline.files[name] == nil {
			missing = append(missing, name)
		}
	}
	manifest, err := json.MarshalIndent(struct {
		State        store.RepositoryState `json:"state"`
		MissingFiles []string              `json:"missing_files"`
	}{baseline.state, missing}, "", "  ")
	if err != nil {
		return directory, err
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), manifest, 0600); err != nil {
		return directory, err
	}
	return directory, nil
}
