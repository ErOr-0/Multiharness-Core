// Package update keeps harness-bridge current: it finds the newest GitHub
// release, checks the download's checksum and swaps the binary into place. The
// running process keeps its own code; the new version runs the next time the
// agent starts harness-bridge.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/config"
	"multiharness-core/internal/jsonfile"
)

const (
	defaultBase = "https://github.com/ErOr-0/Multiharness-Core/releases"
	baseEnv     = "HARNESS_BRIDGE_UPDATE_URL"
	noUpdateEnv = "HARNESS_BRIDGE_NO_UPDATE"
	stateFile   = "update.json"
	lockFile    = "update.lock"
	checkEvery  = time.Hour        // the background check hits the network at most this often
	staleLock   = 10 * time.Minute // a lock older than this was left by a dead process
	autoTimeout = 3 * time.Minute  // one check, download and swap, all told
	maxBinary   = 200 << 20        // the release binary is far smaller than this
	maxSums     = 1 << 20
)

// base is the GitHub releases folder. The env override lets the tests run a
// fake release server.
func base() string {
	if u := os.Getenv(baseEnv); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultBase
}

var versionPattern = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

func parseVersion(v string) ([3]int, bool) {
	m := versionPattern.FindStringSubmatch(v)
	if m == nil {
		return [3]int{}, false
	}
	var out [3]int
	for i := range out {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// Newer compares releases by number, so v1.0.10 is newer than v1.0.9. Values
// that do not parse (dev builds, odd tags) never update and never downgrade.
func Newer(current, latest string) bool {
	c, cOK := parseVersion(current)
	l, lOK := parseVersion(latest)
	if !cOK || !lOK {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// AssetName is the release asset for one system, named exactly as the release
// workflow builds it.
func AssetName(goos, goarch string) string {
	name := "harness-bridge_" + goos + "_" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// Latest returns the newest release tag. It follows no redirect: GitHub
// answers /latest with a 302 to the tag's page, and reading that Location
// avoids the API and its rate limit.
func Latest(ctx context.Context, client *http.Client) (string, error) {
	noRedirects := *client
	noRedirects.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	u := base() + "/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := noRedirects.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 3 {
		return "", fmt.Errorf("fetch %s: %s", u, resp.Status)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("fetch %s: the answer has no Location header", u)
	}
	parsed, err := url.Parse(loc)
	if err != nil {
		return "", fmt.Errorf("fetch %s: bad Location %q", u, loc)
	}
	tag := path.Base(parsed.Path)
	if !versionPattern.MatchString(tag) {
		return "", fmt.Errorf("fetch %s: %q is not a release tag", u, tag)
	}
	return tag, nil
}

// download fetches one release file, refusing to read more than limit bytes:
// the assets have known sizes, and a bigger answer is not one of them.
func download(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download %s: more than %d bytes", url, limit)
	}
	return data, nil
}

// checksum returns a checksums.txt entry in sha256sum format: "<hex>  <name>",
// where the name may be marked binary with a leading *.
func checksum(sums []byte, name string) (string, bool) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == name {
			return fields[0], true
		}
	}
	return "", false
}

// Install downloads tag's binary for this system, checks it against the
// release's checksums.txt and swaps it over exePath. The new binary lands as
// .new and is only swapped in once it checks out, so the exe is never missing
// or half-written.
func Install(ctx context.Context, client *http.Client, tag, exePath string) error {
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	bin, err := download(ctx, client, fmt.Sprintf("%s/download/%s/%s", base(), tag, asset), maxBinary)
	if err != nil {
		return err
	}
	sums, err := download(ctx, client, fmt.Sprintf("%s/download/%s/checksums.txt", base(), tag), maxSums)
	if err != nil {
		return err
	}
	tmp := exePath + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		os.Remove(tmp) // a partial file may exist
		return err
	}
	want, ok := checksum(sums, asset)
	got := sha256.Sum256(bin)
	switch {
	case !ok:
		os.Remove(tmp)
		return fmt.Errorf("checksums.txt has no entry for %s", asset)
	case hex.EncodeToString(got[:]) != want:
		os.Remove(tmp)
		return fmt.Errorf("checksum mismatch for %s", asset)
	}
	if err := swap(tmp, exePath); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// swap replaces exePath with newPath. Windows cannot overwrite a running exe
// but can rename it, so there the swap goes through swapWindows; the tests run
// that path on every system.
func swap(newPath, exePath string) error {
	if runtime.GOOS == "windows" {
		return swapWindows(newPath, exePath)
	}
	return os.Rename(newPath, exePath)
}

// swapWindows renames the running exe aside before putting the new one in its
// place, and puts the old one back when the swap fails. It is a plain function
// that works on every OS so the tests can exercise it anywhere.
func swapWindows(newPath, exePath string) error {
	old := exePath + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		// An .old another process still runs cannot be removed; use a fresh name.
		old = fmt.Sprintf("%s.%d.old", exePath, time.Now().UnixNano())
	}
	if err := os.Rename(exePath, old); err != nil {
		return err
	}
	if err := os.Rename(newPath, exePath); err != nil {
		_ = os.Rename(old, exePath) // the exe must never end up missing
		return err
	}
	return nil
}

// Cleanup removes swap leftovers next to the exe, ignoring errors: they are
// only debris, and one may still be held open on Windows.
func Cleanup(exePath string) {
	old, _ := filepath.Glob(exePath + "*.old")
	for _, file := range append(old, exePath+".new") {
		_ = os.Remove(file)
	}
}

