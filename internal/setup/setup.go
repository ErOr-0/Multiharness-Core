// Package setup connects harness-bridge to a planner and undoes it: the setup
// command and its wizard, each planner's registration hooks, disconnect and
// uninstall.
package setup

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/config"
)

const installTimeout = 15 * time.Minute

type setupOptions struct {
	planner, implementer *agent.Adapter
	router               string
	dir                  string
	yes, lockPlanner     bool
}

// CmdSetup makes sure the chosen CLIs are installed, asking before running any
// installer. With both roles it connects harness-bridge to the planner for all
// projects; with --dir it writes instruction files into that project instead.
// Without flags in a terminal, it runs the wizard.
func CmdSetup(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		if f, ok := stdin.(*os.File); ok && cli.IsTerminal(f) {
			return CmdWizard(stdin, stdout, stderr)
		}
		return cli.UsageError(stdout, stderr, errors.New("give --planner and --implementer, or run setup in a terminal to be asked"))
	}
	opts, err := parseSetup(args)
	if err != nil {
		return cli.UsageError(stdout, stderr, err)
	}
	confirm := confirmer(stdin, stdout, opts.yes)
	ready := true
	seen := map[*agent.Adapter]bool{}
	for _, role := range []struct {
		name string
		a    *agent.Adapter
	}{{"planner", opts.planner}, {"implementer", opts.implementer}} {
		if role.a == nil || seen[role.a] {
			continue
		}
		seen[role.a] = true
		if !ensureInstalled(role.a, role.name, confirm, stdout, stderr) {
			ready = false
		}
	}
	both := opts.planner != nil && opts.implementer != nil
	switch {
	case opts.dir != "":
		// Agents call harness-bridge from their shell here, so it must be on PATH.
		if _, err := exec.LookPath("harness-bridge"); err != nil {
			exe, _ := os.Executable()
			fmt.Fprintf(stdout, "! harness-bridge itself is not on PATH, so agents cannot call it. Install it with the install script, or copy %s into a folder on PATH.\n", exe)
			ready = false
		}
		if both {
			if err := writeAgentFiles(opts, stdout); err != nil {
				fmt.Fprintf(stderr, "harness-bridge: %v\n", err)
				return cli.ExitFailed
			}
		}
	case both:
		if !connectPlanner(opts, stdout) {
			ready = false
		}
	}
	if !ready {
		fmt.Fprintln(stdout, "\nSetup is not complete; see the lines marked ✗ or ! above.")
		return cli.ExitFailed
	}
	fmt.Fprintln(stdout, "\nReady.")
	return cli.ExitOK
}

