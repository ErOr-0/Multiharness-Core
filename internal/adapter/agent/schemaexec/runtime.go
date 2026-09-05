package schemaexec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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

// MissingExecutable is true only when offline discovery found no installed
// candidate and PATH reports absence, never for an incompatible/broken CLI.
func (e *CompatibilityError) MissingExecutable() bool { return e.Code == "missing_cli" }

func (e *CompatibilityError) Error() string {
	if e.Code == "missing_cli" {
		return "Codex CLI was not found. Install Codex and sign in before rerunning; no task was sent."
	}
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
	candidates := r.candidates(workingDir)
	for _, candidate := range candidates {
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
	if len(candidates) == 0 {
		if _, err := exec.LookPath(DefaultExecutable); errors.Is(err, exec.ErrNotFound) {
			return RuntimeSelection{}, &CompatibilityError{Code: "missing_cli"}
		}
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
	for _, flag := range []string{
		"--model",
		"--sandbox",
		"--ephemeral",
		"--json",
		"--color",
		"--cd",
		"--config",
		"--output-schema",
		"--output-last-message",
		"--skip-git-repo-check",
	} {
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

func installedRuntimes(workingDir string) []string {
	paths := []string{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		// Never implicitly execute a repository-local or relative PATH candidate.
		if filepath.IsAbs(dir) {
			paths = append(paths, filepath.Join(dir, DefaultExecutable))
			if runtime.GOOS == "windows" {
				paths = append(paths, filepath.Join(dir, DefaultExecutable+".exe"))
			}
		}
	}
	if runtime.GOOS == "darwin" {
		roots := []string{"/Applications"}
		if home, err := os.UserHomeDir(); err == nil {
			roots = append(roots, filepath.Join(home, "Applications"))
		}
		for _, root := range roots {
			for _, app := range []string{"Codex.app", "ChatGPT.app"} {
				paths = append(paths, filepath.Join(root, app, "Contents", "Resources", "codex"))
			}
		}
	}
	return eligibleRuntimes(paths, workingDir)
}

func eligibleRuntimes(paths []string, workingDir string) []string {
	root, err := filepath.EvalSymlinks(workingDir)
	if err != nil {
		return nil
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil
	}
	seen, result := map[string]bool{}, []string{}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || seen[resolved] {
			continue
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil || insideDirectory(root, parent) || insideDirectory(root, resolved) {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0002 != 0 {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
			continue
		}
		seen[resolved] = true
		result = append(result, resolved)
		if len(result) == 8 {
			break
		}
	}
	return result
}

func insideDirectory(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func cachedRuntimeVersion() (string, error) {
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".codex")
	}
	return readRuntimeCache(filepath.Join(dir, "models_cache.json"))
}

func readRuntimeCache(path string) (string, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	const limit = 8 << 20
	if !info.Mode().IsRegular() || info.Size() > limit {
		return "", errors.New("invalid cache file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return "", errors.New("cache changed during inspection")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) > limit {
		return "", errors.New("could not read bounded cache")
	}
	var cache struct {
		ClientVersion string `json:"client_version"`
	}
	if json.Unmarshal(data, &cache) != nil {
		return "", errors.New("invalid cache metadata")
	}
	if _, ok := releaseVersion(cache.ClientVersion); !ok {
		return "", errors.New("unrecognized cache writer version")
	}
	return cache.ClientVersion, nil
}

// Only stable release versions are auto-selected. Explicit executable pins can
// opt into experimental builds without guessing their compatibility ordering.
func releaseVersion(value string) ([3]uint64, bool) {
	var version [3]uint64
	parts := strings.Split(value, ".")
	if len(parts) != len(version) {
		return version, false
	}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version, false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return version, false
			}
		}
		var err error
		version[i], err = strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version, false
		}
	}
	return version, true
}

func compatibleVersion(candidate, minimum string) bool {
	version, ok := releaseVersion(candidate)
	if !ok {
		return false
	}
	if minimum == "" {
		return true
	}
	floor, ok := releaseVersion(minimum)
	if !ok {
		return false
	}
	for i := range version {
		if version[i] != floor[i] {
			return version[i] > floor[i]
		}
	}
	return true
}
