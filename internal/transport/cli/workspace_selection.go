package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"multiharness-core/internal/config"
)

// The container supplies this delivery boundary; the workflow still owns leases,
// snapshots and change protection for the chosen folder.
func (h *Handler) workspaceRoot() string {
	if h.lookupEnv != nil {
		root, _ := h.lookupEnv("MAGENT_WORKSPACE_ROOT")
		return root
	}
	return ""
}

func (h *Handler) checkedWorkspace(path string) (string, error) {
	root := h.workspaceRoot()
	if root == "" {
		root = h.baseDir
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("choose an existing, accessible folder")
	}
	if h.workspaceRoot() != "" {
		boundary, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", fmt.Errorf("the mounted workspace is unavailable")
		}
		rel, err := filepath.Rel(boundary, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("choose a folder inside %s; host paths must be configured in Docker", root)
		}
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("choose an existing directory")
	}
	return resolved, nil
}

func (h *Handler) selectWorkspace(ctx context.Context, input LineInput, cfg config.Config, view *interactiveView) (config.Config, bool, error) {
	root := h.workspaceRoot()
	if root == "" {
		root = h.baseDir
	}
	// List only immediate children; never crawl every project to draw a menu.
	dir, err := os.Open(root)
	if err != nil {
		return cfg, false, fmt.Errorf("cannot open workspace folder")
	}
	entries, readErr := dir.ReadDir(101)
	dir.Close()
	if readErr != nil && len(entries) == 0 {
		entries = nil
	}
	choices := []string{root}
	var menu strings.Builder
	menu.WriteString("\n  SELECT YOUR WORKSPACE\n  Files are edited directly in the selected folder. Git is optional.\n")
	fmt.Fprintf(&menu, "  0. %s (whole mounted folder)\n", terminalText(root))
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && len(choices) < 31 {
			choices = append(choices, filepath.Join(root, entry.Name()))
			fmt.Fprintf(&menu, "  %d. %s\n", len(choices)-1, terminalText(entry.Name()))
		}
	}
	menu.WriteString("  Enter a number or a relative folder path (for example apps/api).\n  /cancel leaves the selection unchanged. /workspace switches later.\n")
	if err := interactiveWrite(h.stdout, menu.String()); err != nil {
		return cfg, false, err
	}
	for {
		if err := interactiveWrite(h.stdout, "  Folder > "); err != nil {
			return cfg, false, err
		}
		line, err := input.ReadLine(ctx, cfg.MaxTaskBytes)
		if err != nil {
			return cfg, false, err
		}
		line = strings.TrimSpace(line)
		if line == "/cancel" {
			return cfg, false, nil
		}
		if line == "" {
			continue // No implicit consent to work across every mounted project.
		}
		if index, err := strconv.Atoi(line); err == nil && index >= 0 && index < len(choices) {
			line = choices[index]
		}
		path, err := h.checkedWorkspace(line)
		if err != nil {
			if err := view.notice(err.Error(), true); err != nil {
				return cfg, false, err
			}
			continue
		}
		cfg.WorkingDir, cfg.SessionID = path, ""
		return cfg, true, view.notice("Workspace: "+path+". Changes affect your original files.", false)
	}
}
