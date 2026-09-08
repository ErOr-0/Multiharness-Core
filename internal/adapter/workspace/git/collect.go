package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"multiharness-core/internal/store"
)

// Metadata is kept separately from editable files, including for repositories
// below the selected folder. No index or HEAD is ever changed by inspection.
type repositoryMetadata struct {
	Common string `json:"common_directory"`
	GitDir string `json:"git_directory"`
	Marker string `json:"git_marker"`
	Head   string `json:"head"`
	Ref    string `json:"head_ref"`
	Index  string `json:"index_entries"`
}

type collector struct {
	workspace     *Workspace
	ctx           context.Context
	root          string
	result        snapshot
	names         map[string]bool
	dirty         map[string]bool
	status        map[string]string
	checked       map[string]bool
	metadataBytes int
}

func (workspace *Workspace) collect(ctx context.Context, root string) (snapshot, map[string]bool, error) {
	c := &collector{
		workspace: workspace, ctx: ctx, root: root,
		result: snapshot{state: store.RepositoryState{Root: root}, files: map[string]*fileState{}, repositories: map[string]repositoryMetadata{}},
		names:  map[string]bool{}, dirty: map[string]bool{}, status: map[string]string{}, checked: map[string]bool{},
	}
	repo, err := enclosingRepository(root)
	if err != nil {
		return snapshot{}, nil, err
	}
	if repo == "" {
		err = c.plainFolder()
	} else {
		err = c.repository(repo)
	}
	if err != nil {
		return snapshot{}, nil, err
	}
	c.result.dirty = sortedNames(c.dirty)
	var status strings.Builder
	for _, name := range sortedNames(c.status) {
		fmt.Fprintf(&status, "%s %s\n", c.status[name], strconv.Quote(name))
	}
	c.result.state.Status = status.String()
	// Preserve the existing single-root HEAD field. A folder has no single HEAD;
	// every discovered repository still contributes to the overall fingerprint.
	if repo, ok := c.result.repositories[root]; ok {
		c.result.state.Head = repo.Head
	}
	return c.result, c.names, nil
}

