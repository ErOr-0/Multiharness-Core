package codex

import (
	"context"
	"strings"
	"time"

	"multiharness-core/internal/adapter/process"
)

// RuntimeRunner resolves the default Codex executable before starting a task.
// It never installs software, changes shared caches, or retries an exec command.
// Explicit executable paths/names other than "codex" are operator-owned pins.
type RuntimeRunner struct {
	runner       ProcessRunner
	observe      func(string) error
	candidates   func(string) []string
	cacheVersion func() (string, error)
}

// observe receives only the selected release version, before any task starts.
func NewRuntimeRunner(runner ProcessRunner, observe func(string) error) *RuntimeRunner {
	return &RuntimeRunner{runner: runner, observe: observe, candidates: installedRuntimes, cacheVersion: cachedRuntimeVersion}
}

// RuntimeSelection describes local executable selection, not model availability
// or authentication. Version is empty for an explicitly pinned executable.
type RuntimeSelection struct {
	Executable string
	Version    string
}

// CompatibilityError contains only controlled diagnostics, never CLI output or
// cache contents. It is not a provider/billing error and cannot trigger fallback.
type CompatibilityError struct{ Code string }

func (e *CompatibilityError) Error() string {
	if e.Code == "notice_failed" {
		return "Codex runtime selection could not be reported; no task was sent."
	}
	if e.Code == "cache_unreadable" {
		return "Codex compatibility check: model-cache version could not be read safely; check the CLI cache permissions/format. No cache or credentials were changed."
	}
	return "Codex compatibility check: no compatible installed CLI was found. Update Codex or configure a compatible executable path; no task was sent and no software was installed."
}

// Resolve performs bounded, offline --version/exec --help probes. A cached
// catalog's writer version is a conservative compatibility floor: an older CLI
// may not understand newly introduced enum values even for an older model.
// Discovery and cache inspection are repeated, not retained as stale globals.
func (r *RuntimeRunner) Resolve(ctx context.Context, executable, workingDir string) (RuntimeSelection, error) {
	if ctx == nil {
		return RuntimeSelection{}, errNilContext
	}
	if err := ctx.Err(); err != nil {
		return RuntimeSelection{}, err
	}
	if r.runner == nil {
		return RuntimeSelection{}, errNilRunner
	}
	if executable != DefaultExecutable {
		return RuntimeSelection{Executable: executable}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	minimum, err := r.cacheVersion()
	if err != nil {
		return RuntimeSelection{}, &CompatibilityError{Code: "cache_unreadable"}
	}
	for _, candidate := range r.candidates(workingDir) {
		if err := ctx.Err(); err != nil {
			return RuntimeSelection{}, err
		}
		version, ok := r.probe(ctx, candidate, "--version")
		if !strings.HasPrefix(strings.TrimSpace(version), "codex-cli ") {
			continue
		}
		version = strings.TrimPrefix(strings.TrimSpace(version), "codex-cli ")
		if !ok || !compatibleVersion(version, minimum) {
			continue
		}
		help, ok := r.probe(ctx, candidate, "exec", "--help")
		if ok && supportsWorkflowFlags(help) {
			if err := ctx.Err(); err != nil {
				return RuntimeSelection{}, err
			}
			return RuntimeSelection{Executable: candidate, Version: version}, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return RuntimeSelection{}, err
	}
	return RuntimeSelection{}, &CompatibilityError{Code: "no_compatible_cli"}
}

func (r *RuntimeRunner) probe(ctx context.Context, executable string, args ...string) (string, bool) {
	result, err := r.runner.Run(ctx, process.Command{
		Name: executable, Args: args, Timeout: 2 * time.Second, OutputLimit: 64 << 10,
	})
	return result.Stdout, err == nil && result.ExitCode == 0 && !result.StdoutTruncated && !result.StderrTruncated
}

func (r *RuntimeRunner) Run(ctx context.Context, command process.Command) (process.Result, error) {
	if ctx == nil {
		return process.Result{ExitCode: -1}, errNilContext
	}
	if r.runner == nil {
		return process.Result{ExitCode: -1}, errNilRunner
	}
	if command.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, command.Timeout)
		defer cancel()
	}
	selection, err := r.Resolve(ctx, command.Name, command.Dir)
	if err != nil {
		return process.Result{ExitCode: -1}, err
	}
	if r.observe != nil && selection.Version != "" {
		if err := r.observe(selection.Version); err != nil {
			return process.Result{ExitCode: -1}, &CompatibilityError{Code: "notice_failed"}
		}
	}
	if err := ctx.Err(); err != nil {
		return process.Result{ExitCode: -1}, err
	}
	command.Name = selection.Executable
	// Preserve argv, stdin, environment, streams, permissions and timeout. There
	// is exactly one task invocation, including for implementation and repair.
	return r.runner.Run(ctx, command)
}

func supportsWorkflowFlags(help string) bool {
	for _, flag := range []string{"--model", "--sandbox", "--ephemeral", "--json", "--color", "--cd", "--config", "--output-schema", "--output-last-message", "--skip-git-repo-check"} {
		found := false
		for _, token := range strings.Fields(help) {
			if token == flag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
