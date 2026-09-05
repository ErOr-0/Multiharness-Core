package schemaexec

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
)

const runtimeHelp = "--model --sandbox --ephemeral --json --color --cd --config --output-schema --output-last-message --skip-git-repo-check"

func TestMissingRuntimeIsDistinctFromIncompatibility(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	r := NewRuntimeRunner(&fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("missing runtime launched a process")
		return process.Result{}, nil
	}}, nil)
	r.cacheVersion = func() (string, error) { return "", nil }
	r.candidates = func(string) []string { return nil }
	_, err := r.Resolve(t.Context(), "codex", t.TempDir())
	var missing interface{ MissingExecutable() bool }
	if !errors.As(err, &missing) || !missing.MissingExecutable() {
		t.Fatal("missing runtime not classified", err)
	}
	for _, code := range []string{"no_compatible_cli", "cache_unreadable", "notice_failed"} {
		if (&CompatibilityError{Code: code}).MissingExecutable() {
			t.Fatal("unsafe install trigger")
		}
	}
}

func TestRuntimeRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, cache, primary, alternate, want               string
		missingFlag, probeFailure, truncated, noticeFailure bool
	}{
		{name: "new app replaces old CLI", cache: "0.153.0", primary: "0.136.0", alternate: "0.153.0", want: "/app/codex"},
		{
			name:      "compatible primary retained",
			cache:     "0.153.0",
			primary:   "0.153.0",
			alternate: "0.154.0",
			want:      "/primary/codex",
		},
		{name: "newer primary retained", cache: "0.153.0", primary: "0.154.0", want: "/primary/codex"},
		{name: "no cache", primary: "0.136.0", want: "/primary/codex"},
		{name: "unavailable primary", cache: "0.153.0", alternate: "0.153.0", probeFailure: true, want: "/app/codex"},
		{
			name:        "missing required flag",
			cache:       "0.153.0",
			primary:     "0.153.0",
			alternate:   "0.153.0",
			missingFlag: true,
			want:        "/app/codex",
		},
		{name: "truncated probe", primary: "0.153.0", alternate: "0.153.0", truncated: true, want: "/app/codex"},
		{name: "no compatible binary", cache: "0.153.0", primary: "0.136.0", alternate: "0.140.0"},
		{name: "do not guess prerelease", cache: "0.153.0", primary: "0.154.0-beta.1"},
		{name: "failed notice prevents task", cache: "0.153.0", primary: "0.153.0", want: "/primary/codex", noticeFailure: true},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				tasks := 0
				failure := errors.New("task failed; never replay it")
				original := process.Command{
					Name:         "codex",
					Args:         []string{"exec", "--model", "chosen-model", "--sandbox", "workspace-write", "-"},
					Dir:          t.TempDir(),
					Timeout:      time.Minute,
					Stdin:        strings.NewReader("private task"),
					Stdout:       io.Discard,
					Stderr:       io.Discard,
					EnvOverrides: map[string]string{"CUSTOM": "value"},
					EnvUnset:     []string{"REMOVE"},
				}
				runner := &fakeProcessRunner{run: func(ctx context.Context, command process.Command) (process.Result, error) {
					if reflect.DeepEqual(command.Args, original.Args) {
						tasks++
						want := original
						want.Name = tc.want
						if !reflect.DeepEqual(command, want) {
							t.Fatal("runtime selection changed task inputs/permissions/streams")
						}
						return process.Result{ExitCode: 7}, failure
					}
					if command.Stdin != nil || command.Stdout != nil || command.Stderr != nil || command.Timeout <= 0 || command.Timeout > 2*time.Second {
						t.Fatal("preflight received task input/observers or an unbounded timeout")
					}
					primary := command.Name == "/primary/codex"
					if primary && tc.probeFailure {
						return process.Result{Stderr: "SECRET"}, errors.New("SECRET")
					}
					if reflect.DeepEqual(command.Args, []string{"--version"}) {
						version := tc.alternate
						if primary {
							version = tc.primary
						}
						return process.Result{Stdout: "codex-cli " + version, StdoutTruncated: primary && tc.truncated}, nil
					}
					if !reflect.DeepEqual(command.Args, []string{"exec", "--help"}) {
						t.Fatal("unexpected preflight command")
					}
					help := runtimeHelp
					if primary && tc.missingFlag {
						help = strings.ReplaceAll(help, "--output-schema", "--output-schema-unsupported")
					}
					return process.Result{Stdout: help}, nil
				}}
				notices := 0
				runtime := NewRuntimeRunner(runner, func(version string) error {
					notices++
					if _, ok := releaseVersion(version); !ok {
						t.Fatal("notice contained invalid version")
					}
					if tc.noticeFailure {
						return errors.New("SECRET")
					}
					return nil
				})
				runtime.cacheVersion = func() (string, error) { return tc.cache, nil }
				runtime.candidates = func(string) []string { return []string{"/primary/codex", "/app/codex"} }
				_, err := runtime.Run(t.Context(), original)
				if tc.want != "" && !tc.noticeFailure {
					if tasks != 1 || notices != 1 || !errors.Is(err, failure) {
						t.Fatal("lost the task error or replayed the task")
					}
				} else {
					var compatibility *CompatibilityError
					if tasks != 0 || !errors.As(err, &compatibility) || strings.Contains(err.Error(), "SECRET") {
						t.Fatal("unsafe compatibility failure")
					}
				}
			},
		)
	}
}