func enclosingRepository(dir string) (string, error) {
	for {
		info, err := os.Lstat(filepath.Join(dir, ".git"))
		if err == nil {
			if !info.IsDir() && !info.Mode().IsRegular() {
				return "", fmt.Errorf("%w: .git must be a directory or a regular worktree marker", ErrUnsupported)
			}
			return dir, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if filepath.Dir(dir) == dir {
			return "", nil
		}
		dir = filepath.Dir(dir)
	}
}

// Git supplies its .gitignore semantics even without a user repository. Its
// empty index lives in a private temporary directory, never in the workspace.
// This also discovers nested repositories without traversing their .git data.
func (c *collector) plainFolder() error {
	directory, err := os.MkdirTemp("", "multiharness-file-list-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	if _, err := c.workspace.command(c.ctx, directory, false, "init", "--bare", "--quiet", "--template=", directory); err != nil {
		return err
	}
	listing, err := c.workspace.command(c.ctx, c.root, false,
		"--git-dir="+directory, "--work-tree="+c.root,
		"ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return err
	}
	for _, name := range nulFields(listing) {
		if _, err := c.add(c.root, name); err != nil {
			return err
		}
	}
	return nil
}

func (c *collector) repository(root string) error {
	if _, exists := c.result.repositories[root]; exists {
		return nil
	}
	if len(c.result.repositories) >= c.workspace.config.MaxFiles {
		return fmt.Errorf("too many repositories in workspace")
	}
	command := func(allowOne bool, args ...string) (string, error) {
		return c.workspace.command(c.ctx, root, allowOne, args...)
	}
	toplevel, err := command(false, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("inspect repository %q (its Git metadata must be accessible): %w", root, err)
	}
	resolved, err := filepath.EvalSymlinks(strings.TrimSuffix(toplevel, "\n"))
	if err != nil || resolved != root {
		return fmt.Errorf("%w: repository metadata redirects outside %q", ErrUnsupported, root)
	}
	var repo repositoryMetadata
	for _, field := range []struct {
		target *string
		args   []string
		one    bool
	}{
		{&repo.Common, []string{"rev-parse", "--path-format=absolute", "--git-common-dir"}, false},
		{&repo.GitDir, []string{"rev-parse", "--absolute-git-dir"}, false},
		{&repo.Head, []string{"rev-parse", "--verify", "--quiet", "HEAD"}, true},
		{&repo.Ref, []string{"symbolic-ref", "--quiet", "HEAD"}, true},
		{&repo.Index, []string{"ls-files", "--stage", "-z"}, false},
	} {
		value, err := command(field.one, field.args...)
		if err != nil {
			return err
		}
		if len(value) > c.workspace.config.MaxOutputBytes-c.metadataBytes {
			return fmt.Errorf("combined Git metadata exceeds %d bytes", c.workspace.config.MaxOutputBytes)
		}
		c.metadataBytes += len(value)
		*field.target = strings.TrimSuffix(value, "\n")
	}
	for _, value := range []*string{&repo.Common, &repo.GitDir} {
		*value, err = filepath.EvalSymlinks(*value)
		if err != nil {
			return err
		}
	}
	marker, err := os.Lstat(filepath.Join(root, ".git"))
	if err != nil {
		return err
	}
	if marker.Mode().IsRegular() {
		fsRoot, err := os.OpenRoot(root)
		if err != nil {
			return err
		}
		file, err := readFile(fsRoot, ".git", c.workspace.config.MaxFileBytes)
		_ = fsRoot.Close()
		if err != nil || file == nil {
			return fmt.Errorf("read Git worktree marker: %w", errors.Join(err, ErrChangedDuringCapture))
		}
		repo.Marker = string(file.data)
	} else if !marker.IsDir() {
		return fmt.Errorf("%w: invalid Git metadata marker", ErrUnsupported)
	}
	c.result.repositories[root] = repo
	for _, entry := range nulFields(repo.Index) {
		fields, name, ok := strings.Cut(entry, "\t")
		parts := strings.Fields(fields)
		if !ok || len(parts) != 3 {
			return fmt.Errorf("invalid Git index record")
		}
		if parts[0] == "160000" || parts[2] != "0" {
			return fmt.Errorf("%w: submodules and unmerged index entries are not supported", ErrUnsupported)
		}
		if _, err := c.add(root, name); err != nil {
			return err
		}
	}
	flags, err := command(false, "ls-files", "-v", "-z")
	if err != nil {
		return err
	}
	for _, entry := range nulFields(flags) {
		if len(entry) < 2 || entry[0] == 'S' || unicode.IsLower(rune(entry[0])) {
			return fmt.Errorf("%w: sparse/skip-worktree and assume-unchanged entries are not supported", ErrUnsupported)
		}
	}
	untracked, err := command(false, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return err
	}
	for _, name := range nulFields(untracked) {
		if _, err := c.add(root, name); err != nil {
			return err
		}
	}
	status, err := command(false, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames", "--ignore-submodules=none")
	if err != nil {
		return err
	}
	for _, entry := range nulFields(status) {
		if len(entry) < 4 || entry[2] != ' ' {
			return fmt.Errorf("invalid Git status record")
		}
		code, name := entry[:2], entry[3:]
		if strings.Contains(code, "U") || code == "AA" || code == "DD" {
			return fmt.Errorf("%w: unresolved merge", ErrUnsupported)
		}
		rel, err := c.add(root, name)
		if err != nil {
			return err
		}
		if rel != "" {
			c.dirty[rel] = true
			c.status[rel] = code
		}
	}
	return nil
}

// add returns a workspace-relative file name, or an empty string for a nested
// repository or an out-of-scope path. A directory entry from ls-files is a nested
// repository boundary; its own index/status determines protected files.
func (c *collector) add(base, name string) (string, error) {
	if err := c.ctx.Err(); err != nil {
		return "", err
	}
	directory := strings.HasSuffix(name, "/")
	name = strings.TrimSuffix(name, "/")
	if err := validPath(name); err != nil {
		return "", err
	}
	abs := filepath.Join(base, filepath.FromSlash(name))
	rel, err := filepath.Rel(c.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	// A nested repository can contain files also tracked by its parent. Git may
	// then omit an untracked directory entry, so check file ancestors too.
	for parent := filepath.Dir(abs); parent != base && parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		if c.checked[parent] {
			break
		}
		c.checked[parent] = true
		if parent != c.root && !strings.HasPrefix(parent, c.root+string(filepath.Separator)) {
			break
		}
		marker, err := os.Lstat(filepath.Join(parent, ".git"))
		if err == nil {
			if !marker.IsDir() && !marker.Mode().IsRegular() {
				return "", fmt.Errorf("%w: invalid nested Git metadata", ErrUnsupported)
			}
			resolved, err := filepath.EvalSymlinks(parent)
			if err != nil || resolved != parent {
				return "", fmt.Errorf("%w: nested repository path is not a stable directory", ErrUnsupported)
			}
			if err := c.repository(parent); err != nil {
				return "", err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	if directory {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil || resolved != abs {
			return "", fmt.Errorf("%w: nested repository path is not a stable directory", ErrUnsupported)
		}
		return "", c.repository(abs)
	}
	c.names[rel] = true
	if len(c.names) > c.workspace.config.MaxFiles {
		return "", fmt.Errorf("snapshot exceeds %d files", c.workspace.config.MaxFiles)
	}
	return rel, nil
}

func repositoryViolations(before, after snapshot) []string {
	violations := []string{}
	roots := map[string]bool{}
	for root := range before.repositories {
		roots[root] = true
	}
	for root := range after.repositories {
		roots[root] = true
	}
	for _, root := range sortedNames(roots) {
		a, had := before.repositories[root]
		b, has := after.repositories[root]
		label, _ := filepath.Rel(before.state.Root, root)
		if had != has || a.Common != b.Common || a.GitDir != b.GitDir || a.Marker != b.Marker {
			violations = append(violations, "[Git metadata: "+filepath.ToSlash(label)+"]")
		}
		if had && has && a.Index != b.Index {
			violations = append(violations, "[Git index: "+filepath.ToSlash(label)+"]")
		}
		if had && has && (a.Head != b.Head || a.Ref != b.Ref) {
			violations = append(violations, "[Git HEAD: "+filepath.ToSlash(label)+"]")
		}
	}
	return violations
}
