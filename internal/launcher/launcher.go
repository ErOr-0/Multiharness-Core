// Package launcher owns host folder selection and Docker delivery, not workflow policy.
package launcher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	dockerassets "multiharness-core/docker"
)

const image = "er0r2/multiharness-core:preview"

type settings struct {
	Folder string `json:"folder"`
}

type app struct {
	in          io.Reader
	out, stderr io.Writer
	directory   string
	settings    settings
	image       string
}

func Run(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
	if len(args) == 1 && args[0] == "--install" {
		return install(out)
	}
	if len(args) > 1 || (len(args) == 1 && args[0] != "--config" && args[0] != "--update" && args[0] != "--help") {
		return errors.New("use magent, magent --config, or magent --update")
	}
	if len(args) == 1 && args[0] == "--help" {
		_, err := fmt.Fprintln(out, "magent starts your Docker workspace.\nmagent --config changes Folder, Models or Accounts.\nmagent --update downloads the current image.\nmagent --install installs this launcher for your user account.\nInstall and start Docker Desktop first. Project files are edited directly on your PC.")
		return err
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	imageName := os.Getenv("MAGENT_DOCKER_IMAGE")
	if imageName == "" {
		imageName = image
	}
	if strings.HasPrefix(imageName, "-") || strings.ContainsAny(imageName, " \t\r\n") {
		return errors.New("invalid MAGENT_DOCKER_IMAGE")
	}
	a := &app{in: in, out: out, stderr: stderr, directory: filepath.Join(dir, "magent-launcher"), image: imageName}
	data, err := os.ReadFile(filepath.Join(a.directory, "settings.json"))
	if err == nil {
		if err := json.Unmarshal(data, &a.settings); err != nil {
			return fmt.Errorf("cannot read launcher settings: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(args) == 1 && args[0] == "--update" {
		return a.command(ctx, "pull", a.image)
	}
	if len(args) == 1 || a.settings.Folder == "" {
		return a.configure(ctx)
	}
	return a.start(ctx, nil)
}

func (a *app) configure(ctx context.Context) error {
	for {
		fmt.Fprintf(a.out, "\nMAGENT CONFIGURATION\nFolder: %s\n  1. Folder on this computer\n  2. Models and agent roles\n  3. Accounts / sign in\n  4. Start Magent\n  5. Download image update\n  0. Exit\nChoose > ", a.settings.Folder)
		line, err := a.readLine(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch line {
		case "0", "exit":
			return nil
		case "1", "folder":
			fmt.Fprint(a.out, "Full folder path on this computer (blank cancels) > ")
			path, err := a.readLine(ctx)
			if err != nil {
				return err
			}
			if path == "" {
				continue
			}
			path, err = hostFolder(path)
			if err != nil {
				fmt.Fprintln(a.out, err)
				continue
			}
			a.settings.Folder = path
			if err := a.save(); err != nil {
				return err
			}
			fmt.Fprintln(a.out, "Folder saved. Docker will connect this original folder on the next start.")
		case "2", "models":
			if err := a.start(ctx, []string{"configure"}); err != nil {
				fmt.Fprintln(a.stderr, err)
			}
		case "3", "accounts":
			fmt.Fprint(a.out, "Account: 1 Codex / ChatGPT, 2 OpenCode (blank cancels) > ")
			provider, err := a.readLine(ctx)
			if err != nil {
				return err
			}
			if provider == "1" {
				provider = "codex"
			}
			if provider == "2" {
				provider = "opencode"
			}
			if provider == "" {
				continue
			}
			if provider != "codex" && provider != "opencode" {
				fmt.Fprintln(a.out, "Choose Codex or OpenCode.")
				continue
			}
			if err := a.start(ctx, []string{"login", provider}); err != nil {
				fmt.Fprintln(a.stderr, err)
			}
		case "4", "start":
			return a.start(ctx, nil)
		case "5", "update":
			if err := a.command(ctx, "pull", a.image); err != nil {
				fmt.Fprintln(a.stderr, err)
			}
		default:
			fmt.Fprintln(a.out, "Choose a menu item from 0 to 5.")
		}
	}
}

func hostFolder(path string) (string, error) {
	path = strings.Trim(strings.TrimSpace(path), "\"'")
	if !filepath.IsAbs(path) {
		return "", errors.New("enter a full host path, for example D:\\QNE on Windows")
	}
	if strings.ContainsAny(path, ",\r\n\x00\"") {
		return "", errors.New("Docker folder paths cannot contain commas, quotes or control characters")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("folder does not exist or cannot be accessed")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("select an existing directory")
	}
	return resolved, nil
}

func (a *app) save() error {
	if err := os.MkdirAll(a.directory, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a.settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(a.directory, "settings.json"), append(data, '\n'), 0600)
}

func (a *app) start(ctx context.Context, command []string) error {
	folder, err := hostFolder(a.settings.Folder)
	if err != nil {
		return fmt.Errorf("%w; use magent --config to choose your folder", err)
	}
	if err := os.MkdirAll(a.directory, 0700); err != nil {
		return err
	}
	policy := filepath.Join(a.directory, "seccomp.json")
	if err := os.WriteFile(policy, dockerassets.Seccomp, 0600); err != nil {
		return err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	probe := exec.CommandContext(probeCtx, "docker", "info", "--format", "{{.OSType}} {{json .SecurityOptions}}")
	info, err := probe.Output()
	if err != nil {
		return errors.New("Docker is unavailable. Open Docker Desktop, wait for its engine, then try again")
	}
	if !strings.HasPrefix(string(info), "linux ") {
		return errors.New("switch Docker Desktop to Linux containers")
	}
	args := runArgs(folder, policy, runtime.GOOS == "linux", strings.Contains(string(info), "name=apparmor"), command)
	args[len(args)-len(command)-1] = a.image
	var token [12]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	name := "magent-" + hex.EncodeToString(token[:])
	args = append([]string{"run", "--name", name}, args[1:]...)
	defer func() {
		if ctx.Err() != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = exec.CommandContext(cleanupCtx, "docker", "stop", "--time", "2", name).Run()
		}
	}()
	fmt.Fprintf(a.out, "\nOpening %s in Docker. Changes affect the original files.\n", folder)
	return a.command(ctx, args...)
}

func (a *app) readLine(ctx context.Context) (string, error) {
	type result struct {
		value string
		err   error
	}
	done := make(chan result, 1)
	go func() { value, err := readLine(a.in); done <- result{value, err} }()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-done:
		return r.value, r.err
	}
}

func runArgs(folder, policy string, linux, apparmor bool, command []string) []string {
	args := []string{"run", "--rm", "--init", "--interactive", "--tty", "--env", "MAGENT_HOST_LAUNCHER=1", "--cap-drop", "ALL", "--security-opt", "no-new-privileges=true", "--security-opt", "seccomp=" + policy,
		"--mount", "type=bind,src=" + folder + ",dst=/workspace", "--mount", "type=volume,src=magent-state,dst=/state"}
	if linux {
		args = append(args, "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()))
	}
	if apparmor {
		args = append(args, "--security-opt", "apparmor=magent-container-v1")
	}
	return append(append(args, image), command...)
}

func (a *app) command(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = a.in, a.out, a.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Docker command stopped: %w", err)
	}
	return nil
}

// Read one line without buffering input intended for a subsequent provider login.
func readLine(in io.Reader) (string, error) {
	var line strings.Builder
	var b [1]byte
	for {
		n, err := in.Read(b[:])
		if n > 0 {
			if b[0] == '\n' {
				return strings.TrimSpace(line.String()), nil
			}
			if line.Len() >= 8192 {
				return "", errors.New("input is too long")
			}
			line.WriteByte(b[0])
		}
		if err != nil {
			return "", err
		}
	}
}
