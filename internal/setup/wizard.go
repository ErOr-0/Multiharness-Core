package setup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/config"
)

// runQuiet runs a CLI's own configuration command and returns its output on
// failure.
func runQuiet(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return fmt.Errorf("%s %s: %v: %s", filepath.Base(name), strings.Join(args, " "), err, strings.Join(lines[max(0, len(lines)-3):], " | "))
	}
	return nil
}

type wizard struct {
	in       *bufio.Reader
	stdin    *os.File // given to sign-in commands, which talk to the user
	out      io.Writer
	terminal bool   // out is a terminal, so progress can redraw one line
	exe      string // the harness-bridge the planner will start
}

func CmdWizard(stdin io.Reader, stdout, stderr io.Writer) int {
	w := &wizard{in: bufio.NewReader(stdin), out: stdout}
	if f, ok := stdin.(*os.File); ok {
		w.stdin = f
	}
	if f, ok := stdout.(*os.File); ok {
		w.terminal = cli.IsTerminal(f)
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stdout, "Can't find harness-bridge itself: %v\n", err)
		return cli.ExitFailed
	}
	// The planner will start this file later, so it must not be a build or
	// download that is about to disappear.
	if rel, err := filepath.Rel(os.TempDir(), exe); err == nil && !strings.HasPrefix(rel, "..") {
		fmt.Fprintf(stdout, "harness-bridge is running from a temporary folder (%s). Install it with the install script first, then run it again.\n", exe)
		return cli.ExitFailed
	}
	w.exe = exe
	return w.run()
}

func (w *wizard) run() int {
	saved, _ := config.Load()
	exe := w.exe
	fmt.Fprintln(w.out, "harness-bridge setup: one agent plans and reviews your work, another writes the code.")

	found := map[*agent.Adapter]string{}
	for _, a := range agent.All {
		if path, ok := agent.FindBin(a.Bin); ok {
			found[a] = path
		}
	}
	var planners []*agent.Adapter
	for _, a := range agent.All {
		if a.Planner != nil {
			planners = append(planners, a)
		}
	}
	planner := w.choose("Which agent should plan and review your work?", planners, found, agent.Named(saved.Planner))
	impl := w.choose("Which agent should write the code?", agent.All, found, defaultImplementer(saved.Implementer, planner, found))
	if impl == planner {
		fmt.Fprintf(w.out, "  %s will plan and write the code. Pick a different model for the writing below if you want the two to differ.\n", impl.Title)
	}
	savedModel, savedReasoning := "", ""
	if saved.Implementer == impl.Name {
		savedModel, savedReasoning = saved.Model, saved.Reasoning
	}
	model := w.chooseModel(impl, found[impl], savedModel)
	reasoning := w.chooseReasoning(impl, savedReasoning)
	router := w.chooseRouter(saved.Router)

	roles := []*agent.Adapter{planner}
	if impl != planner {
		roles = append(roles, impl)
	}
	var installs []*agent.Adapter
	var steps []string
	for _, a := range roles {
		if _, ok := found[a]; ok {
			continue
		}
		in, ok := a.Installer()
		if !ok {
			fmt.Fprintf(w.out, "\n%s isn't installed and can't be installed automatically on this computer. Install it from %s, then run harness-bridge again.\n", a.Title, a.Docs)
			return cli.ExitFailed
		}
		installs = append(installs, a)
		steps = append(steps, fmt.Sprintf("Install %s:  %s", a.Title, in.Display()))
	}
	steps = append(steps, planner.Planner.ConnectLabel(planner))
	if planner.Planner.AllowNote != "" {
		steps = append(steps, planner.Planner.AllowNote)
	}
	steps = append(steps, "Sign in to any agent that isn't signed in yet (your browser opens)", "Save these choices (run harness-bridge again to change them)")
	fmt.Fprintln(w.out, "\nHere's what happens next:")
	for _, s := range steps {
		fmt.Fprintln(w.out, "  • "+s)
	}
	if !w.yesNo("\nContinue?", true) {
		fmt.Fprintln(w.out, "Nothing was changed.")
		return cli.ExitFailed
	}
	fmt.Fprintln(w.out)

	for _, a := range installs {
		path, ok := w.install(a)
		if !ok {
			return cli.ExitFailed
		}
		found[a] = path
	}
	cfg := config.Config{Planner: planner.Name, Implementer: impl.Name, Router: router, Model: model, Reasoning: reasoning}
	if saved.Implementer == impl.Name {
		cfg.Allow = saved.Allow
	}
	if !w.step(planner.Planner.ConnectLabel(planner), func() error { return planner.Planner.Register(found[planner], exe, cfg) }) {
		return cli.ExitFailed
	}
	if planner.Planner.Allow != nil && !w.step(planner.Planner.AllowNote, planner.Planner.Allow) {
		return cli.ExitFailed
	}
	for _, a := range roles {
		w.signIn(a, found[a])
	}
	path, err := config.Save(cfg)
	if !w.step("Saving your choices", func() error { return err }) {
		return cli.ExitFailed
	}
	note := ""
	switch {
	case model != "" && reasoning != "":
		note = " (model " + model + ", reasoning " + reasoning + ")"
	case model != "":
		note = " (model " + model + ")"
	case reasoning != "":
		note = " (reasoning " + reasoning + ")"
	}
	fmt.Fprintf(w.out, "\nAll set. Open %s in any project and ask for a change: it plans, hands the coding to %s%s and reviews the result.\nYour choices are in %s.\n", planner.Title, impl.Title, note, path)
	if old := agent.Named(saved.Planner); old != nil && old != planner && old.Planner != nil {
		fmt.Fprintf(w.out, "%s is still connected too; `harness-bridge disconnect --planner %s` removes it.\n", old.Title, old.Name)
	}
	return cli.ExitOK
}