func parseSetup(args []string) (setupOptions, error) {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	planner := fs.String("planner", "", "")
	implementer := fs.String("implementer", "", "")
	router := fs.String("router", "", "")
	dir := fs.String("dir", "", "")
	yes := fs.Bool("yes", false, "")
	lock := fs.Bool("lock-planner", false, "")
	if err := fs.Parse(args); err != nil {
		return setupOptions{}, err
	}
	if fs.NArg() > 0 {
		return setupOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	opts := setupOptions{router: *router, dir: *dir, yes: *yes, lockPlanner: *lock}
	for _, role := range []struct {
		flag, value string
		into        **agent.Adapter
	}{{"planner", *planner, &opts.planner}, {"implementer", *implementer, &opts.implementer}} {
		if role.value == "" {
			continue
		}
		if *role.into = agent.Named(role.value); *role.into == nil {
			return setupOptions{}, fmt.Errorf("--%s %q: use one of %s", role.flag, role.value, agent.Names())
		}
	}
	switch {
	case opts.planner == nil && opts.implementer == nil:
		return setupOptions{}, errors.New("give --planner, --implementer or both")
	case opts.planner == agent.Claude && opts.implementer == agent.Claude:
		return setupOptions{}, errors.New("Claude Code cannot run itself headless from inside a session; use its own subagents instead")
	case opts.router != "" && opts.router != "jev" && opts.router != "laya":
		return setupOptions{}, fmt.Errorf("--router %q: use jev or laya", opts.router)
	case opts.lockPlanner && (opts.planner != agent.Claude || opts.implementer == nil || opts.dir == ""):
		return setupOptions{}, errors.New("--lock-planner works only with --planner claude, --implementer and --dir")
	case opts.dir == "" && opts.planner != nil && opts.implementer != nil && opts.planner.Planner == nil:
		return setupOptions{}, fmt.Errorf("%s can't load harness-bridge as an MCP server; add --dir PROJECT to write instructions into that project instead", opts.planner.Title)
	}
	if opts.dir == "" {
		return opts, nil
	}
	dirAbs, err := filepath.Abs(opts.dir)
	if err != nil {
		return setupOptions{}, err
	}
	if info, err := os.Stat(dirAbs); err != nil || !info.IsDir() {
		return setupOptions{}, fmt.Errorf("--dir %s is not a directory", opts.dir)
	}
	opts.dir = dirAbs
	return opts, nil
}

// connectPlanner registers harness-bridge with the planner for all projects
// and saves the choices, as the wizard does, without asking anything.
func connectPlanner(opts setupOptions, stdout io.Writer) bool {
	p, impl := opts.planner, opts.implementer
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stdout, "✗ Can't find harness-bridge itself: %v\n", err)
		return false
	}
	cli, ok := agent.FindBin(p.Bin)
	if !ok {
		fmt.Fprintf(stdout, "✗ Can't connect to %s until it is installed.\n", p.Title)
		return false
	}
	saved, _ := config.Load()
	cfg := config.Config{Planner: p.Name, Implementer: impl.Name, Router: opts.router}
	if saved.Implementer == impl.Name {
		cfg.Model, cfg.Allow = saved.Model, saved.Allow
	}
	w := &wizard{out: stdout}
	if !w.step(p.Planner.ConnectLabel(p), func() error { return p.Planner.Register(cli, exe, cfg) }) {
		return false
	}
	if p.Planner.Allow != nil && !w.step(p.Planner.AllowNote, p.Planner.Allow) {
		return false
	}
	path, err := config.Save(cfg)
	return w.step("Saved the choices in "+path, func() error { return err })
}

// confirmer asks on the terminal. Without one it refuses, so an agent running
// setup must get the user's agreement first and pass --yes.
func confirmer(stdin io.Reader, stdout io.Writer, yes bool) func(string) bool {
	reader := bufio.NewReader(stdin)
	return func(question string) bool {
		if yes {
			fmt.Fprintf(stdout, "%s yes (--yes)\n", question)
			return true
		}
		if f, ok := stdin.(*os.File); !ok || !cli.IsTerminal(f) {
			fmt.Fprintf(stdout, "%s not asked: no terminal. Ask the user, then rerun with --yes to accept.\n", question)
			return false
		}
		fmt.Fprintf(stdout, "%s [y/N] ", question)
		answer, _ := reader.ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}

func ensureInstalled(a *agent.Adapter, role string, confirm func(string) bool, stdout, stderr io.Writer) bool {
	if path, ok := agent.FindBin(a.Bin); ok {
		fmt.Fprintf(stdout, "✓ %s (%s) is installed: %s\n", a.Title, role, path)
		return true
	}
	in, ok := a.Installer()
	if !ok {
		fmt.Fprintf(stdout, "✗ %s (%s) is not installed, and there is no automatic installer for this system. Install it from %s, then sign in with `%s`.\n", a.Title, role, a.Docs, a.Login)
		return false
	}
	fmt.Fprintf(stdout, "%s (%s) is not installed. Its official installer is:\n\n    %s\n\n", a.Title, role, in.Display())
	if !confirm("Run it now?") {
		fmt.Fprintf(stdout, "✗ Skipped. Install %s yourself, then run setup again.\n", a.Title)
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, in.Command[0], in.Command[1:]...)
	cmd.Dir = os.TempDir() // keep the workspace's package-manager config out of it
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stdout, "✗ The %s installer failed: %v. Nothing was retried; see %s.\n", a.Title, err, a.Docs)
		return false
	}
	if _, ok := agent.FindBin(a.Bin); !ok {
		fmt.Fprintf(stdout, "! %s was installed, but `%s` is not on PATH in this terminal yet. Open a new terminal, then sign in with `%s`.\n", a.Title, a.Bin, a.Login)
		return false
	}
	fmt.Fprintf(stdout, "✓ %s is installed. Sign in with `%s` before the first job.\n", a.Title, a.Login)
	return true
}

