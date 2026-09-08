package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// Folder navigation is deliberately not a shell: only explicit directory
// operations run, with the same mounted-tree checks before every operation.
func (h *Handler) selectWorkspace(ctx context.Context, input LineInput, cfg config.Config, view *interactiveView) (config.Config, bool, error) {
	current := h.workspaceRoot()
	if current == "" {
		current = h.baseDir
	}
	if path, err := h.checkedWorkspace(cfg.WorkingDir); err == nil {
		current = path
	}
	for {
		choices, err := workspaceFolders(current)
		if err != nil {
			return cfg, false, err
		}
		var menu strings.Builder
		menu.WriteString("\n  CHOOSE A WORKSPACE\n")
		fmt.Fprintf(&menu, "  Current folder: %s\n", terminalText(current))
		menu.WriteString("  Press Enter to use this folder. Files here are edited directly.\n")
		for i, path := range choices {
			fmt.Fprintf(&menu, "  %d. %s/\n", i+1, terminalText(filepath.Base(path)))
		}
		if len(choices) == 50 {
			menu.WriteString("  Showing up to 50 folders. Use cd PATH for any folder not listed.\n")
		}
		if len(choices) == 0 {
			menu.WriteString("  No subfolders here. Use mkdir NAME to create one.\n")
		}
		menu.WriteString("  Number or cd PATH: open folder | cd ..: parent | mkdir NAME: create\n  ls: refresh | pwd: current path | /cancel: leave browser\n")
		if h.workspaceRoot() != "" {
			menu.WriteString("  " + h.hostFolderHelp() + "\n")
		}
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
			if line == "/cancel" || line == "/quit" {
				return cfg, false, nil
			}
			if line == "" || line == "0" || line == "select" {
				path, err := h.checkedWorkspace(current)
				if err != nil {
					return cfg, false, err
				}
				cfg.WorkingDir, cfg.SessionID = path, ""
				return cfg, true, view.notice("Workspace selected: "+path+". Use /config for your team or type a task.", false)
			}
			command, arg := splitInteractiveWord(line)
			if command == "/config" || command == "/help" {
				if err := view.notice("Press Enter to select the current folder first. Then /config opens your team settings. Use cd PATH or mkdir NAME here.", false); err != nil {
					return cfg, false, err
				}
				continue
			}
			if line == "ls" || line == "dir" {
				break
			}
			if line == "pwd" {
				if err := view.notice(current, false); err != nil {
					return cfg, false, err
				}
				continue
			}
			target := line
			create := command == "mkdir"
			if command == "cd" || create {
				target = strings.TrimSpace(arg)
				if target == "" {
					if err := view.notice("Supply a folder name, for example cd api or mkdir new-project.", true); err != nil {
						return cfg, false, err
					}
					continue
				}
			}
			if len(target) >= 2 && ((target[0] == '"' && target[len(target)-1] == '"') || (target[0] == '\'' && target[len(target)-1] == '\'')) {
				target = target[1 : len(target)-1]
			}
			if h.workspaceRoot() != "" && len(target) >= 3 && target[1] == ':' {
				if err := view.notice("That is a Windows host path. Docker sees your shared folder as /workspace. "+h.hostFolderHelp(), true); err != nil {
					return cfg, false, err
				}
				continue
			}
			if index, err := strconv.Atoi(target); command != "cd" && !create && err == nil && index > 0 && index <= len(choices) {
				target = choices[index-1]
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(current, target)
			}
			if create {
				// Check the existing parent before writing. Do not follow a destination
				// symlink or implicitly create a chain of directories outside this tree.
				parent, err := h.checkedWorkspace(filepath.Dir(target))
				if err == nil {
					err = os.Mkdir(filepath.Join(parent, filepath.Base(target)), 0755)
				}
				if err != nil {
					if err := view.notice("Cannot create folder: choose a new name under an existing, writable shared folder.", true); err != nil {
						return cfg, false, err
					}
					continue
				}
				if err := view.notice("Folder created in your original project. It remains even if you cancel selection.", false); err != nil {
					return cfg, false, err
				}
				break
			}
			path, err := h.checkedWorkspace(target)
			if err != nil {
				if err := view.notice(err.Error(), true); err != nil {
					return cfg, false, err
				}
				continue
			}
			current = path
			break
		}
	}
}

func workspaceFolders(path string) ([]string, error) {
	dir, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot list this folder; check its permissions")
	}
	defer dir.Close()
	entries, err := dir.ReadDir(1001)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("cannot list this folder; check its permissions")
	}
	paths := []string{}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			paths = append(paths, filepath.Join(path, entry.Name()))
		}
	}
	sort.Strings(paths)
	if len(paths) > 50 {
		paths = paths[:50]
	}
	return paths, nil
}

func (h *Handler) hostFolderHelp() string {
	return "Docker can browse only shared folders. Change the workspace bind in the saved Compose configuration and recreate this container to share another PC folder."
}
