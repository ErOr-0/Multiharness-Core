package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Installer is one official way to install a CLI. The first one whose GOOS
// matches and whose Needs program is on PATH is offered.
type Installer struct {
	GOOS    string // "", a GOOS value, or "!windows"
	Needs   string
	Command []string
}

func (in Installer) applies() bool {
	switch {
	case in.GOOS == "":
		return true
	case strings.HasPrefix(in.GOOS, "!"):
		return runtime.GOOS != in.GOOS[1:]
	}
	return runtime.GOOS == in.GOOS
}

// Display is the command as the user would type it.
func (in Installer) Display() string {
	c := in.Command
	switch {
	case len(c) == 3 && (c[0] == "bash" || c[0] == "sh") && c[1] == "-c":
		return c[2]
	case len(c) > 0 && c[0] == "powershell":
		return c[len(c)-1] + "   (in PowerShell)"
	}
	return strings.Join(c, " ")
}

func (a *Adapter) Installer() (Installer, bool) {
	for _, in := range a.Install {
		if !in.applies() {
			continue
		}
		if _, err := exec.LookPath(in.Needs); err == nil {
			return in, true
		}
	}
	return Installer{}, false
}

// FindBin finds an installed CLI on PATH, or in the folders official
// installers use, which a terminal opened before the install may not have on
// its PATH yet.
func FindBin(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, ".local", "bin")}
	exts := []string{""}
	if runtime.GOOS == "windows" {
		dirs = append(dirs, filepath.Join(os.Getenv("APPDATA"), "npm"))
		exts = []string{".exe", ".cmd", ".bat"}
	} else {
		dirs = append(dirs, filepath.Join(home, ".npm-global", "bin"), filepath.Join(home, ".opencode", "bin"), "/usr/local/bin", "/opt/homebrew/bin")
	}
	for _, dir := range dirs {
		for _, ext := range exts {
			path := filepath.Join(dir, name+ext)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return path, true
			}
		}
	}
	return "", false
}
