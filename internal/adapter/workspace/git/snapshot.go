package git

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
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"multiharness-core/internal/store"
)

type fileState struct {
	data []byte
	mode os.FileMode
}
type snapshot struct {
	state store.RepositoryState
	files map[string]*fileState
	index string
	ref   string
	dirty []string
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
	result := snapshot{files: make(map[string]*fileState), state: store.RepositoryState{Root: root}}
	head, err := workspace.command(ctx, root, true, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return snapshot{}, err
	}
	result.state.Head = strings.TrimSpace(head)
	result.ref, err = workspace.command(ctx, root, true, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return snapshot{}, err
	}
	result.index, err = workspace.command(ctx, root, false, "ls-files", "--stage", "-z")
	if err != nil {
		return snapshot{}, err
	}
	names := make(map[string]bool)
	for _, entry := range nulFields(result.index) {
		fields, name, ok := strings.Cut(entry, "\t")
		parts := strings.Fields(fields)
		if !ok || len(parts) != 3 {
			return snapshot{}, fmt.Errorf("invalid Git index record")
		}
		if parts[0] == "160000" || parts[2] != "0" {
			return snapshot{}, fmt.Errorf("%w: submodules and unmerged index entries are not supported", ErrUnsupported)
		}
		names[name] = true
	}
	flags, err := workspace.command(ctx, root, false, "ls-files", "-v", "-z")
	if err != nil {
		return snapshot{}, err
	}
	for _, entry := range nulFields(flags) {
		if len(entry) < 2 || entry[0] == 'S' || unicode.IsLower(rune(entry[0])) {
			return snapshot{}, fmt.Errorf("%w: sparse/skip-worktree and assume-unchanged entries are not supported", ErrUnsupported)
		}
	}
	untracked, err := workspace.command(ctx, root, false, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return snapshot{}, err
	}
	for _, name := range nulFields(untracked) {
		names[name] = true
	}
	for name := range baseline {
		names[name] = true
	} // A changed ignore rule must not hide baseline files.
	status, err := workspace.command(ctx, root, false, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames", "--ignore-submodules=none")
	if err != nil {
		return snapshot{}, err
	}
	var readable strings.Builder
	for _, entry := range nulFields(status) {
		if len(entry) < 4 || entry[2] != ' ' {
			return snapshot{}, fmt.Errorf("invalid Git status record")
		}
		code, name := entry[:2], entry[3:]
		if strings.Contains(code, "U") || code == "AA" || code == "DD" {
			return snapshot{}, fmt.Errorf("%w: unresolved merge", ErrUnsupported)
		}
		if err := validPath(name); err != nil {
			return snapshot{}, err
		}
		result.dirty = append(result.dirty, name)
		names[name] = true
		fmt.Fprintf(&readable, "%s %s\n", code, strconv.Quote(name))
	}
	sort.Strings(result.dirty)
	result.state.Status = readable.String()
	if len(names) > workspace.config.MaxFiles {
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
		if int64(len(file.data)) > workspace.config.MaxSnapshotBytes-total {
			return snapshot{}, fmt.Errorf("snapshot exceeds %d bytes", workspace.config.MaxSnapshotBytes)
		}
		total += int64(len(file.data))
		result.files[name] = file
	}
	hash := sha256.New()
	for _, value := range []string{root, head, result.ref, result.index, status} {
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
		return fmt.Errorf("%w: unsafe or non-UTF-8 repository path %q", ErrUnsupported, name)
	}
	for _, component := range strings.Split(name, "/") {
		if strings.EqualFold(component, ".git") {
			return fmt.Errorf("%w: nested repository metadata", ErrUnsupported)
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
		if info.Size() > limit {
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
		file.data, err = io.ReadAll(io.LimitReader(handle, limit+1))
		closeErr := handle.Close()
		if err != nil || closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		if int64(len(file.data)) > limit {
			return nil, fmt.Errorf("file exceeds %d bytes", limit)
		}
	} else {
		return nil, fmt.Errorf("%w: directories, nested repositories, and special files are not snapshot files", ErrUnsupported)
	}
	if int64(len(file.data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return file, nil
}

func nulFields(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(value, "\x00"), "\x00")
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
