package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"multiharness-core/internal/config"
)

var errWorkspaceMountChanged = errors.New("workspace mount changed")

type savedWorkspace struct {
	Path    string `json:"path"`
	MountID string `json:"mount_id,omitempty"`
}

func (h *Handler) workspaceMountID() string {
	if h.lookupEnv == nil {
		return ""
	}
	id, _ := h.lookupEnv("MAGENT_WORKSPACE_ID")
	return id
}

// Keep the selected child path and bind identity separate from portable team
// defaults. A different host bind requires an explicit project selection.
func (h *Handler) restoreWorkspace(settingsPath string) (string, error) {
	data, err := config.ReadFile(filepath.Join(filepath.Dir(settingsPath), "workspace.json"), 65536)
	if err != nil {
		return "", err
	}
	var selection savedWorkspace
	if err := json.Unmarshal(data, &selection); err != nil {
		return "", err
	}
	if mountID := h.workspaceMountID(); mountID != "" && selection.MountID != mountID {
		return "", errWorkspaceMountChanged
	}
	if selection.Path == "" || filepath.IsAbs(selection.Path) {
		return "", errors.New("invalid saved workspace")
	}
	return h.checkedWorkspace(selection.Path)
}

func (h *Handler) rememberWorkspace(settingsPath, path string) error {
	if h.workspaceRoot() == "" {
		return nil
	}
	checked, err := h.checkedWorkspace(path)
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(h.workspaceRoot())
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, checked)
	if err != nil {
		return err
	}
	data, err := json.Marshal(savedWorkspace{Path: relative, MountID: h.workspaceMountID()})
	if err != nil {
		return err
	}
	directory := filepath.Dir(settingsPath)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".workspace-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(directory, "workspace.json"))
}
