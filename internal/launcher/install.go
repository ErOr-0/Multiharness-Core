package launcher

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

func install(out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".local", "bin")
	name := "magent"
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			return fmt.Errorf("LOCALAPPDATA is unavailable")
		}
		dir = filepath.Join(base, "Programs", "Magent")
		name += ".exe"
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, name)
	if executable != target {
		data, err := os.ReadFile(executable)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0755); err != nil {
			return err
		}
	}
	if err := installPath(dir); err != nil {
		return fmt.Errorf("launcher installed at %s, but PATH update failed: %w", target, err)
	}
	_, err = fmt.Fprintf(out, "Installed: %s\nOpen a new terminal, then run magent --config.\n", target)
	return err
}