func (w *wizard) readLine() string {
	line, _ := w.in.ReadString('\n')
	return strings.TrimSpace(line)
}

// choose asks for one of options. The default is preferred when it is an
// option, else the first installed option, else the first option.
func (w *wizard) choose(question string, options []*agent.Adapter, found map[*agent.Adapter]string, preferred *agent.Adapter) *agent.Adapter {
	def := -1
	for i, a := range options {
		if a == preferred {
			def = i
		}
	}
	for i, a := range options {
		if _, ok := found[a]; ok && def < 0 {
			def = i
		}
	}
	def = max(def, 0)
	fmt.Fprintf(w.out, "\n%s\n", question)
	for i, a := range options {
		state := "will be installed"
		if _, ok := found[a]; ok {
			state = "installed"
		}
		fmt.Fprintf(w.out, "  %d) %-13s %s\n", i+1, a.Title, state)
	}
	for {
		fmt.Fprintf(w.out, "Choose 1-%d [%d]: ", len(options), def+1)
		answer := w.readLine()
		if answer == "" {
			return options[def]
		}
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(options) {
			return options[n-1]
		}
		for _, a := range options {
			if strings.EqualFold(answer, a.Name) {
				return a
			}
		}
		fmt.Fprintln(w.out, "Please type one of the numbers.")
	}
}

// defaultImplementer is the saved choice, else the first installed agent in
// order of how well it works headless, else Codex.
func defaultImplementer(saved string, planner *agent.Adapter, found map[*agent.Adapter]string) *agent.Adapter {
	if a := agent.Named(saved); a != nil && a != planner {
		return a
	}
	for _, a := range []*agent.Adapter{agent.Codex, agent.Claude, agent.OpenCode, agent.CommandCode, agent.Muse} {
		if _, ok := found[a]; ok && a != planner {
			return a
		}
	}
	return agent.Codex
}

func (w *wizard) chooseRouter(saved string) string {
	options := []string{"", "laya", "jev"}
	def := 0
	for i, o := range options {
		if o == saved {
			def = i
		}
	}
	fmt.Fprintln(w.out, "\nBefore planning, ask a decision model whether a request needs a plan at all?")
	fmt.Fprintln(w.out, "  1) No")
	fmt.Fprintln(w.out, "  2) Laya, on your own server (LAYA_ENDPOINT, default http://127.0.0.1:8765)")
	fmt.Fprintln(w.out, "  3) Jev, through OpenRouter (needs OPENROUTER_API_KEY)")
	for {
		fmt.Fprintf(w.out, "Choose 1-3 [%d]: ", def+1)
		answer := w.readLine()
		if answer == "" {
			return options[def]
		}
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= 3 {
			return options[n-1]
		}
		fmt.Fprintln(w.out, "Please type 1, 2 or 3.")
	}
}

