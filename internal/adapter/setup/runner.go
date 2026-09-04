// Package setup handles missing local dependencies outside workflow policy.
// Installation is terminal-consented, bounded, and never replays an agent call.
package setup

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"time"

	"multiharness-core/internal/adapter/process"
)

type ProcessRunner interface {
	Run(context.Context, process.Command) (process.Result, error)
}

// Request contains only application-owned strings, never task or provider text.
type Request struct {
	Tool    string
	Command string
}

type Confirmation func(context.Context, Request) (bool, error)

// Manager is shared by one run's dependency adapters. It has no global state.
type Manager struct {
	Confirm Confirmation
	Timeout time.Duration
	Runner  ProcessRunner
	lookup  func(string) (string, error)
	lock    func() (io.Closer, error)
}

func NewManager(runner ProcessRunner, confirm Confirmation, timeout time.Duration) *Manager {
	return &Manager{Runner: runner, Confirm: confirm, Timeout: timeout, lookup: exec.LookPath, lock: installationLock}
}

// Error exposes controlled setup diagnostics. It is deliberately not a provider
// failure: installation failures must not trigger retries or billing fallback.
type Error struct {
	Tool string
	Code string
}

func (e *Error) Error() string {
	tool := knownTool(e.Tool)
	switch e.Code {
	case "installed":
		return tool + " installation completed. " + guidance(e.Tool) + " No task was replayed; rerun the workflow after setup."
	case "install_failed":
		return tool + " installation failed (network, package-manager, disk or permissions error). Check the package manager manually; no elevation or automatic retry was attempted. " + guidance(e.Tool)
	case "install_timeout":
		return tool + " installation timed out. A partial installation may remain; inspect it before retrying. " + guidance(e.Tool)
	case "not_on_path":
		return tool + " installer completed but the CLI is not executable on PATH. Check the installation and restart your terminal. No task was replayed. " + guidance(e.Tool)
	case "install_busy":
		return tool + " installation could not acquire the private setup lock. Another installation may be running, or the cache permissions need attention. No installation was started."
	case "no_installer":
		return tool + " is missing and a safe npm executable was not found. Install Node.js/npm yourself or use the documented OS package manager; Multiharness never bootstraps package managers. " + guidance(e.Tool)
	case "confirmation_failed":
		return tool + " installation confirmation failed; no installation was started."
	case "pinned":
		return "A configured " + tool + " executable is unavailable. Fix the configured executable path and its permissions/interpreter; explicit pins are never replaced or installed automatically. " + guidance(e.Tool)
	default:
		return tool + " is not installed or is unavailable on PATH. " + guidance(e.Tool) + " Install it manually or rerun from an interactive terminal with --install-mode prompt."
	}
}

func knownTool(tool string) string {
	switch tool {
	case "codex":
		return "Codex"
	case "opencode":
		return "OpenCode"
	case "git":
		return "Git"
	default:
		return "Dependency"
	}
}

func guidance(tool string) string {
	switch tool {
	case "codex":
		return "Install: npm install -g @openai/codex (macOS alternative: brew install --cask codex). Then run codex to sign in. https://learn.chatgpt.com/docs/codex/cli"
	case "opencode":
		return "Install: npm install -g opencode-ai (macOS/Linux alternative: brew install anomalyco/tap/opencode). Then run opencode and /connect to configure your chosen provider/model. https://opencode.ai/docs/"
	case "git":
		return "Install Git using your OS package manager (macOS: xcode-select --install), then verify git --version. https://git-scm.com/downloads"
	default:
		return "Check the configured executable and operating-system requirements."
	}
}

// Runner delegates normally and intercepts only a proven pre-start missing
// executable. It never retries a started process, including exit 127 or errors
// printed by a provider. Codex supplies its offline-discovery missing marker.
type Runner struct {
	Runner  ProcessRunner
	Manager *Manager
	Tool    string
}

func (r Runner) Run(ctx context.Context, command process.Command) (process.Result, error) {
	if ctx == nil || r.Runner == nil {
		return process.Result{ExitCode: -1}, errors.New("dependency runner requires context and process runner")
	}
	if err := ctx.Err(); err != nil {
		return process.Result{ExitCode: -1}, err
	}
	if command.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, command.Timeout)
		defer cancel()
	}
	result, err := r.Runner.Run(ctx, command)
	var missing interface{ MissingExecutable() bool }
	var start *process.RunError
	isMissing := errors.As(err, &missing) && missing.MissingExecutable()
	isMissing = isMissing || (errors.As(err, &start) && start.Kind == process.ErrorKindExecutableNotFound)
	if !isMissing {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if command.Name != r.Tool {
		return result, &Error{Tool: r.Tool, Code: "pinned"}
	}
	if r.Manager == nil {
		return result, &Error{Tool: r.Tool, Code: "missing"}
	}
	return result, r.Manager.handle(ctx, r.Tool, command.Dir)
}
