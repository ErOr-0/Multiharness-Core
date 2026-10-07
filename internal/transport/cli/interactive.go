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
	"unicode/utf8"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/adapter/agent/activity"
	decisionadapter "multiharness-core/internal/adapter/decision/openrouter"
	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/history"
	"multiharness-core/internal/transport/cli/progress"
	"multiharness-core/internal/transport/cli/screen"
	"multiharness-core/internal/transport/cli/term"
)

// LineInput reads one bounded line without reading ahead into a later consent
// prompt. Production uses the same cancellation-aware terminal reader as consent.
type LineInput interface {
	ReadLine(context.Context, int) (string, error)
}

// Interactive is a terminal transport over the same runWorkflow entry point.
// Direct follow-ups reuse only the selected agent and workspace session.
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
	var cfg config.Config
	var resetRoles []string
	var err error
	if filename == settingsPath {
		// Settings this app saved are migrated, never a reason to refuse to start.
		if cfg, resetRoles, err = config.LoadPersonal(filename, h.baseDir, h.lookupEnv, overrides); err == nil && len(resetRoles) > 0 {
			err = saveInteractiveConfig(filename, cfg)
		}
	} else {
		cfg, err = config.Load(filename, h.baseDir, h.lookupEnv, overrides)
	}
	if err != nil {
		_, _ = fmt.Fprintln(h.stderr, terminalText(err.Error()))
		return ExitUsage
	}
	view := &screen.View{Writer: h.stdout}
	view.Configure(cfg, h.lookupEnv)
	if err := view.Welcome(cfg); err != nil {
		return ExitFailed
	}
	if len(resetRoles) > 0 {
		if err := view.Notice("OpenCode is no longer supported. Reset to the default agent: "+strings.Join(resetRoles, ", ")+". Use /config to choose another.", true); err != nil {
			return ExitFailed
		}
	}
	if h.workspaceRoot() != "" {
		path, restoreErr := h.restoreWorkspace(settingsPath)
		if restoreErr == nil {
			cfg.WorkingDir, cfg.SessionID = path, ""
			if err := view.Notice("Workspace restored: "+path, false); err != nil {
				return ExitFailed
			}
		} else {
			if errors.Is(restoreErr, errWorkspaceMountChanged) {
				if err := view.Notice("Shared PC folder changed. Choose a workspace in the new mount.", false); err != nil {
					return ExitFailed
				}
			} else if !errors.Is(restoreErr, os.ErrNotExist) {
				if err := view.Notice("Saved workspace is unavailable. Select a folder inside the current mount.", true); err != nil {
					return ExitFailed
				}
			}
			var selected bool
			cfg, selected, err = h.selectWorkspace(ctx, input, cfg, view)
			if ctx.Err() != nil {
				return ExitCancelled
			}
			if errors.Is(err, io.EOF) || (err == nil && !selected) {
				return ExitSuccess
			}
			if err != nil {
				_ = view.Notice(err.Error(), true)
				return ExitFailed
			}
			if err := h.rememberWorkspace(settingsPath, cfg.WorkingDir); err != nil {
				_ = view.Notice("Cannot save workspace selection: "+err.Error(), true)
				return ExitFailed
			}
		}
		overrides["workdir"], overrides["session-id"] = cfg.WorkingDir, ""
		if filename == "" {
			if err := view.Notice("First run: configure your agent. Your choices save automatically.", false); err != nil {
				return ExitFailed
			}
			var completed bool
			cfg, completed, err = h.configureInteractive(ctx, input, filename, settingsPath, overrides, cfg, view)
			if ctx.Err() != nil {
				return ExitCancelled
			}
			if errors.Is(err, io.EOF) || (err == nil && !completed) {
				return ExitSuccess
			}
			if err != nil {
				_ = view.Notice(err.Error(), true)
				return ExitFailed
			}
		}
	}
	if _, err := h.readiness(ctx, cfg, view, false); err != nil {
		if ctx.Err() != nil {
			return ExitCancelled
		}
		return ExitFailed
	}
	archive, err := history.Open(historyPath(settingsPath))
	if err != nil {
		_ = view.Notice("Cannot open private conversation history: "+err.Error(), true)
		return ExitFailed
	}
	defer archive.Close()
	conversationID, err := archive.Resume(cfg.WorkingDir)
	if err != nil {
		_ = view.Notice("Cannot resume conversation history: "+err.Error(), true)
		return ExitFailed
	}
	teamTurns, err := recentTeamTurns(archive, conversationID)
	if err != nil {
		_ = view.Notice("Saved conversation is damaged: "+err.Error(), true)
		return ExitFailed
	}
	focusedPlanID, err := archive.Focus(conversationID)
	if err != nil {
		_ = view.Notice("Cannot restore plan selection: "+err.Error(), true)
		return ExitFailed
	}
	var lastFailures []activity.Event
	var lastFailureCount uint64
	for {
		if ctx.Err() != nil {
			return ExitCancelled
		}
		view.Configure(cfg, h.lookupEnv)
		if err := view.Prompt(); err != nil {
			return ExitFailed
		}
		if styled, ok := input.(interface{ SetCommandView(*screen.View) }); ok {
			styled.SetCommandView(view)
		}
		var line string
		var err error
		if commands, ok := input.(interface {
			ReadCommand(context.Context, int, func(string) []string) (string, error)
		}); ok {
			line, err = commands.ReadCommand(ctx, cfg.MaxTaskBytes, CommandSuggestions)
		} else {
			line, err = input.ReadLine(ctx, cfg.MaxTaskBytes)
		}
		if ctx.Err() != nil {
			return ExitCancelled
		}
		if errors.Is(err, io.EOF) {
			return ExitSuccess
		}
		if err != nil {
			if errors.Is(err, term.ErrInputTooLong) {
				if term.Write(h.stdout, "Input too long; task was not started.\n") != nil {
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
		if strings.HasPrefix(line, "/") && !strings.HasPrefix(strings.ToLower(line), "/plan ") {
			command, value := splitInteractiveWord(line)
			command = strings.ToLower(command)
			if value != "" && (command == "/save" || command == "/quit" || command == "/exit" || command == "/config" || command == "/setup" || command == "/settings" || command == "/configuration" || command == "/help" || command == "/options" || command == "/diagnostics" || command == "/failures") {
				if view.Notice(command+" does not take arguments. Use /help for examples.", true) != nil {
					return ExitFailed
				}
				continue
			}
			previousConfig := cfg
			var commandErr error
			switch command {
			case "/plan":
				commandErr = errors.New("use /plan REQUEST")
			case "/new":
				if value != "" {
					commandErr = errors.New("/new does not take arguments")
					break
				}
				cfg.SessionID, overrides["session-id"] = "", ""
				teamTurns = nil
				conversationID, commandErr = archive.NewConversation(cfg.WorkingDir)
				if commandErr != nil {
					break
				}
				focusedPlanID = ""
				commandErr = view.Notice("New conversation. The next task starts without prior agent context.", false)
			case "/plans":
				var plans []history.PlanMeta
				if value == "" {
					plans, commandErr = archive.ListPlans(cfg.WorkingDir, 12)
				} else {
					plans, commandErr = archive.SearchPlans(cfg.WorkingDir, value, 12)
				}
				if commandErr == nil {
					commandErr = view.Notice(formatPlans(plans), false)
				}
			case "/use":
				if value == "" {
					commandErr = errors.New("use /use PLAN_ID")
					break
				}
				var plan contract.Plan
				plan, _, commandErr = archive.LoadPlan(cfg.WorkingDir, value)
				if commandErr == nil {
					commandErr = archive.SetFocus(conversationID, plan.ID)
				}
				if commandErr == nil {
					focusedPlanID = plan.ID
					commandErr = view.Notice(fmt.Sprintf("Selected plan %s (v%d): %s", plan.ID, plan.Version, plan.Title), false)
				}
			case "/history":
				var turns []history.Turn
				if value == "" {
					turns, commandErr = archive.Recent(conversationID, 8)
				} else {
					turns, commandErr = archive.SearchWorkspaceTurns(cfg.WorkingDir, value, 8)
				}
				if commandErr == nil {
					commandErr = view.Notice(formatHistory(turns), false)
				}
			case "/quit", "/exit":
				return ExitSuccess
			case "/help":
				commandErr = view.Help()
			case "/diagnostics":
				commandErr = view.Notice(lastProviderDiagnostic(filepath.Dir(settingsPath)))
			case "/failures":
				commandErr = view.FailureDetails(ctx, input, lastFailures, lastFailureCount)
			case "/setup":
				commandErr = h.completeAccountSetup(ctx, input, cfg, view)
			case "/configuration":
				commandErr = view.Settings(cfg)
				if commandErr == nil {
					_, commandErr = h.readiness(ctx, cfg, view, false)
				}
			case "/settings":
				commandErr = view.Settings(cfg)
			case "/permissions":
				cfg, commandErr = h.configurePermissions(ctx, input, value, filename, settingsPath, overrides, cfg, view)
			case "/options":
				for _, option := range config.Options() {
					if commandErr = term.Write(h.stdout, option.Name+" — "+option.Help+"\n"); commandErr != nil {
						break
					}
				}
			case "/login":
				if value == "jev" || value == "laya" {
					name := decisionadapter.ProviderName(value)
					if cfg.Mode != "team" || !cfg.Decision.Enabled {
						commandErr = errors.New(name + " is not required for this workflow")
					} else if provider := cfg.Decision.Effective().Provider; provider != value {
						commandErr = fmt.Errorf("the decision provider is %s; use /login %s, or /set decision-provider %s first", provider, provider, value)
					} else if h.loginDecisionKey == nil {
						commandErr = errors.New(name + " key input is unavailable")
					} else {
						commandErr = h.loginDecisionKey(ctx, cfg.Decision)
						if commandErr == nil {
							_, commandErr = h.readiness(ctx, cfg, view, false)
						}
					}
				} else if !supportedHarness(value) {
					commandErr = errors.New("use /login codex, /login claude, /login muse, /login jev or /login laya")
				} else {
					commandErr = h.loginSelected(ctx, cfg, value)
					if commandErr == nil {
						_, commandErr = h.readiness(ctx, cfg, view, false)
					}
				}
			case "/config":
				if h.workspaceRoot() != "" {
					cfg, commandErr = h.configureContainer(ctx, input, filename, settingsPath, overrides, cfg, view)
				} else {
					cfg, _, commandErr = h.configureInteractive(ctx, input, filename, settingsPath, overrides, cfg, view)
				}
			case "/workspace":
				var selected bool
				if value != "" {
					commandErr = errors.New("use /workspace without arguments to select a folder")
					break
				}
				cfg, selected, commandErr = h.selectWorkspace(ctx, input, cfg, view)
				if commandErr == nil && selected {
					overrides["workdir"], overrides["session-id"] = cfg.WorkingDir, ""
					commandErr = h.rememberWorkspace(settingsPath, cfg.WorkingDir)
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
				if option.Name == "reviewer-harness" {
					selectInteractiveReviewer(candidate, cfg, setting)
				}
				if option.Name == "implementer-harness" {
					selectInteractiveImplementer(candidate, cfg, setting)
				}
				var updated config.Config
				updated, commandErr = config.Load(filename, h.baseDir, h.lookupEnv, candidate)
				if commandErr == nil {
					var model string
					if model, commandErr = h.checkModelSetting(ctx, updated, option.Name, setting, view); commandErr == nil && model != setting {
						candidate[option.Name] = model
						updated, commandErr = config.Load(filename, h.baseDir, h.lookupEnv, candidate)
					}
				}
				if commandErr == nil {
					overrides, cfg = candidate, updated
					view.Configure(cfg, h.lookupEnv)
					message := option.Name + " updated. /save to remember it."
					if switchedPlanner {
						message = "Planner provider changed with matching model/executable defaults. /config to customize; /save to remember."
					}
					commandErr = view.Notice(message, false)
				} else if option.Name == "decision-enabled" && (setting == "jev" || setting == "laya") {
					// A provider name on the on/off switch is the most likely slip.
					commandErr = fmt.Errorf("decision-enabled is the on/off switch and takes true or false\nTo choose the decision model, use /set decision-provider %s (then /set decision-enabled true if routing is off)\nCurrent settings kept", setting)
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
					view.Configure(cfg, h.lookupEnv)
					commandErr = view.Settings(cfg)
				}
			case "/save":
				commandErr = h.rememberWorkspace(settingsPath, cfg.WorkingDir)
				if commandErr == nil {
					commandErr = saveInteractiveConfig(settingsPath, cfg)
				}
				if commandErr == nil {
					commandErr = view.Notice("Saved settings. Container launches remember the selected workspace.", false)
				}
			default:
				commandErr = fmt.Errorf("unknown command %q.%s Use /help for commands", terminalText(command), spellingSuggestion(command, []string{"/setup", "/configuration", "/config", "/new", "/plans", "/use", "/history", "/login", "/workspace", "/settings", "/permissions", "/diagnostics", "/failures", "/set", "/load", "/save", "/options", "/help", "/quit", "/exit"}))
			}
			if commandErr == nil && command == "/config" {
				_, commandErr = h.readiness(ctx, cfg, view, false)
			}
			if commandErr == nil && (command == "/set" || command == "/load" || command == "/save") {
				_, commandErr = h.readiness(ctx, cfg, view, true)
			}
			if commandErr == nil && !sameConversation(previousConfig, cfg) {
				cfg.SessionID, overrides["session-id"] = "", ""
				if previousConfig.WorkingDir != cfg.WorkingDir {
					conversationID, commandErr = archive.Resume(cfg.WorkingDir)
					if commandErr == nil {
						teamTurns, commandErr = recentTeamTurns(archive, conversationID)
					}
					if commandErr == nil {
						focusedPlanID, commandErr = archive.Focus(conversationID)
					}
				} else if previousConfig.Mode != cfg.Mode {
					teamTurns = nil
					conversationID, commandErr = archive.NewConversation(cfg.WorkingDir)
					focusedPlanID = ""
				}
			}
			if commandErr != nil {
				if errors.Is(commandErr, term.ErrOutput) {
					return ExitFailed
				}
				if ctx.Err() != nil {
					return ExitCancelled
				}
				if errors.Is(commandErr, io.EOF) {
					return ExitSuccess
				}
				if view.Notice(commandErr.Error(), true) != nil {
					return ExitFailed
				}
			}
			continue
		}
		if h.workspaceRoot() != "" {
			path, err := h.checkedWorkspace(cfg.WorkingDir)
			if err != nil {
				if view.Notice(err.Error()+". Use /workspace to select a folder.", true) != nil {
					return ExitFailed
				}
				continue
			}
			cfg.WorkingDir = path
		}
		forcedPlanOnly := strings.HasPrefix(strings.ToLower(line), "/plan ")
		if forcedPlanOnly {
			line = strings.TrimSpace(line[len("/plan "):])
		}
		in := contract.TaskInput{Task: line, WorkingDir: cfg.WorkingDir, MaxRepairAttempts: cfg.MaxRepairAttempts, SessionID: cfg.SessionID}
		in.PlanOnly = forcedPlanOnly || explicitPlanRequest(line)
		selectedPlanID, candidates, selectErr := resolvePlanID(archive, cfg.WorkingDir, line, focusedPlanID)
		if selectErr != nil {
			if view.Notice("Cannot search saved plans: "+selectErr.Error(), true) != nil {
				return ExitFailed
			}
			continue
		}
		if len(candidates) > 1 {
			if view.Notice("Several plans match. Select one with /use PLAN_ID, then repeat your request.\n"+formatPlans(candidates), true) != nil {
				return ExitFailed
			}
			continue
		}
		in.PlanArtifactID = history.NewArtifactID("plan")
		in.CaseArtifactID = history.NewArtifactID("case")
		in.ImplementationArtifactID = history.NewArtifactID("impl")
		if cfg.Mode == "team" && selectedPlanID != "" {
			plan, savedHead, loadErr := archive.LoadPlan(cfg.WorkingDir, selectedPlanID)
			if loadErr != nil {
				if view.Notice("Cannot load selected plan: "+loadErr.Error()+". Use /plans or /use PLAN_ID.", true) != nil {
					return ExitFailed
				}
				continue
			}
			in.SelectedPlanStale = savedHead != "" && savedHead != workspaceHead(cfg.WorkingDir)
			in.SelectedPlan = &plan
		}
		if cfg.Mode == "team" {
			in.RecentTurns = contract.SelectRecentTurns(line, teamTurns)
			in.RecentTurns = recallTurn(archive, conversationID, line, in.RecentTurns)
		}
		if err := in.Validate(); err != nil || !utf8.ValidString(line) || strings.ContainsRune(line, 0) {
			if term.Write(h.stdout, "Invalid task text.\n") != nil {
				return ExitFailed
			}
			continue
		}
		ready, err := h.readinessForTask(ctx, cfg, view)
		if ctx.Err() != nil {
			return ExitCancelled
		}
		if err != nil {
			return ExitFailed
		}
		if !ready {
			continue
		}
		p := newPresentation(h.stdout, h.stderr)
		p.human = view
		p.diagnosticDir = filepath.Dir(settingsPath)
		if control, ok := input.(progress.Control); ok {
			p.progress.Control = control
		}
		h.runWorkflow(ctx, cfg, in, p)
		lastFailures, lastFailureCount = p.progress.FailureDetails()
		if disclosure, ok := input.(interface {
			SetFailures([]activity.Event, uint64)
		}); ok {
			disclosure.SetFailures(lastFailures, lastFailureCount)
		}
		if lastFailureCount > 0 {
			if view.Notice(fmt.Sprintf("%d tool failure event(s) captured · click ▶ beside the next prompt or use /failures", lastFailureCount), true) != nil {
				return ExitFailed
			}
		}
		savedTurn := false
		if p.output.Status != "" {
			assistant := displayedReply(p.output)
			planID, saveErr := archive.SaveTurn(conversationID, line, assistant, p.output, selectedPlanID, workspaceHead(cfg.WorkingDir))
			if saveErr != nil {
				if view.Notice("Conversation was not saved: "+saveErr.Error(), true) != nil {
					return ExitFailed
				}
			} else {
				savedTurn = true
				if planID != "" {
					focusedPlanID = planID
					savedPlan, _, loadErr := archive.LoadPlan(cfg.WorkingDir, planID)
					if loadErr != nil {
						if view.Notice("Plan was indexed but cannot be read: "+loadErr.Error(), true) != nil {
							return ExitFailed
						}
					}
					if loadErr == nil && view.Notice(fmt.Sprintf("Plan saved as %s (v%d). Use /use %s later.", planID, savedPlan.Version, planID), false) != nil {
						return ExitFailed
					}
				}
			}
		}
		if cfg.Mode == "direct" && p.output.Direct != nil && p.output.Direct.SessionID != "" {
			cfg.SessionID = p.output.Direct.SessionID
			overrides["session-id"] = cfg.SessionID
		}
		p.progress.Stop()
		if err := h.rememberAuthenticationFailure(cfg, p.output, view); err != nil {
			return ExitFailed
		}
		if p.outputErr != nil {
			return ExitFailed
		}
		if cfg.Mode == "team" {
			if savedTurn {
				if saved, e := recentTeamTurns(archive, conversationID); e == nil {
					teamTurns = saved
				} else {
					teamTurns = contract.AppendTurn(teamTurns, line, p.output)
				}
			} else {
				teamTurns = contract.AppendTurn(teamTurns, line, p.output)
			}
		}
		if _, err := p.progress.Failure(); err != nil {
			return ExitFailed
		}
	}
}

func (h *Handler) configureInteractive(ctx context.Context, input LineInput, filename, settingsPath string, overrides map[string]string, cfg config.Config, view *screen.View) (config.Config, bool, error) {
	candidate := maps.Clone(overrides)
	updated := cfg
	heading := "CONFIGURE YOUR TEAM"
	if cfg.Mode == "direct" {
		heading = "CONFIGURE YOUR AGENT"
	}
	modeHelp := "Team mode: configure a planner, implementer and reviewer separately."
	if cfg.Mode == "direct" {
		modeHelp = "Direct mode: one agent handles the task. For separate planner/implementer/reviewer roles, use /set mode team."
	}
	if err := view.Print("\n" + view.Paragraph(heading, 2, "1;36") + "  " + view.Rule() + "\n" + view.Paragraph(modeHelp, 4, "0") + view.Paragraph("Enter keeps a value · /cancel discards this setup", 4, "2")); err != nil {
		return cfg, false, err
	}
	roles := []string{"planner", "implementer", "reviewer"}
	if cfg.Mode == "direct" {
		roles = []string{"implementer"}
	}
	steps := len(roles) * 3
	catalogs := map[account.Request]modelChoices{}
	for step := range steps {
		role := roles[step/3]
		selected := updated.Planner
		if role == "implementer" {
			selected = config.Planner(updated.Implementer)
		}
		if role == "reviewer" {
			selected = updated.Reviewer
		}
		displayRole := role
		if cfg.Mode == "direct" {
			displayRole = "agent"
		}
		option, label, current := role+"-harness", displayRole+": codex, claude or muse", selected.Harness
		var models modelChoices
		switch step % 3 {
		case 1:
			option, label, current = role+"-model", screen.HarnessName(selected.Harness)+" "+displayRole+" model", selected.Model
			var err error
			if models, err = h.modelChoices(ctx, updated, selected, view, catalogs); err != nil {
				return cfg, false, err
			}
			label += models.menu(current, view.ContentWidth())
		case 2:
			option, label, current = role+"-reasoning", screen.HarnessName(selected.Harness)+" "+displayRole+" reasoning", selected.Reasoning
			for index, choice := range reasoningChoices(selected.Harness) {
				label += fmt.Sprintf("\n    %d. %s", index+1, choice)
			}
			label += "\n  Choose a number or name. Higher effort can take longer. Enter keeps the value shown."
		}

		for {
			display := current
			if display == "" {
				display = "CLI default"
			}
			if err := view.Print("\n" + view.Paragraph(fmt.Sprintf("%d/%d · %s", step+1, steps, label), 4, "1;36") + view.Paragraph("Current: "+display, 4, "2") + "  " + view.Paint("❯ ", "1;36")); err != nil {
				return cfg, false, err
			}
			var value string
			var err error
			if strings.HasSuffix(option, "-model") {
				value, err = readModelAnswer(ctx, input, cfg.MaxTaskBytes, models)
			} else {
				value, err = input.ReadLine(ctx, cfg.MaxTaskBytes)
			}
			if err != nil {
				if errors.Is(err, term.ErrInputTooLong) {
					if view.Notice("Value too long. Retype this field; earlier answers are kept.", true) != nil {
						return cfg, false, term.ErrOutput
					}
					continue
				}
				return cfg, false, err
			}
			value = strings.TrimSpace(value)
			if strings.EqualFold(value, "/cancel") {
				return cfg, false, view.Notice("Setup cancelled. Previous configuration kept.", false)
			}
			if value == "" {
				if err := models.keep(current); err != nil {
					if err := view.Notice(err.Error()+".", true); err != nil {
						return cfg, false, err
					}
					continue
				}
				break
			}
			if strings.HasSuffix(option, "-reasoning") {
				value = reasoningSelection(selected.Harness, value)
			}
			if strings.HasSuffix(option, "-model") {
				if value, err = models.selection(value); err != nil {
					if err := view.Notice(err.Error()+". Earlier answers are kept.", true); err != nil {
						return cfg, false, err
					}
					continue
				}
			}
			_, normalized, err := interactiveSetting(option + " " + value)
			if err == nil {
				trial := maps.Clone(candidate)
				trial[option] = normalized
				if option == "planner-harness" {
					selectInteractivePlanner(trial, updated, normalized)
				}
				if option == "reviewer-harness" {
					selectInteractiveReviewer(trial, updated, normalized)
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
			if err := view.Notice(err.Error()+". Please retry this field; earlier answers are kept.", true); err != nil {
				return cfg, false, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return cfg, false, err
	}
	if err := saveInteractiveConfig(settingsPath, updated); err != nil {
		return cfg, false, fmt.Errorf("cannot save team settings: %w", err)
	}
	maps.Copy(overrides, candidate)
	if err := view.Notice("Settings saved. Checking workflow prerequisites.", false); err != nil {
		return updated, true, err
	}
	return updated, true, h.completeAccountSetup(ctx, input, updated, view)
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
func terminalText(value string) string { return contract.PlainText(value) }
