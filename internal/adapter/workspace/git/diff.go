package git

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"multiharness-core/internal/store"
)

func writeSnapshot(directory string, files map[string]*fileState, names []string) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	for _, name := range names {
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

func (workspace *Workspace) diff(ctx context.Context, before, after map[string]*fileState, names []string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	directory, err := os.MkdirTemp("", "multiharness-diff-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(directory)
	if err := writeSnapshot(filepath.Join(directory, "before"), before, names); err != nil {
		return "", err
	}
	if err := writeSnapshot(filepath.Join(directory, "after"), after, names); err != nil {
		return "", err
	}
	diff, err := workspace.command(ctx, directory, true, "diff", "--no-index", "--binary", "--no-renames", "--no-ext-diff", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/", "--", "before", "after")
	if err != nil {
		return "", err
	}
	// Keep Git's relative before/after tree paths verbatim. Rewriting headers
	// is ambiguous for filenames containing spaces or repeated path prefixes.
	// ChangedFiles supplies unambiguous repository-relative names separately.
	return diff, nil
}

// Recovery copies are retained only when preservation failed or current state
// could not be inspected. They are never automatically applied to the checkout.
func saveRecovery(baseline snapshot) (string, error) {
	directory, err := os.MkdirTemp("", "multiharness-recovery-")
	if err != nil {
		return "", fmt.Errorf("save recovery snapshot: %w", err)
	}
	if err := writeSnapshot(filepath.Join(directory, "files"), baseline.files, sortedNames(baseline.files)); err != nil {
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
		IndexEntries string                `json:"index_entries"`
		HeadRef      string                `json:"head_ref"`
		MissingFiles []string              `json:"missing_files"`
	}{baseline.state, baseline.index, baseline.ref, missing}, "", "  ")
	if err != nil {
		return directory, err
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), manifest, 0600); err != nil {
		return directory, err
	}
	return directory, nil
}
