package setup

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
)

type runFunc func(context.Context, process.Command) (process.Result, error)

func (f runFunc) Run(ctx context.Context, c process.Command) (process.Result, error) {
	return f(ctx, c)
}

func TestSetupNeverReplaysAnAgent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cause     error
		command   string
		wantSetup bool
	}{
		{"missing default", &process.RunError{Kind: process.ErrorKindExecutableNotFound}, "opencode", true},
		{"missing pin", &process.RunError{Kind: process.ErrorKindExecutableNotFound}, "/custom/opencode", true},
		{"missing custom name", &process.RunError{Kind: process.ErrorKindExecutableNotFound}, "my-agent", true},
		{"exit 127", &process.RunError{Kind: process.ErrorKindNonZeroExit, ExitCode: 127}, "opencode", false},
		{"permission denied", &process.RunError{Kind: process.ErrorKindStart}, "opencode", false},
		{"wrong directory", &process.RunError{Kind: process.ErrorKindWorkingDirectory}, "opencode", false},
		{"provider billing text", errors.New("insufficient_quota"), "opencode", false},
		{"success", nil, "opencode", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			command := process.Command{Name: tc.command, Args: []string{"run", "--model", "chosen"}, Stdin: strings.NewReader("private"), Stdout: io.Discard, EnvOverrides: map[string]string{"PRIVATE": "value"}}
			r := Runner{Tool: "opencode", Runner: runFunc(func(ctx context.Context, got process.Command) (process.Result, error) {
				calls++
				if !reflect.DeepEqual(got, command) {
					t.Fatal("changed command")
				}
				return process.Result{ExitCode: -1}, tc.cause
			})}
			_, err := r.Run(t.Context(), command)
			var setupErr *Error
			if errors.As(err, &setupErr) != tc.wantSetup || calls != 1 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			if !tc.wantSetup && !errors.Is(err, tc.cause) {
				t.Fatal("lost original error")
			}
		})
	}
}

