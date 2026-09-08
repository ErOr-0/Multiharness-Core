package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"multiharness-core/internal/config"
)

// Remember only the selection within the mounted tree, separately from portable
// team defaults. Every restored selection is checked against the current mount.
func (h *Handler) restoreWorkspace(settingsPath string) (string, error) {
	data, err := config.ReadFile(filepath.Join(filepath.Dir(settingsPath), "workspace.json"), 65536)
	if err != nil {
		return "", err
	}
	var selection struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(data, &selection); err != nil {
		return "", err
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
	data, err := json.Marshal(struct {
		Path string `json:"path"`
	}{relative})
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
