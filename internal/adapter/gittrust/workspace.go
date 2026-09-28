// Package gittrust prepares Git access for an explicitly selected container
// workspace without changing repository files or ownership.
package gittrust

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"multiharness-core/internal/adapter/process"
)

// Prepare registers only the selected mounted repository in the app user's
// persistent Git configuration, before native agents enter their sandboxes.
func Prepare(ctx context.Context, mount, selected string) error {
	if mount == "" {
		return nil // Host invocations retain their Git trust policy.
	}
	root, err := filepath.EvalSymlinks(mount)
	if err != nil {
		return fmt.Errorf("resolve mounted workspace: %w", err)
	}
	dir, err := filepath.EvalSymlinks(selected)
	if err != nil {
		return fmt.Errorf("resolve selected workspace: %w", err)
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(dir) {
		return errors.New("Git workspace trust requires absolute paths")
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("Git workspace trust must stay inside the mounted folder")
	}
	// Git discovers parent repositories when the selected folder is src/.
	// Inspect only ancestors within the mount, never scan child projects.
	for {
		info, statErr := os.Lstat(filepath.Join(dir, ".git"))
		if statErr == nil {
			if !info.IsDir() && !info.Mode().IsRegular() {
				return errors.New("Git metadata must be a directory or worktree file")
			}
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect selected repository: %w", statErr)
		}
		if dir == root {
			return nil // Plain folders do not require Git.
		}
		dir = filepath.Dir(dir)
	}
	if strings.Contains(dir, "*") {
		return errors.New("cannot register a Git trust path containing a wildcard")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	runner := process.NewOSRunner()
	command := process.Command{Name: "git", Dir: home, Timeout: 10 * time.Second, Args: []string{"config", "--global", "--fixed-value", "--get-all", "safe.directory", filepath.ToSlash(dir)}}
	result, err := runner.Run(ctx, command)
	if err == nil {
		return nil
	}
	if result.ExitCode != 1 {
		return fmt.Errorf("read container Git trust: %w", err)
	}
	command.Args = []string{"config", "--global", "--add", "safe.directory", filepath.ToSlash(dir)}
	if _, err := runner.Run(ctx, command); err != nil {
		return fmt.Errorf("register selected repository in container Git trust: %w", err)
	}
	return nil
}