const (
	blockBegin = "<!-- harness-bridge:begin (written by `harness-bridge setup`; rerunning it replaces this block) -->"
	blockEnd   = "<!-- harness-bridge:end -->"
)

func writeAgentFiles(opts setupOptions, stdout io.Writer) error {
	p, impl := opts.planner, opts.implementer
	if p == agent.Claude {
		path := filepath.Join(opts.dir, ".claude", "agents", subagentName(impl)+".md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(subagentFile(impl)), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ Wrote %s\n", path)
	}
	path := filepath.Join(opts.dir, p.Instructions)
	if err := upsertBlock(path, plannerInstructions(p, impl, opts.router)); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "✓ Added the plan-delegate-review instructions to %s\n", path)
	if opts.lockPlanner {
		path := filepath.Join(opts.dir, ".claude", "settings.json")
		if err := lockClaudePlanner(path); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ %s now stops Claude editing files itself, so every change goes through %s\n", path, impl.Title)
	}
	return nil
}

func subagentName(impl *agent.Adapter) string { return impl.Name + "-implementer" }

func subagentFile(impl *agent.Adapter) string {
	var b strings.Builder
	followUp, session := "", ""
	if impl.Resume {
		followUp = " For follow-up fixes, also give it the session_id from the previous report."
		session = " Add `--session <session_id>` only when you were given one."
	}
	fmt.Fprintf(&b, "---\nname: %s\ndescription: Hands an implementation plan to %s through harness-bridge and reports back what it did. Give it the complete plan.%s\ntools: Bash\nmodel: haiku\n---\n\n", subagentName(impl), impl.Title, followUp)
	fmt.Fprintf(&b, "You pass work from the main agent to %s and bring back its report.\nYou don't plan, edit files or judge the result.\n\n", impl.Title)
	fmt.Fprintf(&b, "1. Start the job. Put the plan you were given, unchanged, between the heredoc\n   markers.%s\n\n", session)
	fmt.Fprintf(&b, "   ```bash\n   harness-bridge start --to %s --dir . <<'HARNESS_BRIDGE_PROMPT'\n   <the plan, exactly as given>\n   HARNESS_BRIDGE_PROMPT\n   ```\n\n", impl.Name)
	b.WriteString("2. Wait for it. Run `harness-bridge wait <job> --max 9m` with the Bash timeout\n   set to 600000 ms. While the status is `running`, run the same command again.\n   Don't stop while the job is running.\n\n")
	fmt.Fprintf(&b, "3. When the status is anything else, reply with these fields from the final\n   JSON: `status`, `session_id`, `message`, `files_changed`, `commands`, `git`,\n   `permission_denied` and `error`. Quote %s's message as it is. Don't add\n   your own assessment.\n", impl.Title)
	return b.String()
}