// Only fixture functions simulate npm. This test cannot install or call models.
func TestInstallationFailureMatrix(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("automatic installation is Unix-only")
	}
	for _, tc := range []struct {
		mode, want        string
		installs, prompts int
	}{
		{"yes", "installed", 1, 1},
		{"no", "missing", 0, 1},
		{"disabled", "missing", 0, 0},
		{"no npm", "no_installer", 0, 0},
		{"relative npm", "no_installer", 0, 0},
		{"repo npm", "no_installer", 0, 0},
		{"world writable npm", "no_installer", 0, 0},
		{"existing broken tool", "pinned", 0, 0},
		{"confirmation error", "confirmation_failed", 0, 1},
		{"install failure", "install_failed", 1, 1},
		{"exit without error", "install_failed", 1, 1},
		{"timeout", "install_timeout", 1, 1},
		{"still missing", "not_on_path", 1, 1},
		{"installed in repo", "not_on_path", 1, 1},
		{"concurrent install", "pinned", 0, 1},
		{"busy lock", "install_busy", 0, 1},
		{"git", "missing", 0, 0},
		{"unknown package", "missing", 0, 0},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			repo, bin := t.TempDir(), t.TempDir()
			npm, installed := filepath.Join(bin, "npm"), filepath.Join(bin, "opencode")
			for _, path := range []string{npm, installed} {
				if err := os.WriteFile(path, []byte("fixture"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if tc.mode == "world writable npm" {
				if err := os.Chmod(npm, 0777); err != nil {
					t.Fatal(err)
				}
			}
			installs, prompts, lookups := 0, 0, 0
			m := NewManager(runFunc(func(ctx context.Context, c process.Command) (process.Result, error) {
				installs++
				if c.Name != npm || c.Dir == repo || c.Dir == "" || c.Stdin != nil || c.Stdout != nil || c.Stderr != nil || c.OutputLimit != 32<<10 || c.Timeout <= 0 {
					t.Fatal("unsafe installer command")
				}
				want := []string{"install", "--global", "--registry=https://registry.npmjs.org", "--no-audit", "--no-fund", "opencode-ai@1.18.23"}
				if !reflect.DeepEqual(c.Args, want) {
					t.Fatalf("unapproved package: %v", c.Args)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("unbounded installer")
				}
				switch tc.mode {
				case "timeout":
					return process.Result{Stderr: "SECRET"}, &process.RunError{Kind: process.ErrorKindTimeout}
				case "install failure":
					return process.Result{Stderr: "SECRET"}, errors.New("SECRET")
				case "exit without error":
					return process.Result{ExitCode: 1, Stderr: "SECRET"}, nil
				}
				return process.Result{}, nil
			}), func(ctx context.Context, request Request) (bool, error) {
				prompts++
				if request.Tool != "opencode" || !strings.Contains(request.Command, "opencode-ai@1.18.23") {
					t.Fatal("incomplete consent")
				}
				if tc.mode == "confirmation error" {
					return true, errors.New("SECRET")
				}
				return tc.mode != "no", nil
			}, time.Second)
			m.lock = func() (io.Closer, error) {
				if tc.mode == "busy lock" {
					return nil, errors.New("PRIVATE LOCK")
				}
				return io.NopCloser(strings.NewReader("")), nil
			}
			m.lookup = func(name string) (string, error) {
				if name == "npm" {
					switch tc.mode {
					case "no npm":
						return "", exec.ErrNotFound
					case "relative npm":
						return "./npm", nil
					case "repo npm":
						return filepath.Join(repo, "npm"), nil
					}
					return npm, nil
				}
				lookups++
				if tc.mode == "existing broken tool" || (tc.mode == "concurrent install" && lookups > 1) {
					return installed, nil
				}
				if installs > 0 && tc.mode != "still missing" {
					if tc.mode == "installed in repo" {
						return filepath.Join(repo, "opencode"), nil
					}
					return installed, nil
				}
				return "", exec.ErrNotFound
			}
			if tc.mode == "disabled" {
				m.Confirm = nil
			}
			tool := "opencode"
			if tc.mode == "git" {
				tool = "git"
			}
			if tc.mode == "unknown package" {
				tool = "private-package"
			}
			err := m.handle(t.Context(), tool, repo)
			var e *Error
			if !errors.As(err, &e) || e.Code != tc.want || installs != tc.installs || prompts != tc.prompts {
				t.Fatalf("error=%v installs=%d prompts=%d", err, installs, prompts)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatal("installer diagnostics leaked")
			}
			entries, _ := os.ReadDir(repo)
			if len(entries) != 0 {
				t.Fatal("setup wrote to target repository")
			}
		})
	}
}

func TestInstallationCancellationAndStageDeadline(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("automatic installation is Unix-only")
	}
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before launch", false: "during consent"}[before], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if before {
				cancel()
			}
			bin := filepath.Join(t.TempDir(), "npm")
			if err := os.WriteFile(bin, []byte("fixture"), 0700); err != nil {
				t.Fatal(err)
			}
			m := NewManager(runFunc(func(context.Context, process.Command) (process.Result, error) {
				t.Fatal("installer ran")
				return process.Result{}, nil
			}), func(ctx context.Context, _ Request) (bool, error) { <-ctx.Done(); return false, ctx.Err() }, time.Minute)
			m.lookup = func(name string) (string, error) {
				if name == "npm" {
					return bin, nil
				}
				return "", exec.ErrNotFound
			}
			r := Runner{Tool: "opencode", Manager: m, Runner: runFunc(func(context.Context, process.Command) (process.Result, error) {
				if before {
					t.Fatal("started after cancellation")
				}
				return process.Result{}, &process.RunError{Kind: process.ErrorKindExecutableNotFound}
			})}
			_, err := r.Run(ctx, process.Command{Name: "opencode", Dir: t.TempDir(), Timeout: 10 * time.Millisecond})
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("lost cancellation: %v", err)
			}
		})
	}
}
