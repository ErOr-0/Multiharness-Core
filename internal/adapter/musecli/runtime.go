// Package musecli owns the shared native Muse CLI boundary.
package musecli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Executable resolves the official Windows launcher's installed native binary.
// Never pass model arguments or prompts through cmd.exe. Explicit pins stay pins.
func Executable(name string) (string, error) {
	if runtime.GOOS != "windows" || name != "muse" {
		return name, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("Muse Code is not installed; install it from https://dev.meta.ai/docs/muse-code and run muse login: %w", err)
	}
	if strings.EqualFold(filepath.Ext(path), ".exe") {
		return path, nil
	}
	if !strings.EqualFold(filepath.Base(path), "muse.cmd") {
		return "", fmt.Errorf("unsupported Muse launcher; configure the native executable path")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(path), ".muse-version"))
	if err != nil {
		return "", fmt.Errorf("read installed Muse version: %w", err)
	}
	version := strings.TrimSpace(string(data))
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-R[0-9]+(\.[0-9]+)?$`).MatchString(version) {
		return "", fmt.Errorf("invalid installed Muse version")
	}
	native := filepath.Join(filepath.Dir(path), "muse-bin-"+version+".exe")
	if info, err := os.Stat(native); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("installed Muse native executable is unavailable; repair the Muse installation")
	}
	return native, nil
}
