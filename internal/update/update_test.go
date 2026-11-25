package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/config"
	"multiharness-core/internal/testkit"
)

func TestMain(m *testing.M) { testkit.Main(m) }

// testHome points the state at a fresh folder and clears the env guards testkit
// and a caller's shell may set, so Auto is free to talk to the fake server.
func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("HARNESS_BRIDGE_HOME", t.TempDir())
	t.Setenv(noUpdateEnv, "")
	t.Setenv(agent.JobEnv, "")
}

// fakeExe points resolveExe at a temp file, so no test ever replaces its own
// test binary.
func fakeExe(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "harness-bridge")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	old := resolveExe
	resolveExe = func() (string, error) { return path, nil }
	t.Cleanup(func() { resolveExe = old })
	return path
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := cli.Version
	cli.Version = v
	t.Cleanup(func() { cli.Version = old })
}

func sumsFor(name string, body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]) + "  " + name + "\n"
}

// releaseServer serves the release URLs: /latest redirects to the tag's page,
// and the download folder serves the assets. It counts every request.
func releaseServer(t *testing.T, tag string, files map[string]string, counts *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if counts != nil {
			counts.Add(1)
		}
		switch {
		case req.URL.Path == "/latest":
			w.Header().Set("Location", "/releases/tag/"+tag)
			w.WriteHeader(http.StatusFound)
		case strings.HasPrefix(req.URL.Path, "/download/"):
			if body, ok := files[filepath.Base(req.URL.Path)]; ok {
				_, _ = io.WriteString(w, body)
				return
			}
			http.NotFound(w, req)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runUpdate(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = CmdUpdate(args, &out, &errOut)
	return out.String(), errOut.String(), code
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		newer           bool
	}{
		{"v1.0.6", "v1.0.7", true},
		{"v1.0.7", "v1.0.6", false},
		{"v1.0.9", "v1.0.10", true},
		{"v1.0.10", "v1.0.9", false}, // an older release never downgrades
		{"v1.2.3", "v1.2.3", false},
		{"v0.9.9", "v1.0.0", true},
		{"dev", "v1.0.0", false},
		{"v1.0.0", "dev", false},
		{"v1.0.0", "nightly", false},
		{"1.0.0", "v1.0.1", false},
		{"v1.2", "v1.2.1", false},
		{"v1.0.0", "v1.0", false},
	} {
		if got := Newer(tc.current, tc.latest); got != tc.newer {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.newer)
		}
	}
}

func TestLatest(t *testing.T) {
	t.Run("a redirect to a tag page gives the tag", func(t *testing.T) {
		srv := releaseServer(t, "v1.2.3", nil, nil)
		t.Setenv(baseEnv, srv.URL)
		tag, err := Latest(context.Background(), srv.Client())
		if err != nil || tag != "v1.2.3" {
			t.Fatalf("Latest: %q, %v", tag, err)
		}
	})
	t.Run("a 200 response is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {}))
		t.Cleanup(srv.Close)
		t.Setenv(baseEnv, srv.URL)
		if tag, err := Latest(context.Background(), srv.Client()); err == nil {
			t.Fatalf("Latest: %q, want an error", tag)
		}
	})
	t.Run("a tag that is not a version is an error", func(t *testing.T) {
		srv := releaseServer(t, "nightly", nil, nil)
		t.Setenv(baseEnv, srv.URL)
		if tag, err := Latest(context.Background(), srv.Client()); err == nil {
			t.Fatalf("Latest: %q, want an error", tag)
		}
	})
}

func TestInstallReplacesTheBinary(t *testing.T) {
	testHome(t)
	exe := fakeExe(t, "old-binary")
	body := "the new binary"
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	srv := releaseServer(t, "v1.2.3", map[string]string{
		asset:           body,
		"checksums.txt": sumsFor(asset, []byte(body)),
	}, nil)
	t.Setenv(baseEnv, srv.URL)
	if err := Install(context.Background(), srv.Client(), "v1.2.3", exe); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := readFile(t, exe); got != body {
		t.Fatalf("the exe holds %q", got)
	}
	if _, err := os.Stat(exe + ".new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the .new file was left behind: %v", err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(exe); err != nil || info.Mode()&0o111 == 0 {
			t.Fatalf("the swapped-in exe is not executable: %v %v", info, err)
		}
	}
}

func TestInstallChecksumFailuresLeaveTheBinaryAlone(t *testing.T) {
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	for _, tc := range []struct {
		name string
		sums string
		want string
	}{
		{"mismatch", sumsFor(asset, []byte("some other binary")), "checksum mismatch"},
		{"missing entry", sumsFor("harness-bridge_other", []byte("x")), "no entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testHome(t)
			exe := fakeExe(t, "old-binary")
			srv := releaseServer(t, "v1.2.3", map[string]string{asset: "new-binary", "checksums.txt": tc.sums}, nil)
			t.Setenv(baseEnv, srv.URL)
			err := Install(context.Background(), srv.Client(), "v1.2.3", exe)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Install: %v, want an error with %q", err, tc.want)
			}
			if got := readFile(t, exe); got != "old-binary" {
				t.Fatalf("the exe changed to %q", got)
			}
			if _, err := os.Stat(exe + ".new"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("the .new file was left behind: %v", err)
			}
		})
	}
}

