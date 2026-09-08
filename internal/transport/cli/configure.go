package cli

import (
	"context"
	"errors"
	"io"
	"os"

	"multiharness-core/internal/config"
)

// ConfigureTeam exposes the existing team editor to the host launcher's menu.
// It makes no agent calls and never changes the selected host mount.
func (h *Handler) ConfigureTeam(ctx context.Context, input LineInput, settingsPath string) int {
	filename := settingsPath
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		filename = ""
	}
	overrides := map[string]string{}
	cfg, err := config.Load(filename, h.baseDir, h.lookupEnv, overrides)
	view := &interactiveView{writer: h.stdout}
	if err == nil {
		view.configure(cfg, h.lookupEnv)
		cfg, err = h.configureInteractive(ctx, input, filename, overrides, cfg, view)
	}
	if errors.Is(err, io.EOF) {
		return ExitSuccess
	}
	if err == nil {
		err = saveInteractiveConfig(settingsPath, cfg)
	}
	if err != nil {
		_ = view.notice(err.Error(), true)
		return ExitFailed
	}
	if view.notice("Models saved in Docker. Returning to host configuration.", false) != nil {
		return ExitFailed
	}
	return ExitSuccess
}
