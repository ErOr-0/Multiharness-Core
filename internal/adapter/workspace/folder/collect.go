package folder

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"

	ignore "github.com/sabhiram/go-gitignore"
	"multiharness-core/internal/store"
)

type ignoreRule struct {
	base    string
	pattern *ignore.GitIgnore
	negate  bool
}

// Rules are local to the selected folder. No parent repository, global Git
// configuration, index, or external executable participates in file selection.
func (w *Workspace) collect(ctx context.Context, root string) (snapshot, map[string]bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := snapshot{state: store.RepositoryState{Root: root}, files: map[string]*fileState{}}
	names := map[string]bool{}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return result, nil, err
	}
	defer handle.Close()
	// Bound concurrent directory IO on slow bind mounts. Each subtree retains
	// its own inherited ignore rules; no directory is omitted for performance.
	var mu sync.Mutex
	var workers sync.WaitGroup
	slots := make(chan struct{}, 7)
	var firstErr error
	fail := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		mu.Unlock()
	}
	var visit func(string, []ignoreRule) error
	visit = func(dir string, inherited []ignoreRule) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		rules := append([]ignoreRule{}, inherited...)
		for _, filename := range []string{".gitignore", ".magentignore"} {
			name := path.Join(dir, filename)
			file, err := readFile(handle, name, 1<<20)
			if err != nil {
				return fmt.Errorf("read folder exclusions %q: %w", name, err)
			}
			if file == nil || file.mode&os.ModeSymlink != 0 {
				continue
			}
			for _, line := range strings.Split(string(file.data), "\n") {
				line = strings.TrimSuffix(line, "\r")
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				negate := strings.HasPrefix(line, "!")
				if negate {
					line = strings.TrimPrefix(line, "!")
				}
				rules = append(rules, ignoreRule{base: dir, pattern: ignore.CompileIgnoreLines(line), negate: negate})
			}
		}
		// Open through os.Root so a raced directory symlink cannot escape the target.
		entries, err := fs.ReadDir(handle.FS(), dir)
		if err != nil {
			return err
		}
		scanAdvanced(ctx, "listing files", 0)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			scanAdvanced(ctx, "listing files", 0)
			name := path.Join(dir, entry.Name())
			switch strings.ToLower(entry.Name()) {
			case ".git", ".hg", ".svn":
				continue
			}
			ignored := false
			candidate := name
			if entry.IsDir() {
				candidate += "/"
			}
			// Last matching rule wins; stop at that match instead of evaluating
			// every earlier rule for each entry in a large workspace.
			for i := len(rules) - 1; i >= 0; i-- {
				rule := rules[i]
				relative := candidate
				if rule.base != "." {
					relative = strings.TrimPrefix(candidate, rule.base+"/")
				}
				if rule.pattern.MatchesPath(relative) {
					ignored = !rule.negate
					break
				}
			}
			if ignored {
				continue
			}
			if entry.IsDir() {
				select {
				case slots <- struct{}{}:
					workers.Add(1)
					go func(name string, rules []ignoreRule) {
						defer workers.Done()
						defer func() { <-slots }()
						fail(visit(name, rules))
					}(name, rules)
				default:
					if err := visit(name, rules); err != nil {
						return err
					}
				}
			} else {
				if err := validPath(name); err != nil {
					return err
				}
				mu.Lock()
				names[name] = true
				count := len(names)
				mu.Unlock()
				scanAdvanced(ctx, "listing files", 1)
				if w.config.MaxFiles > 0 && count > w.config.MaxFiles {
					return fmt.Errorf("snapshot exceeds %d files", w.config.MaxFiles)
				}
			}
		}
		return nil
	}
	fail(visit(".", nil))
	workers.Wait()
	if firstErr != nil {
		return result, names, fmt.Errorf("enumerating workspace %q after %d files: %w", root, len(names), firstErr)
	}
	result.existingFiles = sortedNames(names) // Existing files, independent of VCS status.
	return result, names, nil
}