// The Windows swap runs on every OS, so the tests cover it everywhere.
func TestSwapWindows(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "harness-bridge.exe")
	testkit.Write(t, exe, "old-binary")
	testkit.Write(t, exe+".new", "new-binary")
	if err := swapWindows(exe+".new", exe); err != nil {
		t.Fatalf("swapWindows: %v", err)
	}
	if got := readFile(t, exe); got != "new-binary" {
		t.Fatalf("the exe holds %q", got)
	}
	if got := readFile(t, exe+".old"); got != "old-binary" {
		t.Fatalf("the .old file holds %q", got)
	}
	if _, err := os.Stat(exe + ".new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the .new file was left behind: %v", err)
	}
}

// An .old that cannot be removed (still running) must not stop the swap: a
// unique name is used instead. A non-empty directory stands in for it, since
// that cannot be removed on any OS.
func TestSwapWindowsWithAStuckOldFile(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "harness-bridge.exe")
	testkit.Write(t, exe, "old-binary")
	testkit.Write(t, exe+".new", "new-binary")
	stuck := exe + ".old"
	testkit.Write(t, filepath.Join(stuck, "held"), "x")
	if err := swapWindows(exe+".new", exe); err != nil {
		t.Fatalf("swapWindows: %v", err)
	}
	if got := readFile(t, exe); got != "new-binary" {
		t.Fatalf("the exe holds %q", got)
	}
	matches, _ := filepath.Glob(exe + "*.old")
	found := false
	for _, m := range matches {
		info, err := os.Stat(m)
		if err == nil && !info.IsDir() && readFile(t, m) == "old-binary" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the old exe was not kept under a unique name: %v", matches)
	}
}

func TestCleanupRemovesLeftovers(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "harness-bridge")
	testkit.Write(t, exe, "binary")
	for _, name := range []string{exe + ".old", exe + ".123.old", exe + ".new"} {
		testkit.Write(t, name, "leftover")
	}
	testkit.Write(t, filepath.Join(dir, "unrelated.txt"), "keep")
	Cleanup(exe)
	for _, name := range []string{exe + ".old", exe + ".123.old", exe + ".new"} {
		if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s survived: %v", name, err)
		}
	}
	if readFile(t, exe) != "binary" {
		t.Error("Cleanup removed the exe itself")
	}
	if readFile(t, filepath.Join(dir, "unrelated.txt")) != "keep" {
		t.Error("Cleanup removed an unrelated file")
	}
}

// A lock a dead process left behind is stale after 10 minutes and is taken
// over instead of obeyed.
func TestLockTakesOverAStaleLock(t *testing.T) {
	testHome(t)
	dir, err := stateDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, lockFile)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-staleLock - time.Minute)
	if err := os.Chtimes(path, stale, stale); err != nil {
		t.Fatal(err)
	}
	release, err := lock()
	if err != nil {
		t.Fatalf("lock() over a stale lock: %v", err)
	}
	release()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the lock was left behind: %v", err)
	}
}

// Tidy must not delete another install's files while that install holds the
// lock, and must remove them once the lock is free.
func TestTidyClearsLeftoversOnlyWithoutTheLock(t *testing.T) {
	testHome(t)
	exe := fakeExe(t, "binary")
	testkit.Write(t, exe+".old", "leftover")
	release, err := lock()
	if err != nil {
		t.Fatal(err)
	}
	Tidy()
	if _, err := os.Stat(exe + ".old"); err != nil {
		t.Fatalf("Tidy removed an install's leftovers while the lock was held: %v", err)
	}
	release()
	Tidy()
	if _, err := os.Stat(exe + ".old"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Tidy left the .old behind: %v", err)
	}
}

func TestAutoSkipsWithoutTouchingTheNetwork(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current string
		cfg     config.Config
		prepare func(t *testing.T)
	}{
		{"the no-update env var", "v1.0.0", config.Config{}, func(t *testing.T) {
			t.Setenv(noUpdateEnv, "1")
		}},
		{"the config opt-out", "v1.0.0", config.Config{NoAutoUpdate: true}, nil},
		{"a dev build", "dev", config.Config{}, nil},
		{"a job's implementer", "v1.0.0", config.Config{}, func(t *testing.T) {
			t.Setenv(agent.JobEnv, "1")
		}},
		{"a fresh throttle entry", "v1.0.0", config.Config{}, func(t *testing.T) {
			if err := writeState(state{CheckedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}},
		{"a held lock", "v1.0.0", config.Config{}, func(t *testing.T) {
			dir, err := stateDir()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, lockFile), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testHome(t)
			fakeExe(t, "old-binary")
			counts := new(atomic.Int64)
			srv := releaseServer(t, "v9.9.9", nil, counts)
			t.Setenv(baseEnv, srv.URL)
			if tc.prepare != nil {
				tc.prepare(t)
			}
			installed, err := Auto(context.Background(), tc.current, tc.cfg)
			if err != nil || installed != "" {
				t.Fatalf("Auto: installed %q, err %v", installed, err)
			}
			if counts.Load() != 0 {
				t.Fatalf("Auto made %d requests", counts.Load())
			}
		})
	}
}

