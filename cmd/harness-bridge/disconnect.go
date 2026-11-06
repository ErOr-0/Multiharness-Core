package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// cmdDisconnect undoes what setup did for one planner, or all of them, so the
// planner works as it did before. The program and the saved implementer stay.
func cmdDisconnect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("disconnect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("planner", "", "")
	all := fs.Bool("all", false, "")
	if err := fs.Parse(args); err != nil {
		return usageError(stdout, stderr, err)
	}
	if fs.NArg() > 0 {
		return usageError(stdout, stderr, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}
	cfg, _ := loadConfig()
	var targets []*adapter
	switch {
	case *all:
		targets = connectablePlanners()
	case *name != "" || cfg.Planner != "":
		a := adapterNamed(firstNonEmpty(*name, cfg.Planner))
		if a == nil || a.Planner == nil {
			return usageError(stdout, stderr, fmt.Errorf("--planner %q: use one of %s", firstNonEmpty(*name, cfg.Planner), plannerNames()))
		}
		targets = []*adapter{a}
	default:
		return usageError(stdout, stderr, errors.New("setup hasn't connected a planner; pass --planner NAME or --all"))
	}
	if !disconnectPlanners(targets, stdout) {
		return exitFailed
	}
	fmt.Fprintln(stdout, "\nDone. Run harness-bridge to connect again.")
	return exitOK
}

// disconnectPlanners undoes each planner's registration and forgets it as the
// saved planner. It reports whether every step worked.
func disconnectPlanners(targets []*adapter, stdout io.Writer) bool {
	w := &wizard{out: stdout}
	ok := true
	for _, a := range targets {
		cli, _ := findBin(a.Bin)
		if !w.step("Disconnected harness-bridge from "+a.Title, func() error { return a.Planner.unregister(cli) }) {
			ok = false
		}
	}
	cfg, err := loadConfig()
	if err == nil && cfg.Planner != "" && slices.ContainsFunc(targets, func(a *adapter) bool { return a.Name == cfg.Planner }) {
		cfg.Planner = ""
		if _, err := saveConfig(cfg); err != nil {
			fmt.Fprintf(stdout, "  ✗ Updating your saved choices: %v\n", err)
			ok = false
		}
	}
	return ok
}

func connectablePlanners() []*adapter {
	var list []*adapter
	for _, a := range adapters {
		if a.Planner != nil {
			list = append(list, a)
		}
	}
	return list
}

func plannerNames() string {
	var names []string
	for _, a := range connectablePlanners() {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

// uninstallPlan is what uninstall removes. Empty fields are skipped.
type uninstallPlan struct {
	config     string // the saved choices: a file, or the folder holding them
	jobs       string // job logs: the jobs folder, or the folder holding it
	program    string // the running harness-bridge, when the installer put it there
	programDir string // on Windows, the installer's folder, removed with it
	pathEntry  string // on Windows, the installer's PATH entry
	leftAlone  string // a harness-bridge outside the installer's folder
}

func planUninstall() uninstallPlan {
	var p uninstallPlan
	if path, err := configPath(); err == nil {
		p.config = path
		if os.Getenv("HARNESS_BRIDGE_CONFIG") == "" {
			p.config = filepath.Dir(path)
		}
	}
	if root, err := jobsRoot(); err == nil {
		p.jobs = root
		if os.Getenv("HARNESS_BRIDGE_HOME") == "" {
			p.jobs = filepath.Dir(root)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return p
	}
	dir := installDir()
	if !sameDir(filepath.Dir(exe), dir) {
		p.leftAlone = exe
		return p
	}
	p.program = exe
	if runtime.GOOS == "windows" {
		p.programDir, p.pathEntry = dir, dir
	}
	return p
}

// installDir is where install.sh and install.ps1 put harness-bridge by default.
func installDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "harness-bridge")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func cmdUninstall(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "")
	if err := fs.Parse(args); err != nil {
		return usageError(stdout, stderr, err)
	}
	if fs.NArg() > 0 {
		return usageError(stdout, stderr, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}
	return uninstall(planUninstall(), confirmer(stdin, stdout, *yes), stdout)
}

func uninstall(p uninstallPlan, confirm func(string) bool, stdout io.Writer) int {
	fmt.Fprintln(stdout, "harness-bridge uninstall will:")
	fmt.Fprintf(stdout, "  • Disconnect harness-bridge from every planner (%s)\n", plannerNames())
	for _, item := range []struct{ label, path string }{{"Delete your saved choices", p.config}, {"Delete the job logs", p.jobs}, {"Delete the program", firstNonEmpty(p.programDir, p.program)}, {"Remove it from your PATH", p.pathEntry}} {
		if item.path != "" {
			fmt.Fprintf(stdout, "  • %s: %s\n", item.label, item.path)
		}
	}
	if p.leftAlone != "" {
		fmt.Fprintf(stdout, "  • Leave %s, which wasn't installed by the install script; delete it yourself\n", p.leftAlone)
	}
	if !confirm("\nRemove harness-bridge?") {
		fmt.Fprintln(stdout, "Nothing was changed.")
		return exitFailed
	}
	fmt.Fprintln(stdout)
	ok := disconnectPlanners(connectablePlanners(), stdout)
	w := &wizard{out: stdout}
	for _, item := range []struct{ label, path string }{{"Deleted your saved choices", p.config}, {"Deleted the job logs", p.jobs}} {
		if item.path != "" && !w.step(item.label, func() error { return removeOurs(item.path) }) {
			ok = false
		}
	}
	if p.pathEntry != "" && !w.step("Removed it from your PATH", func() error { return removeFromUserPath(p.pathEntry) }) {
		ok = false
	}
	switch {
	case p.programDir != "":
		// A running program can't delete itself on Windows, so its folder goes
		// a moment after this exits.
		if !w.step("Deleting the program as soon as this finishes", func() error { return removeAfterExit(p.programDir) }) {
			ok = false
		}
	case p.program != "":
		if !w.step("Deleted the program", func() error { return os.Remove(p.program) }) {
			ok = false
		}
	}
	if !ok {
		fmt.Fprintln(stdout, "\nSome steps failed; see the lines marked ✗.")
		return exitFailed
	}
	fmt.Fprintln(stdout, "\nharness-bridge is removed. Your agent CLIs are still installed.")
	return exitOK
}

// removeOurs deletes a harness-bridge file or folder. Folders are deleted only
// when they are named for harness-bridge or its jobs, so a wrong path can never
// take anything else with it.
func removeOurs(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.Remove(path)
	}
	if base := filepath.Base(path); base != "harness-bridge" && base != "jobs" {
		return fmt.Errorf("refusing to delete %s: it isn't a harness-bridge folder", path)
	}
	return os.RemoveAll(path)
}

// removeFromUserPath drops dir from the user PATH the Windows installer added
// it to. Tests replace it.
var removeFromUserPath = func(dir string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	script := "$d = '" + strings.ReplaceAll(dir, "'", "''") + "'; $p = [Environment]::GetEnvironmentVariable('Path', 'User'); " +
		"if ($p) { [Environment]::SetEnvironmentVariable('Path', ((($p -split ';') | Where-Object { $_ -and $_.TrimEnd('\\') -ne $d.TrimEnd('\\') }) -join ';'), 'User') }"
	return runQuiet(time.Minute, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
}

// removeBlock deletes the block setup wrote into an instructions file, keeping
// the rest. A file left empty is deleted. It reports whether there was a block.
func removeBlock(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	text := string(data)
	begin, end := strings.Index(text, blockBegin), strings.Index(text, blockEnd)
	if begin < 0 || end < begin {
		return false, nil
	}
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	before := strings.TrimRight(text[:begin], "\r\n")
	after := strings.TrimLeft(text[end+len(blockEnd):], "\r\n")
	var rest string
	switch {
	case before == "" && after == "":
		return true, os.Remove(path)
	case before == "":
		rest = after
	case after == "":
		rest = before + newline
	default:
		rest = before + newline + newline + after
	}
	return true, os.WriteFile(path, []byte(rest), 0o644)
}

// removeClaudeAllow takes one allow rule out of a Claude Code settings file,
// keeping everything else.
func removeClaudeAllow(path, rule string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	settings := map[string]any{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("%s is not valid JSON, so the %s rule was left in it", path, rule)
	}
	perms, _ := settings["permissions"].(map[string]any)
	allow, _ := perms["allow"].([]any)
	kept := slices.DeleteFunc(slices.Clone(allow), func(v any) bool { return v == rule })
	if len(kept) == len(allow) {
		return nil
	}
	perms["allow"] = kept
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}
