package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

var errInputTooLong = errors.New("input exceeds the configured byte limit")
var errInteractiveOutput = errors.New("terminal output failed")

// LineInput reads one bounded line without reading ahead into a later consent
// prompt. Production uses the same cancellation-aware terminal reader as consent.
type LineInput interface {
	ReadLine(context.Context, int) (string, error)
}

// Interactive is a terminal transport over the same runWorkflow entry point.
// Each submitted task is independent; provider history is never implied here.
func (h *Handler) Interactive(ctx context.Context, input LineInput, settingsPath string) int {
	if ctx == nil || input == nil || !filepath.IsAbs(settingsPath) {
		return ExitUsage
	}
	filename := ""
	if _, err := os.Stat(settingsPath); err == nil {
		filename = settingsPath
	} else if !errors.Is(err, os.ErrNotExist) {
		_, _ = fmt.Fprintln(h.stderr, "Cannot read personal settings.")
		return ExitUsage
	}
	if h.lookupEnv != nil {
		if value, ok := h.lookupEnv("MULTIHARNESS_CONFIG"); ok {
			filename = value
		}
	}
	overrides := map[string]string{}
	cfg, err := config.Load(filename, h.baseDir, h.lookupEnv, overrides)
	if err != nil {
		_, _ = fmt.Fprintln(h.stderr, terminalText(err.Error()))
		return ExitUsage
	}
	view := &interactiveView{writer: h.stdout}
	view.configure(cfg, h.lookupEnv)
	if err := view.welcome(cfg); err != nil {
		return ExitFailed
	}
	if h.workspaceRoot() != "" {
		var selected bool
		cfg, selected, err = h.selectWorkspace(ctx, input, cfg, view)
		if ctx.Err() != nil {
			return ExitCancelled
		}
		if errors.Is(err, io.EOF) || (err == nil && !selected) {
			return ExitSuccess
		}
		if err != nil {
			_ = view.notice(err.Error(), true)
			return ExitFailed
		}
		overrides["workdir"], overrides["session-id"] = cfg.WorkingDir, ""
	}
	for {
		if ctx.Err() != nil {
			return ExitCancelled
		}
		view.configure(cfg, h.lookupEnv)
		if err := view.prompt(); err != nil {
			return ExitFailed
		}
		line, err := input.ReadLine(ctx, cfg.MaxTaskBytes)
		if ctx.Err() != nil {
			return ExitCancelled
		}
		if errors.Is(err, io.EOF) {
			return ExitSuccess
		}
		if err != nil {
			if errors.Is(err, errInputTooLong) {
				if interactiveWrite(h.stdout, "Input too long; task was not started.\n") != nil {
					return ExitFailed
				}
				continue
			}
			return ExitFailed
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			command, value := splitInteractiveWord(line)
			command = strings.ToLower(command)
			if value != "" && (command == "/save" || command == "/quit" || command == "/exit" || command == "/config" || command == "/settings" || command == "/help" || command == "/options") {
				if view.notice(command+" does not take arguments. Use /help for examples.", true) != nil {
					return ExitFailed
				}
				continue
			}
			var commandErr error
			switch command {
			case "/quit", "/exit":
				return ExitSuccess
			case "/help":
				commandErr = view.help()
			case "/settings":
				commandErr = view.settings(cfg)
			case "/options":
				for _, option := range config.Options() {
					if commandErr = interactiveWrite(h.stdout, option.Name+" — "+option.Help+"\n"); commandErr != nil {
						break
					}
				}
			case "/config":
				cfg, commandErr = h.configureInteractive(ctx, input, filename, overrides, cfg, view)
			case "/workspace":
				var selected bool
				if value != "" {
					commandErr = errors.New("use /workspace without arguments to select a folder")
					break
				}
				cfg, selected, commandErr = h.selectWorkspace(ctx, input, cfg, view)
				if commandErr == nil && selected {
					overrides["workdir"], overrides["session-id"] = cfg.WorkingDir, ""
				}
			case "/set":
				var option config.Option
				var setting string
				option, setting, commandErr = interactiveSetting(value)
				if commandErr != nil {
					break
				}
				switchedPlanner := option.Name == "planner-harness" && setting != cfg.Planner.Harness
				candidate := maps.Clone(overrides)
				candidate[option.Name] = setting
				if option.Name == "planner-harness" {
					selectInteractivePlanner(candidate, cfg, setting)
				}
				if option.Name == "implementer-harness" {
					selectInteractiveImplementer(candidate, cfg, setting)
				}
				var updated config.Config
				updated, commandErr = config.Load(filename, h.baseDir, h.lookupEnv, candidate)
				if commandErr == nil {
					overrides, cfg = candidate, updated
					view.configure(cfg, h.lookupEnv)
					message := option.Name + " updated. /save to remember it."
					if switchedPlanner {
						message = "Planner provider changed with matching model/executable defaults. /config to customize; /save to remember."
					}
					commandErr = view.notice(message, false)
				} else {
					commandErr = fmt.Errorf("%s\n%s\nCurrent settings kept; try /set %s VALUE again", commandErr, option.Help, option.Name)
				}
			case "/load":
				if value == "" {
					commandErr = errors.New("use /load PATH")
					break
				}
				var updated config.Config
				value, commandErr = normalizeSetting(config.Option{Name: "config"}, value)
				if commandErr != nil {
					break
				}
				if value == "" {
					commandErr = errors.New("use /load PATH; the configuration path cannot be empty")
					break
				}
				updated, commandErr = config.Load(value, h.baseDir, h.lookupEnv, nil)
				if commandErr == nil {
					filename, overrides, cfg = value, map[string]string{}, updated
					view.configure(cfg, h.lookupEnv)
					commandErr = view.settings(cfg)
				}
			case "/save":
				commandErr = saveInteractiveConfig(settingsPath, cfg)
				if commandErr == nil {
					commandErr = view.notice("Saved personal defaults. Future launches use their current directory.", false)
				}
			default:
				commandErr = fmt.Errorf("unknown command %q.%s Use /help for commands", terminalText(command), spellingSuggestion(command, []string{"/config", "/settings", "/set", "/load", "/save", "/options", "/help", "/quit", "/exit"}))
			}
			if commandErr != nil {
				if errors.Is(commandErr, errInteractiveOutput) {
					return ExitFailed
				}
				if ctx.Err() != nil {
					return ExitCancelled
				}
				if errors.Is(commandErr, io.EOF) {
					return ExitSuccess
				}
				if view.notice(commandErr.Error(), true) != nil {
					return ExitFailed
				}
			}
			continue
		}
		if h.workspaceRoot() != "" {
			path, err := h.checkedWorkspace(cfg.WorkingDir)
			if err != nil {
				if view.notice(err.Error()+". Use /workspace to select a folder.", true) != nil {
					return ExitFailed
				}
				continue
			}
			cfg.WorkingDir = path
		}
		in := store.TaskInput{Task: line, WorkingDir: cfg.WorkingDir, MaxRepairAttempts: cfg.MaxRepairAttempts, SessionID: cfg.SessionID}
		if err := in.Validate(); err != nil || !utf8.ValidString(line) || strings.ContainsRune(line, 0) {
			if interactiveWrite(h.stdout, "Invalid task text.\n") != nil {
				return ExitFailed
			}
			continue
		}
		p := newPresentation(h.stdout, h.stderr)
		p.human = view
		h.runWorkflow(ctx, cfg, in, p)
		p.progress.stop()
		if p.outputErr != nil {
			return ExitFailed
		}
		if err, _ := p.progress.failure(); err != nil {
			return ExitFailed
		}
	}
}