// pick asks for one of options by number or name. Enter keeps current ("" is
// the CLI's own default) and "default" clears it. Text that is not an option
// narrows a long list; with no match it is taken as typed.
func (w *wizard) pick(prompt string, options []string, current string) string {
	shown := current
	if shown == "" {
		shown = "default"
	}
	// visible is the list a number refers to: the full list while it is on
	// screen, or the last printed matches while narrowing a long one.
	var visible []string
	if len(options) <= 30 {
		for i, o := range options {
			fmt.Fprintf(w.out, "  %d) %s\n", i+1, o)
		}
		visible = options
	} else {
		fmt.Fprintf(w.out, "  %d choices. Type part of a name to see the matches, or a full name.\n", len(options))
	}
	for {
		fmt.Fprintf(w.out, "%s [%s]: ", prompt, shown)
		answer := w.readLine()
		switch {
		case answer == "":
			return current
		case strings.EqualFold(answer, "default"):
			return ""
		}
		if n, err := strconv.Atoi(answer); err == nil && len(visible) > 0 && n >= 1 && n <= len(visible) {
			return visible[n-1]
		}
		for _, o := range options {
			if strings.EqualFold(answer, o) {
				return o
			}
		}
		var matches []string
		for _, o := range options {
			if strings.Contains(strings.ToLower(o), strings.ToLower(answer)) {
				matches = append(matches, o)
			}
		}
		switch {
		case len(matches) >= 1 && len(matches) <= 30:
			for i, m := range matches {
				fmt.Fprintf(w.out, "  %d) %s\n", i+1, m)
			}
			visible = matches
		case len(matches) > 30:
			fmt.Fprintf(w.out, "  %d matches. Type more of the name.\n", len(matches))
		case agent.SafeValue(answer):
			// A name the list does not have: a model newer than the list,
			// or a CLI without one.
			return answer
		default:
			fmt.Fprintln(w.out, "That name has characters it can't contain. Try again, or press Enter.")
		}
	}
}

// chooseModel asks which model the implementer should use for every job. The
// list comes from the CLI itself when it has one, so it is always current.
// An empty answer keeps current (the saved model, or "" for the CLI's own
// default); "default" clears a saved one.
func (w *wizard) chooseModel(impl *agent.Adapter, path string, current string) string {
	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "Which model should %s use?\n", impl.Title)
	var options []string
	live := false
	if path != "" {
		if options = impl.Models(path); len(options) > 0 {
			live = true
		}
	}
	if len(options) == 0 {
		options = impl.ModelAliases
	}
	switch {
	case live:
		fmt.Fprintf(w.out, "  From %s's own list, so it is current.\n", impl.Title)
	case len(options) > 0:
		fmt.Fprintln(w.out, "  These names always mean the newest model of each family; you can also type a full model name.")
	case len(impl.ModelList) > 0 && path == "":
		fmt.Fprintf(w.out, "  %s isn't installed yet, so its models can't be listed. Type a name, or press Enter and run harness-bridge again later to pick one.\n", impl.Title)
	case len(impl.ModelList) > 0:
		fmt.Fprintf(w.out, "  %s didn't list its models. Type a name, or press Enter for its default.\n", impl.Title)
	default:
		fmt.Fprintf(w.out, "  %s has no command that lists its models. Type a name, or press Enter for its default.\n", impl.Title)
	}
	if impl == agent.OpenCode {
		fmt.Fprintln(w.out, "  OpenCode names a model as provider/model, with #variant for the reasoning level.")
	}
	if current != "" {
		fmt.Fprintf(w.out, "  Press Enter to keep %s, or type default for %s's own default.\n", current, impl.Title)
	} else {
		fmt.Fprintf(w.out, "  Press Enter for %s's own default.\n", impl.Title)
	}
	return w.pick("Model", options, current)
}

