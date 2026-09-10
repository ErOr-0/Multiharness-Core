package folder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"multiharness-core/internal/store"
)

type fileState struct {
	data []byte
	mode os.FileMode
}
type snapshot struct {
	state         store.RepositoryState
	files         map[string]*fileState
	existingFiles []string
}

func (workspace *Workspace) stableCapture(ctx context.Context, root string, baseline map[string]*fileState) (snapshot, error) {
	if ctx == nil {
		return snapshot{}, fmt.Errorf("workspace context is required")
	}
	ctx, cancel := context.WithTimeout(ctx, workspace.config.Timeout)
	defer cancel()
	first, err := workspace.capture(ctx, root, baseline)
	if err != nil {
		return snapshot{}, err
	}
	second, err := workspace.capture(ctx, root, baseline)
	if err != nil {
		return snapshot{}, err
	}
	if first.state.Fingerprint != second.state.Fingerprint {
		return snapshot{}, ErrChangedDuringCapture
	}
	return second, nil
}

func (workspace *Workspace) capture(ctx context.Context, root string, baseline map[string]*fileState) (snapshot, error) {
	result, names, err := workspace.collect(ctx, root)
	if err != nil {
		return snapshot{}, err
	}
	for name := range baseline {
		names[name] = true
	} // A changed ignore rule must not hide baseline files.
	if workspace.config.MaxFiles > 0 && len(names) > workspace.config.MaxFiles {
		return snapshot{}, fmt.Errorf("snapshot exceeds %d files", workspace.config.MaxFiles)
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return snapshot{}, err
	}
	defer rootFS.Close()
	var total int64
	for _, name := range sortedNames(names) {
		if err := ctx.Err(); err != nil {
			return snapshot{}, err
		}
		if err := validPath(name); err != nil {
			return snapshot{}, err
		}
		file, err := readFile(rootFS, name, workspace.config.MaxFileBytes)
		if err != nil {
			return snapshot{}, fmt.Errorf("snapshot %q: %w", name, err)
		}
		if file == nil {
			// Retain absent paths, including staged deletions, so a later
			// ignore rule cannot hide their recreation from preservation checks.
			result.files[name] = nil
			continue
		}
		if workspace.config.MaxSnapshotBytes > 0 && int64(len(file.data)) > workspace.config.MaxSnapshotBytes-total {
			return snapshot{}, fmt.Errorf("snapshot exceeds %d bytes", workspace.config.MaxSnapshotBytes)
		}
		if workspace.config.MaxSnapshotBytes > 0 {
			total += int64(len(file.data))
		}
		result.files[name] = file
	}
	hash := sha256.New()
	for _, value := range []string{root, result.state.Head, result.state.Status} {
		fmt.Fprintf(hash, "%d:%s", len(value), value)
	}
	for _, name := range sortedNames(result.files) {
		file := result.files[name]
		if file == nil {
			fmt.Fprintf(hash, "%d:%s:missing:", len(name), name)
			continue
		}
		fmt.Fprintf(hash, "%d:%s:%d:%d:", len(name), name, file.mode, len(file.data))
		_, _ = hash.Write(file.data)
	}
	result.state.Fingerprint = fmt.Sprintf("%x", hash.Sum(nil))
	return result, nil
}

func validPath(name string) error {
	if !utf8.ValidString(name) || !fs.ValidPath(name) || strings.ContainsRune(name, 0) {
		return fmt.Errorf("%w: unsafe or non-UTF-8 workspace path %q", ErrUnsupported, name)
	}
	for _, component := range strings.Split(name, "/") {
		if strings.EqualFold(component, ".git") {
			return fmt.Errorf("%w: Git metadata is not a workspace file", ErrUnsupported)
		}
	}
	return nil
}

func readFile(root *os.Root, name string, limit int64) (*fileState, error) {
	// Never follow an ancestor symlink: even an in-tree alias would make
	// preservation and file attribution ambiguous.
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		info, err := root.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("non-directory path ancestor %q", parent)
		}
	}
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	file := &fileState{mode: info.Mode()}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := root.Readlink(name)
		if err != nil {
			return nil, err
		}
		file.data = []byte(target)
	} else if info.Mode().IsRegular() {
		if limit > 0 && info.Size() > limit {
			return nil, fmt.Errorf("file exceeds %d bytes", limit)
		}
		handle, err := root.OpenFile(name, os.O_RDONLY|snapshotReadFlags, 0)
		if err != nil {
			return nil, err
		}
		opened, err := handle.Stat()
		if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) || info.Mode() != opened.Mode() {
			_ = handle.Close()
			return nil, fmt.Errorf("file type changed during capture")
		}
		var reader io.Reader = handle
		if limit > 0 {
			reader = io.LimitReader(handle, limit+1)
		}
		file.data, err = io.ReadAll(reader)
		closeErr := handle.Close()
		if err != nil || closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
	} else {
		return nil, fmt.Errorf("%w: directories and special files are not snapshot files", ErrUnsupported)
	}
	if limit > 0 && int64(len(file.data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return file, nil
}

func sortedNames[T any](entries map[string]T) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sameFile(a, b *fileState) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.mode == b.mode && bytes.Equal(a.data, b.data)
}

func changedFiles(before, after map[string]*fileState) []string {
	names := make(map[string]bool, len(before)+len(after))
	for name := range before {
		names[name] = true
	}
	for name := range after {
		names[name] = true
	}
	changed := []string{}
	for _, name := range sortedNames(names) {
		if !sameFile(before[name], after[name]) {
			changed = append(changed, name)
		}
	}
	return changed
}