func TestAutoInstallsANewRelease(t *testing.T) {
	testHome(t)
	exe := fakeExe(t, "old-binary")
	body := "the new binary"
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	counts := new(atomic.Int64)
	srv := releaseServer(t, "v1.2.3", map[string]string{
		asset:           body,
		"checksums.txt": sumsFor(asset, []byte(body)),
	}, counts)
	t.Setenv(baseEnv, srv.URL)

	installed, err := Auto(context.Background(), "v1.0.0", config.Config{})
	if err != nil || installed != "v1.2.3" {
		t.Fatalf("Auto: installed %q, err %v", installed, err)
	}
	if got := readFile(t, exe); got != body {
		t.Fatalf("the exe holds %q", got)
	}
	if _, err := os.Stat(exe + ".new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the .new file was left behind: %v", err)
	}
	st := readState()
	if st.CheckedAt.IsZero() || st.Latest != "v1.2.3" || st.Installed != "v1.2.3" || st.Error != "" {
		t.Fatalf("state %+v", st)
	}
	dir, err := stateDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, lockFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the lock was left behind: %v", err)
	}
	// Right after a check, the throttle keeps the next one off the network.
	before := counts.Load()
	if installed, err := Auto(context.Background(), "v1.0.0", config.Config{}); err != nil || installed != "" {
		t.Fatalf("the second Auto: installed %q, err %v", installed, err)
	}
	if counts.Load() != before {
		t.Fatalf("the second Auto made %d requests", counts.Load()-before)
	}
}

func TestAutoRecordsAnError(t *testing.T) {
	testHome(t)
	fakeExe(t, "old-binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv(baseEnv, srv.URL)
	if installed, err := Auto(context.Background(), "v1.0.0", config.Config{}); err == nil || installed != "" {
		t.Fatalf("Auto: installed %q, err %v", installed, err)
	}
	st := readState()
	if st.Error == "" || st.CheckedAt.IsZero() {
		t.Fatalf("state %+v", st)
	}
}

func TestUpdateCommand(t *testing.T) {
	t.Run("a dev build exits 1", func(t *testing.T) {
		testHome(t)
		stdout, stderr, code := runUpdate(t)
		if code != cli.ExitFailed || stdout != "" || !strings.Contains(stderr, "this is a development build (version dev)") {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("--check reports a newer release", func(t *testing.T) {
		testHome(t)
		fakeExe(t, "old-binary")
		setVersion(t, "v1.0.0")
		srv := releaseServer(t, "v1.2.3", nil, nil)
		t.Setenv(baseEnv, srv.URL)
		stdout, stderr, code := runUpdate(t, "--check")
		if code != cli.ExitOK || stderr != "" || stdout != "harness-bridge v1.2.3 is available (you have v1.0.0); run harness-bridge update\n" {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("already current", func(t *testing.T) {
		testHome(t)
		setVersion(t, "v1.2.3")
		srv := releaseServer(t, "v1.2.3", nil, nil)
		t.Setenv(baseEnv, srv.URL)
		stdout, stderr, code := runUpdate(t, "--check")
		if code != cli.ExitOK || stderr != "" || stdout != "harness-bridge is up to date (v1.2.3)\n" {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("installs and says to restart", func(t *testing.T) {
		testHome(t)
		exe := fakeExe(t, "old-binary")
		setVersion(t, "v1.0.0")
		body := "the new binary"
		asset := AssetName(runtime.GOOS, runtime.GOARCH)
		srv := releaseServer(t, "v1.2.3", map[string]string{
			asset:           body,
			"checksums.txt": sumsFor(asset, []byte(body)),
		}, nil)
		t.Setenv(baseEnv, srv.URL)
		stdout, stderr, code := runUpdate(t)
		if code != cli.ExitOK || stderr != "" || stdout != "Updated harness-bridge v1.0.0 -> v1.2.3. Restart your coding agent to use it.\n" {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		if got := readFile(t, exe); got != body {
			t.Fatalf("the exe holds %q", got)
		}
	})
	t.Run("an unknown flag is a usage error", func(t *testing.T) {
		testHome(t)
		if _, _, code := runUpdate(t, "--bogus"); code != cli.ExitUsage {
			t.Fatalf("exit %d, want %d", code, cli.ExitUsage)
		}
	})
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
