package codex

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func installedRuntimes(workingDir string) []string {
	paths := []string{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		// Never implicitly execute a repository-local or relative PATH candidate.
		if filepath.IsAbs(dir) {
			paths = append(paths, filepath.Join(dir, DefaultExecutable))
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
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Mode().Perm()&0002 != 0 {
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