// chooseReasoning asks how hard the implementer should think on each job.
// CLIs without a separate reasoning setting keep none.
func (w *wizard) chooseReasoning(impl *agent.Adapter, current string) string {
	if len(impl.Efforts) == 0 {
		return ""
	}
	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "How hard should %s think on each job?\n", impl.Title)
	if current != "" {
		fmt.Fprintf(w.out, "  Press Enter to keep %s, or type default for %s's own default.\n", current, impl.Title)
	} else {
		fmt.Fprintf(w.out, "  Press Enter for %s's own default.\n", impl.Title)
	}
	return w.pick("Reasoning", impl.Efforts, current)
}

func (w *wizard) yesNo(question string, def bool) bool {
	hint := "[Y/n]"
	if !def {
		hint = "[y/N]"
	}
	for {
		fmt.Fprintf(w.out, "%s %s ", question, hint)
		switch strings.ToLower(w.readLine()) {
		case "":
			return def
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
	}
}

// step runs fn and prints one line saying whether it worked.
func (w *wizard) step(label string, fn func() error) bool {
	if err := fn(); err != nil {
		fmt.Fprintf(w.out, "  ✗ %s: %v\n", label, err)
		return false
	}
	fmt.Fprintf(w.out, "  ✓ %s\n", label)
	return true
}

// install runs the official installer in the background, showing progress,
// with its output in a log file.
func (w *wizard) install(a *agent.Adapter) (string, bool) {
	in, _ := a.Installer()
	logPath := filepath.Join(os.TempDir(), "harness-bridge-install-"+a.Name+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		fmt.Fprintf(w.out, "  ✗ Installing %s: %v\n", a.Title, err)
		return "", false
	}
	defer logFile.Close()
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, in.Command[0], in.Command[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = os.TempDir(), logFile, logFile
	started := time.Now()
	label := "Installing " + a.Title
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(w.out, "  ✗ %s: %v\n", label, err)
		return "", false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if !w.terminal {
		fmt.Fprintf(w.out, "  … %s\n", label)
	}
	spin := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	tick := time.NewTicker(120 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		select {
		case err := <-done:
			elapsed := time.Since(started).Round(time.Second)
			if w.terminal {
				fmt.Fprint(w.out, "\r\033[K")
			}
			if err != nil {
				fmt.Fprintf(w.out, "  ✗ %s failed after %s (%v). Details: %s\n", label, elapsed, err, logPath)
				return "", false
			}
			path, ok := agent.FindBin(a.Bin)
			if !ok {
				fmt.Fprintf(w.out, "  ✗ %s finished, but `%s` can't be found. Open a new terminal and run harness-bridge again. Details: %s\n", label, a.Bin, logPath)
				return "", false
			}
			fmt.Fprintf(w.out, "  ✓ Installed %s (%s)\n", a.Title, elapsed)
			return path, true
		case <-tick.C:
			if w.terminal {
				fmt.Fprintf(w.out, "\r  %c %s… %s", spin[i%len(spin)], label, time.Since(started).Round(time.Second))
			}
		}
	}
}

// signIn checks whether the agent is signed in and, if not, runs its sign-in
// with the terminal attached so it can open the browser and ask questions.
func (w *wizard) signIn(a *agent.Adapter, path string) {
	if len(a.LoginCheck) > 0 {
		if runQuiet(30*time.Second, path, a.LoginCheck...) == nil {
			fmt.Fprintf(w.out, "  ✓ Signed in to %s\n", a.Title)
			return
		}
	} else if !w.yesNo(fmt.Sprintf("  Sign in to %s now? Skip this if you already are.", a.Title), false) {
		fmt.Fprintf(w.out, "  • Skipped. If %s asks you to sign in later, run `%s`.\n", a.Title, a.Login)
		return
	}
	fmt.Fprintf(w.out, "  Signing in to %s. Follow the steps it shows; a browser window may open.\n", a.Title)
	args := strings.Fields(a.Login)[1:]
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = w.stdin, w.out, w.out
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(w.out, "  ✗ Sign-in to %s didn't finish (%v). Run `%s` later.\n", a.Title, err, a.Login)
		return
	}
	fmt.Fprintf(w.out, "  ✓ Signed in to %s\n", a.Title)
}
