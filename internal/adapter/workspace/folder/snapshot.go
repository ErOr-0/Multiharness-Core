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
	"sync"
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
	parent := ctx
	ctx, watch := startScan(ctx, workspace.config.Timeout, workspace.config.Observe)
	defer watch.close()
	watch.nextPass(1)
	scanAdvanced(ctx, "listing files", 0)
	first, err := workspace.capture(ctx, root, baseline)
	if err != nil {
		return snapshot{}, watch.failure(root, parent, err)
	}
	watch.nextPass(2)
	scanAdvanced(ctx, "listing files", 0)
	second, err := workspace.capture(ctx, root, first.files)
	if err != nil {
		return snapshot{}, watch.failure(root, parent, err)
	}
	if err := ctx.Err(); err != nil {
		return snapshot{}, watch.failure(root, parent, err)
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
	scanAdvanced(ctx, "reading files", 0)
	ordered := sortedNames(names)
	var directory *os.Root
	lastDir := ""
	defer func() {
		if directory != nil {
			_ = directory.Close()
		}
	}()
	for offset := 0; offset < len(ordered); {
		if err := ctx.Err(); err != nil {
			return snapshot{}, err
		}
		dir := path.Dir(ordered[offset])
		if dir != lastDir {
			if directory != nil {
				_ = directory.Close()
				directory = nil
			}
			directory, err = openSnapshotDirectory(rootFS, dir)
			if err != nil {
				return snapshot{}, fmt.Errorf("snapshot directory %q: %w", dir, err)
			}
			lastDir = dir
		}
		end := offset + 1
		for end < len(ordered) && end < offset+8 && path.Dir(ordered[end]) == dir {
			end++
		}
		batch := ordered[offset:end]
		offset = end
		files := make([]*fileState, len(batch))
		errorsByFile := make([]error, len(batch))
		var readers sync.WaitGroup
		for i, name := range batch {
			readers.Add(1)
			go func() {
				defer readers.Done()
				if err := ctx.Err(); err != nil {
					errorsByFile[i] = err
					return
				}
				if err := validPath(name); err != nil {
					errorsByFile[i] = err
					return
				}
				if directory != nil {
					files[i], errorsByFile[i] = readFile(directory, path.Base(name), workspace.config.MaxFileBytes)
				}
				if errorsByFile[i] == nil {
					if previous, exists := baseline[name]; exists && sameFile(previous, files[i]) {
						files[i] = previous
					}
					scanAdvanced(ctx, "reading files", 1)
				}
			}()
		}
		readers.Wait()
		for i, name := range batch {
			if err := validPath(name); err != nil {
				return snapshot{}, err
			}
			file, err := files[i], errorsByFile[i]
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
	}
	hash := sha256.New()
	for _, value := range []string{root, result.state.Head, result.state.Status} {
		fmt.Fprintf(hash, "%d:%s", len(value), value)
	}
	for _, name := range sortedNames(result.files) {
		if err := ctx.Err(); err != nil {
			return snapshot{}, err
		}
		file := result.files[name]
		if file == nil {
			fmt.Fprintf(hash, "%d:%s:missing:", len(name), name)
			continue
		}
		fmt.Fprintf(hash, "%d:%s:%d:%d:", len(name), name, file.mode, len(file.data))
		_, _ = hash.Write(file.data)
		scanAdvanced(ctx, "hashing files", 1)
	}
	result.state.Fingerprint = fmt.Sprintf("%x", hash.Sum(nil))
	return result, nil
}

// Retain a verified directory handle while reading its files. This avoids
// rewalking every ancestor for every file on a slow bind mount, while rejecting
// ancestor symlinks and retaining os.Root's escape protection.
func openSnapshotDirectory(root *os.Root, dir string) (*os.Root, error) {
	var expected os.FileInfo
	for parent := dir; parent != "."; parent = path.Dir(parent) {
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
		if parent == dir {
			expected = info
		}
	}
	handle, err := root.OpenRoot(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expected != nil {
		opened, err := handle.Stat(".")
		if err != nil || !os.SameFile(expected, opened) {
			_ = handle.Close()
			return nil, fmt.Errorf("directory changed during capture")
		}
	}
	return handle, nil
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