// Tidy removes swap leftovers next to the exe, but only while the update lock
// is free: an install in progress owns its own files. It ignores the throttle
// and the opt-outs, so leftovers of a manual update still go.
func Tidy() {
	release, err := lock()
	if err != nil {
		return
	}
	defer release()
	if exe, err := resolveExe(); err == nil {
		Cleanup(exe)
	}
}

// ExePath is the running harness-bridge's own binary, with symlinks resolved:
// an update must replace the real file.
func ExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// resolveExe is a variable so the tests can point updates at a temp file
// instead of their own test binary.
var resolveExe = ExePath

// state is update.json: when the updater last checked and what it found. The
// checked_at makes the next check wait its hour.
type state struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest,omitempty"`
	Installed string    `json:"installed,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// stateDir holds update.json and update.lock: the folder job.Root() sits
// under, so uninstall takes them with the job logs.
func stateDir() (string, error) {
	if home := os.Getenv("HARNESS_BRIDGE_HOME"); home != "" {
		return filepath.Abs(home)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("no user cache directory; set HARNESS_BRIDGE_HOME: %w", err)
	}
	return filepath.Join(cache, "harness-bridge"), nil
}

// readState returns the zero state when nothing is recorded yet.
func readState() state {
	var st state
	dir, err := stateDir()
	if err != nil {
		return st
	}
	_ = jsonfile.Read(filepath.Join(dir, stateFile), &st)
	return st
}

func writeState(st state) error {
	dir, err := stateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return jsonfile.Write(filepath.Join(dir, stateFile), st)
}

// RemoveState deletes the updater's state and lock in dir. uninstall calls it
// so nothing of ours outlives the program.
func RemoveState(dir string) error {
	var first error
	for _, name := range []string{stateFile, lockFile} {
		err := os.Remove(filepath.Join(dir, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) && first == nil {
			first = err
		}
	}
	return first
}

// lock keeps one process installing at a time: several MCP servers can start
// at once. A lock a crashed process left behind is stale after 10 minutes.
func lock() (func(), error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, lockFile)
	for range 2 {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, statErr
		}
		if time.Since(info.ModTime()) < staleLock {
			return nil, errors.New("another update is already running")
		}
		// Stale: a dead process left it. Take it over and retry once. Another
		// process may have removed it first; that is a retry, not a failure.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return nil, errors.New("another update is already running")
}

// Auto checks for a newer release and installs it in the background, at most
// once an hour. It prints nothing: the MCP server's stdout is its protocol
// channel. It returns the tag it installed, if any.
func Auto(ctx context.Context, current string, cfg config.Config) (string, error) {
	if os.Getenv(noUpdateEnv) != "" || cfg.NoAutoUpdate {
		return "", nil
	}
	if _, ok := parseVersion(current); !ok {
		return "", nil // a dev build never updates
	}
	if os.Getenv(agent.JobEnv) != "" {
		return "", nil // a job runs harness-bridge as its own tool
	}
	st := readState()
	if time.Since(st.CheckedAt) < checkEvery {
		return "", nil
	}
	release, err := lock()
	if err != nil {
		return "", nil // another server is already checking
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, autoTimeout)
	defer cancel()
	client := &http.Client{}
	var installed string
	exe, err := resolveExe()
	if err == nil {
		var latest string
		if latest, err = Latest(ctx, client); err == nil {
			st.Latest = latest
			if Newer(current, latest) {
				if err = Install(ctx, client, latest, exe); err == nil {
					installed = latest
					st.Installed = latest
				}
			}
		}
	}
	st.CheckedAt, st.Error = time.Now(), ""
	if err != nil {
		st.Error = err.Error()
	}
	_ = writeState(st)
	return installed, err
}

// CmdUpdate updates harness-bridge on request. Unlike the background check it
// ignores the hourly throttle and the opt-outs, because the user asked for it,
// but it takes the same lock.
func CmdUpdate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	check := fs.Bool("check", false, "")
	if err := fs.Parse(args); err != nil {
		return cli.UsageError(stdout, stderr, err)
	}
	if fs.NArg() > 0 {
		return cli.UsageError(stdout, stderr, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}
	current := cli.Version
	if _, ok := parseVersion(current); !ok {
		fmt.Fprintf(stderr, "harness-bridge: this is a development build (version %s); install a release with install.sh or install.ps1 to get updates\n", current)
		return cli.ExitFailed
	}
	release, err := lock()
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return cli.ExitFailed
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), autoTimeout)
	defer cancel()
	client := &http.Client{}
	tag, err := Latest(ctx, client)
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return cli.ExitFailed
	}
	if !Newer(current, tag) {
		fmt.Fprintf(stdout, "harness-bridge is up to date (%s)\n", current)
		return cli.ExitOK
	}
	if *check {
		fmt.Fprintf(stdout, "harness-bridge %s is available (you have %s); run harness-bridge update\n", tag, current)
		return cli.ExitOK
	}
	exe, err := resolveExe()
	if err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return cli.ExitFailed
	}
	Cleanup(exe)
	if err := Install(ctx, client, tag, exe); err != nil {
		fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
		return cli.ExitFailed
	}
	fmt.Fprintf(stdout, "Updated harness-bridge %s -> %s. Restart your coding agent to use it.\n", current, tag)
	return cli.ExitOK
}