func plannerInstructions(p, impl *agent.Adapter, router string) string {
	var b strings.Builder
	via := "through `harness-bridge`"
	if p == agent.Claude {
		via = "through the `" + subagentName(impl) + "` subagent"
	}
	b.WriteString("## Plan, delegate, review\n\n")
	fmt.Fprintf(&b, "For coding tasks in this project, you plan and review, and %s implements %s. Don't edit code yourself.\n\n", impl.Title, via)
	step := 1
	if router != "" {
		fmt.Fprintf(&b, "%d. **Route.** Pipe the user's request into `harness-bridge route --provider %s`. If `route` is `answer`, answer read-only and stop here. If it is `direct_implement`, a short plan is enough.\n", step, router)
		step++
	}
	fmt.Fprintf(&b, "%d. **Plan.** Read what you need, then write a plan %s can follow without asking questions: the goal, the files to change, the steps, the checks to run (for example `go test ./...`) and what done looks like. End the plan with: \"When you finish, reply with what you changed, the checks you ran and their results, and anything you could not do.\"\n", step, impl.Title)
	step++
	if p == agent.Claude {
		fmt.Fprintf(&b, "%d. **Delegate.** Give the whole plan to `%s`.\n", step, subagentName(impl))
	} else {
		flags := "--to " + impl.Name + " --dir ."
		if impl == agent.Claude {
			flags += " --allow 'Bash(go test:*)'"
		}
		fmt.Fprintf(&b, "%d. **Delegate.** Start a job with the whole plan on stdin:\n\n   ```sh\n   harness-bridge start %s <<'HARNESS_BRIDGE_PROMPT'\n   <the plan>\n   HARNESS_BRIDGE_PROMPT\n   ```\n\n", step, flags)
		b.WriteString("   Then run `harness-bridge wait <job> --max 2m`, and again while the status is `running`. If your command times out first, run `wait` again: the job keeps going.")
		if p == agent.Codex {
			b.WriteString(" harness-bridge needs network access and writes job files outside the workspace, so if the sandbox blocks it, ask to run it outside the sandbox.")
		}
		b.WriteString("\n")
	}
	step++
	fmt.Fprintf(&b, "%d. **Review the repository, not the report.** Run `git diff` and the project's checks yourself, and compare the result with the plan. If `git.dirty_before` is above 0, the workspace already had uncommitted changes before %s started.\n", step, impl.Title)
	step++
	repair := "send the specific fixes as a new job that repeats the context it needs, because " + impl.Title + " cannot continue a session"
	switch {
	case impl.Resume && p == agent.Claude:
		repair = fmt.Sprintf("give `%s` the specific fixes together with the `session_id` from its last report, so %s continues the same session", subagentName(impl), impl.Title)
	case impl.Resume:
		repair = fmt.Sprintf("start a new job with the specific fixes and `--session <session_id>` from the last report, so %s continues the same session", impl.Title)
	}
	fmt.Fprintf(&b, "%d. **Repair.** If something is wrong, %s. Stop after 3 repair rounds and report what is still open.\n", step, repair)
	step++
	fmt.Fprintf(&b, "%d. **Report** to the user what changed, what you checked and how, and what is still open.\n\n", step)
	for _, note := range impl.Notes {
		fmt.Fprintf(&b, "- %s\n", note)
	}
	if len(impl.Notes) > 0 {
		b.WriteString("\n")
	}
	b.WriteString("Answer questions, explain code and review existing changes yourself. Delegate only work that changes code.\n")
	return b.String()
}

// upsertBlock replaces the managed block in path, or appends it, leaving the
// rest of the file as it was.
func upsertBlock(path, body string) error {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	text := string(old)
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	block := strings.ReplaceAll(blockBegin+"\n"+body+blockEnd, "\n", newline)
	// The end marker is looked for after the begin marker, so a stray end
	// marker from damaged text doesn't cut the file short.
	begin := strings.Index(text, blockBegin)
	var end int
	if begin < 0 {
		end = -1
	} else if rel := strings.Index(text[begin:], blockEnd); rel >= 0 {
		end = begin + rel
	} else {
		return fmt.Errorf("%s: the harness-bridge block's end marker is missing, so the file was left unchanged", path)
	}
	switch {
	case end >= 0:
		text = text[:begin] + block + text[end+len(blockEnd):]
	case strings.TrimSpace(text) == "":
		text = block + newline
	default:
		text = strings.TrimRight(text, "\r\n") + newline + newline + block + newline
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// lockClaudePlanner denies Claude's own edit tools and allows harness-bridge,
// keeping every other setting in the file.
func lockClaudePlanner(path string) error {
	return addClaudePermissions(path,
		[]string{"Bash(harness-bridge:*)", "Bash(git diff:*)", "Bash(git status:*)"},
		[]string{"Edit", "Write", "NotebookEdit"})
}

// addClaudePermissions adds allow and deny rules to a Claude Code settings
// file, keeping every other setting and rule in it.
func addClaudePermissions(path string, allow, deny []string) error {
	settings := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("%s is not valid JSON, so it was left unchanged: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	perms, _ := settings["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	add := func(key string, rules []string) {
		if len(rules) == 0 {
			return
		}
		var list []any
		if existing, ok := perms[key].([]any); ok {
			list = existing
		}
		for _, rule := range rules {
			if !slices.Contains(list, any(rule)) {
				list = append(list, rule)
			}
		}
		perms[key] = list
	}
	add("allow", allow)
	add("deny", deny)
	settings["permissions"] = perms
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