func (h *Handler) configureInteractive(ctx context.Context, input LineInput, filename string, overrides map[string]string, cfg config.Config, view *interactiveView) (config.Config, error) {
	candidate := maps.Clone(overrides)
	updated := cfg
	if h.workspaceRoot() != "" {
		var selected bool
		var err error
		updated, selected, err = h.selectWorkspace(ctx, input, cfg, view)
		if err != nil || !selected {
			return cfg, err
		}
		candidate["workdir"], candidate["session-id"] = updated.WorkingDir, ""
	}
	if err := interactiveWrite(h.stdout, "\n  "+view.paint("CONFIGURE YOUR TEAM", "1;36")+"\n  Enter keeps a value · /cancel discards this setup\n"); err != nil {
		return cfg, err
	}
	for step := range 5 {
		option, label, current := "planner-harness", "Planner: codex or opencode", updated.Planner.Harness
		switch step {
		case 1:
			option, label, current = "planner-model", "Codex planner model", updated.Planner.Model
			if updated.Planner.Harness == "opencode" {
				option, label, current = "planner-model", "OpenCode planner model (provider/model)", updated.Planner.Model
			}
		case 2:
			option, label, current = "implementer-harness", "Implementer: codex or opencode", updated.Implementer.Harness
		case 3:
			option, label, current = "implementer-model", "Codex implementation model", updated.Implementer.Model
			if updated.Implementer.Harness == "opencode" {
				label = "OpenCode implementation model (provider/model)"
			}
		case 4:
			option, label, current = "reviewer-model", "Codex reviewer model", updated.Reviewer.Model
		}
		for {
			display := current
			if display == "" {
				display = "CLI default"
			}
			if err := interactiveWrite(h.stdout, fmt.Sprintf("\n  %s %s\n  %s %s ", view.paint(fmt.Sprintf("%d/5", step+1), "2"), label, view.paint("["+terminalText(display)+"]", "2"), view.paint("❯", "36"))); err != nil {
				return cfg, err
			}
			value, err := input.ReadLine(ctx, cfg.MaxTaskBytes)
			if err != nil {
				if errors.Is(err, errInputTooLong) {
					if view.notice("Value too long. Retype this field; earlier answers are kept.", true) != nil {
						return cfg, errInteractiveOutput
					}
					continue
				}
				return cfg, err
			}
			value = strings.TrimSpace(value)
			if strings.EqualFold(value, "/cancel") {
				return cfg, view.notice("Setup cancelled. Previous configuration kept.", false)
			}
			if value == "" {
				break
			}
			_, normalized, err := interactiveSetting(option + " " + value)
			if err == nil {
				trial := maps.Clone(candidate)
				trial[option] = normalized
				if option == "planner-harness" {
					selectInteractivePlanner(trial, updated, normalized)
				}
				if option == "implementer-harness" {
					selectInteractiveImplementer(trial, updated, normalized)
				}
				var next config.Config
				next, err = config.Load(filename, h.baseDir, h.lookupEnv, trial)
				if err == nil {
					candidate, updated = trial, next
					break
				}
			}
			if err := view.notice(err.Error()+". Please retry this field; earlier answers are kept.", true); err != nil {
				return cfg, err
			}
		}
	}
	maps.Copy(overrides, candidate)
	return updated, view.notice("Team configured. /save to remember it; /set for other settings.", false)
}

func saveInteractiveConfig(filename string, cfg config.Config) error {
	// Personal defaults must not send a later launch to a previous checkout or
	// resume its implementation session. A task's working directory stays local.
	cfg.WorkingDir, cfg.SessionID = ".", ""
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(filename), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filename)
}

// Provider text is content, never terminal escape sequences.
func terminalText(value string) string {
	return strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, value)
}

func interactiveWrite(w io.Writer, value string) error {
	n, err := io.WriteString(w, value)
	if err == nil && n != len(value) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return errors.Join(errInteractiveOutput, err)
	}
	return err
}
