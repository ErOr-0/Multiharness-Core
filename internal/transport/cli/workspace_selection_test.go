package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func TestDockerWorkspaceSelectionGuardsTasksAndSwitchesWithoutCopies(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	for _, name := range []string{"api", "web"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	var folders []string
	factory := func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
		folders = append(folders, cfg.WorkingDir)
		if cfg.SessionID != "" {
			t.Fatal("workspace switch retained a session")
		}
		// Stop before providers: selecting a workspace must not copy any project.
		return nil, os.ErrNotExist
	}
	h := newHandler(t, factory, &stdout, &stderr, root, map[string]string{"MAGENT_WORKSPACE_ROOT": root})
	input := &promptLines{lines: []string{
		outside, "missing", "api", "", "first task",
		"/set workdir " + outside, "blocked task",
		"/workspace", "cd ..", "web", "", "second task", "/quit",
	}}
	code := h.Interactive(t.Context(), input, filepath.Join(t.TempDir(), "config.json"))
	if code != 0 || len(folders) != 2 || folders[0] != filepath.Join(root, "api") || folders[1] != filepath.Join(root, "web") {
		t.Fatalf("code=%d folders=%v output=%s", code, folders, stdout.String())
	}
	if !strings.Contains(stdout.String(), "choose a folder inside") {
		t.Fatal(stdout.String())
	}
}

func TestDockerWorkspaceCancelAndConfigAreAtomic(t *testing.T) {
	for _, lines := range [][]string{{"/cancel", "must not run"}, {"0", "/config", "0", "/cancel", "/quit"}} {
		root := t.TempDir()
		var out bytes.Buffer
		h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
			t.Fatal("cancelled selection started work")
			return nil, nil
		}, &out, &out, root, map[string]string{"MAGENT_WORKSPACE_ROOT": root})
		if code := h.Interactive(t.Context(), &promptLines{lines: lines}, filepath.Join(t.TempDir(), "config.json")); code != 0 {
			t.Fatal(code, out.String())
		}
	}
}

func TestDockerWorkspaceRejectsSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip("symlinks unavailable", err)
	}
	var out bytes.Buffer
	h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		t.Fatal("escaped workspace started work")
		return nil, nil
	}, &out, &out, root, map[string]string{"MAGENT_WORKSPACE_ROOT": root})
	if code := h.Interactive(t.Context(), &promptLines{lines: []string{"escape", "/cancel"}}, filepath.Join(t.TempDir(), "config.json")); code != 0 || !strings.Contains(out.String(), "choose a folder inside") {
		t.Fatal(code, out.String())
	}
}

func TestWorkspaceBrowserNavigatesCreatesAndSelectsOnEnter(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	var out bytes.Buffer
	calls := 0
	h := newHandler(t, func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
		calls++
		if cfg.WorkingDir != filepath.Join(root, "My Project", "api") {
			t.Fatalf("selected wrong directory: %s", cfg.WorkingDir)
		}
		return nil, os.ErrNotExist
	}, &out, &out, root, map[string]string{"MAGENT_WORKSPACE_ROOT": root})
	lines := []string{"/config", `D:\QNE`, `mkdir "My Project"`, "1", "mkdir api", "cd api", "pwd", "ls", "", "task", "/quit"}
	if code := h.Interactive(t.Context(), &promptLines{lines: lines}, filepath.Join(t.TempDir(), "config.json")); code != 0 || calls != 1 {
		t.Fatal(code, calls, out.String())
	}
	if info, err := os.Stat(filepath.Join(root, "My Project", "api")); err != nil || !info.IsDir() {
		t.Fatal("mkdir did not create the original directory", err)
	}
	if !strings.Contains(out.String(), "Windows host path") {
		t.Fatal("missing actionable host-path guidance")
	}
}

func TestWorkspaceBrowserCannotCreateOutsideMountAndCancelKeepsCreatedFolder(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	var out bytes.Buffer
	h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		t.Fatal("cancelled browser started a task")
		return nil, nil
	}, &out, &out, root, map[string]string{"MAGENT_WORKSPACE_ROOT": root})
	lines := []string{"mkdir " + filepath.Join(outside, "forbidden"), "mkdir kept", "/cancel"}
	if code := h.Interactive(t.Context(), &promptLines{lines: lines}, filepath.Join(t.TempDir(), "config.json")); code != 0 {
		t.Fatal(code, out.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "forbidden")); !os.IsNotExist(err) {
		t.Fatal("created outside mount", err)
	}
	if _, err := os.Stat(filepath.Join(root, "kept")); err != nil {
		t.Fatal("cancel removed an explicitly created folder", err)
	}
}
