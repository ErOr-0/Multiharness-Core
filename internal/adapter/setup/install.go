package setup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"multiharness-core/internal/adapter/process"
)

// Fixed package identities and versions are release-owned, not task/config
// input. No curl-to-shell, sudo, package-manager bootstrap or latest upgrade.
func packageName(tool string) string {
	switch tool {
	case "codex":
		return "@openai/codex@0.153.0"
	case "opencode":
		return "opencode-ai@1.18.23"
	default:
		return ""
	}
}

func (m *Manager) handle(ctx context.Context, tool, workdir string) error {
	failure := func(code string) error { return &Error{Tool: tool, Code: code} }
	if m.Confirm == nil || m.Runner == nil || m.lookup == nil || m.lock == nil || m.Timeout <= 0 || m.Timeout > 30*time.Minute || packageName(tool) == "" {
		return failure("missing")
	}
	// Native Windows needs a cancellable console/process-tree implementation
	// before executing unattended installers. Give manual/WSL guidance instead.
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return failure("missing")
	}
	// Distinguish truly absent CLIs from broken interpreters, permissions, and
	// repository-local PATH entries. Never overwrite an existing installation.
	if _, err := m.lookup(tool); !errors.Is(err, exec.ErrNotFound) {
		return failure("pinned")
	}
	npm, err := m.lookup("npm")
	if err != nil || !trustedExecutable(npm, workdir) {
		return failure("no_installer")
	}
	args := []string{"install", "--global", "--registry=https://registry.npmjs.org", "--no-audit", "--no-fund", packageName(tool)}
	yes, err := m.Confirm(ctx, Request{Tool: tool, Command: "npm " + strings.Join(args, " ")})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return failure("confirmation_failed")
	}
	if !yes {
		return failure("missing")
	}
	lease, err := m.lock()
	if err != nil || lease == nil {
		return failure("install_busy")
	}
	defer lease.Close()
	// A second process may have installed it while the user was deciding.
	if _, err := m.lookup(tool); !errors.Is(err, exec.ErrNotFound) {
		return failure("pinned")
	}
	if !trustedExecutable(npm, workdir) {
		return failure("install_failed")
	}
	// Never load npm project configuration or write package files in the target.
	tempRoot, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil || !outsideWorkspace(tempRoot, workdir) {
		return failure("install_failed")
	}
	dir, err := os.MkdirTemp(tempRoot, "multiharness-install-")
	if err != nil {
		return failure("install_failed")
	}
	defer os.RemoveAll(dir)
	installCtx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	result, err := m.Runner.Run(installCtx, process.Command{
		Name: npm, Args: args, Dir: dir, Timeout: m.Timeout, OutputLimit: 32 << 10,
		// No task stdin, output callbacks or provider-specific child environment
		// is inherited from the unavailable agent command.
		EnvOverrides: map[string]string{"CI": "1", "NPM_CONFIG_UPDATE_NOTIFIER": "false"},
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var runErr *process.RunError
	if errors.Is(installCtx.Err(), context.DeadlineExceeded) || (errors.As(err, &runErr) && runErr.Kind == process.ErrorKindTimeout) {
		return failure("install_timeout")
	}
	if err != nil || result.ExitCode != 0 {
		return failure("install_failed")
	}
	installed, err := m.lookup(tool)
	if err != nil || !trustedExecutable(installed, workdir) {
		return failure("not_on_path")
	}
	// Authentication and protocol compatibility remain separate gates. In
	// particular, an installer exit zero is never workflow approval.
	return failure("installed")
}

func trustedExecutable(path, workdir string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	for _, candidate := range []string{path, resolved} {
		if !outsideWorkspace(candidate, workdir) {
			return false
		}
	}
	info, err := os.Stat(resolved)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 && info.Mode().Perm()&0002 == 0
}

func outsideWorkspace(path, workdir string) bool {
	root, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
