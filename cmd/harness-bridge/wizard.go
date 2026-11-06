package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// plannerHooks connect harness-bridge to an agent that plans, for all
// projects at once, by registering `harness-bridge mcp` as an MCP server.
type plannerHooks struct {
	// register connects exe (run as `exe mcp`) to the planner whose CLI is at
	// cli. cfg is what the user chose, for planners that need instructions too.
	register func(cli, exe string, cfg Config) error
	// connectNote, if set, replaces the plan line that says what register does.
	connectNote string
	// allow, if set, lets the planner call harness-bridge without asking each
	// time; allowNote describes it in the wizard's plan.
	allow     func() error
	allowNote string
}

func (h *plannerHooks) connectLabel(a *adapter) string {
	return firstNonEmpty(h.connectNote, "Connect harness-bridge to "+a.Title+" for all your projects")
}

// findBin finds an installed CLI on PATH, or in the folders official
// installers use, which a terminal opened before the install may not have on
// its PATH yet.
func findBin(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, ".local", "bin")}
	exts := []string{""}
	if runtime.GOOS == "windows" {
		dirs = append(dirs, filepath.Join(os.Getenv("APPDATA"), "npm"))
		exts = []string{".exe", ".cmd", ".bat"}
	} else {
		dirs = append(dirs, filepath.Join(home, ".npm-global", "bin"), filepath.Join(home, ".opencode", "bin"), "/usr/local/bin", "/opt/homebrew/bin")
	}
	for _, dir := range dirs {
		for _, ext := range exts {
			path := filepath.Join(dir, name+ext)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return path, true
			}
		}
	}
	return "", false
}

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

func cmdWizard(stdin io.Reader, stdout, stderr io.Writer) int {
	w := &wizard{in: bufio.NewReader(stdin), out: stdout}
	if f, ok := stdin.(*os.File); ok {
		w.stdin = f
	}
	if f, ok := stdout.(*os.File); ok {
		w.terminal = isTerminal(f)
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stdout, "Can't find harness-bridge itself: %v\n", err)
		return exitFailed
	}
	// The planner will start this file later, so it must not be a build or
	// download that is about to disappear.
	if rel, err := filepath.Rel(os.TempDir(), exe); err == nil && !strings.HasPrefix(rel, "..") {
		fmt.Fprintf(stdout, "harness-bridge is running from a temporary folder (%s). Install it with the install script first, then run it again.\n", exe)
		return exitFailed
	}
	w.exe = exe
	return w.run()
}

func (w *wizard) run() int {
	saved, _ := loadConfig()
	exe := w.exe
	fmt.Fprintln(w.out, "harness-bridge setup: one agent plans and reviews your work, another writes the code.")

	found := map[*adapter]string{}
	for _, a := range adapters {
		if path, ok := findBin(a.Bin); ok {
			found[a] = path
		}
	}
	var planners []*adapter
	for _, a := range adapters {
		if a.Planner != nil {
			planners = append(planners, a)
		}
	}
	planner := w.choose("Which agent should plan and review your work?", planners, found, adapterNamed(saved.Planner))
	var implementers []*adapter
	for _, a := range adapters {
		if !(planner == claudeAdapter && a == claudeAdapter) {
			implementers = append(implementers, a)
		}
	}
	impl := w.choose("Which agent should write the code?", implementers, found, defaultImplementer(saved.Implementer, planner, found))
	router := w.chooseRouter(saved.Router)

	roles := []*adapter{planner}
	if impl != planner {
		roles = append(roles, impl)
	}
	var installs []*adapter
	var steps []string
	for _, a := range roles {
		if _, ok := found[a]; ok {
			continue
		}
		in, ok := a.installer()
		if !ok {
			fmt.Fprintf(w.out, "\n%s isn't installed and can't be installed automatically on this computer. Install it from %s, then run harness-bridge again.\n", a.Title, a.Docs)
			return exitFailed
		}
		installs = append(installs, a)
		steps = append(steps, fmt.Sprintf("Install %s:  %s", a.Title, in.display()))
	}
	steps = append(steps, planner.Planner.connectLabel(planner))
	if planner.Planner.allowNote != "" {
		steps = append(steps, planner.Planner.allowNote)
	}
	steps = append(steps, "Sign in to any agent that isn't signed in yet (your browser opens)", "Save these choices (run harness-bridge again to change them)")
	fmt.Fprintln(w.out, "\nHere's what happens next:")
	for _, s := range steps {
		fmt.Fprintln(w.out, "  • "+s)
	}
	if !w.yesNo("\nContinue?", true) {
		fmt.Fprintln(w.out, "Nothing was changed.")
		return exitFailed
	}
	fmt.Fprintln(w.out)

	for _, a := range installs {
		path, ok := w.install(a)
		if !ok {
			return exitFailed
		}
		found[a] = path
	}
	cfg := Config{Planner: planner.Name, Implementer: impl.Name, Router: router}
	if saved.Implementer == impl.Name {
		cfg.Model, cfg.Allow = saved.Model, saved.Allow
	}
	if !w.step(planner.Planner.connectLabel(planner), func() error { return planner.Planner.register(found[planner], exe, cfg) }) {
		return exitFailed
	}
	if planner.Planner.allow != nil && !w.step(planner.Planner.allowNote, planner.Planner.allow) {
		return exitFailed
	}
	for _, a := range roles {
		w.signIn(a, found[a])
	}
	path, err := saveConfig(cfg)
	if !w.step("Saving your choices", func() error { return err }) {
		return exitFailed
	}
	fmt.Fprintf(w.out, "\nAll set. Open %s in any project and ask for a change: it plans, hands the coding to %s and reviews the result.\nYour choices are in %s.\n", planner.Title, impl.Title, path)
	return exitOK
}

func (w *wizard) readLine() string {
	line, _ := w.in.ReadString('\n')
	return strings.TrimSpace(line)
}

// choose asks for one of options. The default is preferred when it is an
// option, else the first installed option, else the first option.
func (w *wizard) choose(question string, options []*adapter, found map[*adapter]string, preferred *adapter) *adapter {
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
func defaultImplementer(saved string, planner *adapter, found map[*adapter]string) *adapter {
	if a := adapterNamed(saved); a != nil && a != planner {
		return a
	}
	for _, a := range []*adapter{codexAdapter, claudeAdapter, opencodeAdapter, commandCodeAdapter, museAdapter} {
		if _, ok := found[a]; ok && a != planner {
			return a
		}
	}
	return codexAdapter
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
func (w *wizard) install(a *adapter) (string, bool) {
	in, _ := a.installer()
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
			path, ok := findBin(a.Bin)
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
func (w *wizard) signIn(a *adapter, path string) {
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