// Optional local metadata check: only --version/exec --help, never a model call.
func TestInstalledRuntimeSelection(t *testing.T) {
	if os.Getenv("MULTIHARNESS_RUNTIME_CHECK") != "1" {
		t.Skip("local executable preflight is opt-in")
	}
	if os.Getenv("CI") != "" {
		t.Fatal("installed-runtime checks are local only")
	}
	selection, err := NewRuntimeRunner(process.NewOSRunner(), nil).Resolve(t.Context(), "codex", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Selected Codex %s: %s; no model request", selection.Version, selection.Executable)
}

func TestRuntimePinsCancellationAndCacheFailure(t *testing.T) {
	for _, mode := range []string{"pin", "bad cache", "cancelled", "deadline", "nil context", "nil runner"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			runner := &fakeProcessRunner{run: func(ctx context.Context, command process.Command) (process.Result, error) {
				calls++
				if mode == "deadline" {
					<-ctx.Done()
					return process.Result{}, ctx.Err()
				}
				if command.Name != "/pinned/codex" {
					t.Fatal("unexpected process launch")
				}
				return process.Result{}, nil
			}}
			runtime := NewRuntimeRunner(runner, nil)
			runtime.candidates = func(string) []string { return []string{"/candidate/codex"} }
			runtime.cacheVersion = func() (string, error) {
				if mode == "pin" {
					t.Fatal("inspected shared cache for a pinned executable")
				}
				if mode == "bad cache" {
					return "", errors.New("PRIVATE CACHE")
				}
				return "0.153.0", nil
			}
			command := process.Command{Name: "codex", Timeout: 20 * time.Millisecond}
			ctx := t.Context()
			if mode == "pin" {
				command.Name = "/pinned/codex"
			}
			if mode == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if mode == "nil context" {
				ctx = nil
			}
			if mode == "nil runner" {
				runtime.runner = nil
			}
			_, err := runtime.Run(ctx, command)
			switch mode {
			case "pin":
				if err != nil || calls != 1 {
					t.Fatal("explicit pin was overridden")
				}
			case "deadline":
				if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
					t.Fatal("preflight ignored whole-command deadline")
				}
			case "cancelled":
				if !errors.Is(err, context.Canceled) || calls != 0 {
					t.Fatal("cancelled request started a process")
				}
			default:
				if err == nil || calls != 0 || strings.Contains(err.Error(), "PRIVATE CACHE") {
					t.Fatal("invalid input/cache was not safely rejected")
				}
			}
		})
	}
}

func TestRuntimeDiscoveryAndCache(t *testing.T) {
	root, bin := t.TempDir(), t.TempDir()
	paths := []string{
		filepath.Join(root, "codex"),
		filepath.Join(bin, "codex"),
		filepath.Join(bin, "unsafe"),
		filepath.Join(bin, "alias"),
		filepath.Join(root, "alias"),
	}
	for i, path := range paths[:3] {
		mode := os.FileMode(0700)
		if i == 2 {
			mode = 0702
		}
		if err := os.WriteFile(path, []byte("fixture, never executed"), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range paths[3:] {
		if err := os.Symlink(paths[1], link); err != nil {
			if runtime.GOOS == "windows" {
				t.Skip("skipping symlink check on Windows without privilege")
			}
			t.Fatal(err)
		}
	}
	want, err := filepath.EvalSymlinks(paths[1])
	if err != nil {
		t.Fatal(err)
	}
	if got := eligibleRuntimes(append(paths, "relative/codex", bin), root); !reflect.DeepEqual(got, []string{want}) {
		t.Fatalf("unsafe/duplicate candidates: %v", got)
	}
	cache := filepath.Join(root, "models_cache.json")
	if version, err := readRuntimeCache(cache); err != nil || version != "" {
		t.Fatal("missing cache should allow CLI probes")
	}
	for _, data := range []string{`{"client_version":"0.153.0","models":[]}`, `{`, `{}`, `{"client_version":"secret"}`} {
		if err := os.WriteFile(cache, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		version, err := readRuntimeCache(cache)
		if strings.Contains(data, "0.153.0") {
			if err != nil || version != "0.153.0" {
				t.Fatal("valid cache rejected")
			}
		} else if err == nil {
			t.Fatal("invalid cache accepted")
		}
		after, _ := os.ReadFile(cache)
		if string(after) != data {
			t.Fatal("shared cache was modified")
		}
	}
	if _, err := readRuntimeCache(root); err == nil {
		t.Fatal("accepted directory as cache")
	}
	for _, version := range []string{"0.153.0-alpha.1", "0.0153.0", "0.153", "+0.153.0", "0.153.18446744073709551616"} {
		if compatibleVersion(version, "0.136.0") {
			t.Fatalf("guessed version ordering: %s", version)
		}
	}
}
