package setup

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

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/config"
	"multiharness-core/internal/job"
)

// CmdDisconnect undoes what setup did for one planner, or all of them, so the
// planner works as it did before. The program and the saved implementer stay.
func CmdDisconnect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("disconnect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("planner", "", "")
	all := fs.Bool("all", false, "")
	if err := fs.Parse(args); err != nil {
		return cli.UsageError(stdout, stderr, err)
	}
	if fs.NArg() > 0 {
		return cli.UsageError(stdout, stderr, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}
	cfg, _ := config.Load()
	var targets []*agent.Adapter
	switch {
	case *all:
		targets = connectablePlanners()
	case *name != "" || cfg.Planner != "":
		a := agent.Named(agent.FirstNonEmpty(*name, cfg.Planner))
		if a == nil || a.Planner == nil {
			return cli.UsageError(stdout, stderr, fmt.Errorf("--planner %q: use one of %s", agent.FirstNonEmpty(*name, cfg.Planner), plannerNames()))
		}
		targets = []*agent.Adapter{a}
	default:
		return cli.UsageError(stdout, stderr, errors.New("setup hasn't connected a planner; pass --planner NAME or --all"))
	}
	if !disconnectPlanners(targets, stdout) {
		return cli.ExitFailed
	}
	fmt.Fprintln(stdout, "\nDone. Run harness-bridge to connect again.")
	return cli.ExitOK
}

// disconnectPlanners undoes each planner's registration and forgets it as the
// saved planner, but only when its unregister worked — a planner still
// connected must stay the saved one so a rerun can find it.
func disconnectPlanners(targets []*agent.Adapter, stdout io.Writer) bool {
	w := &wizard{out: stdout}
	ok := true
	var unregistered []string
	for _, a := range targets {
		cli, _ := agent.FindBin(a.Bin)
		if w.step("Disconnected harness-bridge from "+a.Title, func() error { return a.Planner.Unregister(cli) }) {
			unregistered = append(unregistered, a.Name)
		} else {
			ok = false
		}
	}
	cfg, err := config.Load()
	if err == nil && cfg.Planner != "" && slices.Contains(unregistered, cfg.Planner) {
		cfg.Planner = ""
		if _, err := config.Save(cfg); err != nil {
			fmt.Fprintf(stdout, "  ✗ Updating your saved choices: %v\n", err)
			ok = false
		}
	}
	return ok
}

func connectablePlanners() []*agent.Adapter {
	var list []*agent.Adapter
	for _, a := range agent.All {
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
	if path, err := config.Path(); err == nil {
		p.config = path
		if os.Getenv("HARNESS_BRIDGE_CONFIG") == "" {
			p.config = filepath.Dir(path)
		}
	}
	if root, err := job.Root(); err == nil {
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

func CmdUninstall(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "")
	if err := fs.Parse(args); err != nil {
		return cli.UsageError(stdout, stderr, err)
	}
	if fs.NArg() > 0 {
		return cli.UsageError(stdout, stderr, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}
	return uninstall(planUninstall(), confirmer(stdin, stdout, *yes), stdout)
}

func uninstall(p uninstallPlan, confirm func(string) bool, stdout io.Writer) int {
	fmt.Fprintln(stdout, "harness-bridge uninstall will:")
	fmt.Fprintf(stdout, "  • Disconnect harness-bridge from every planner (%s)\n", plannerNames())
	for _, item := range []struct{ label, path string }{{"Delete your saved choices", p.config}, {"Delete the job logs", p.jobs}, {"Delete the program", agent.FirstNonEmpty(p.programDir, p.program)}, {"Remove it from your PATH", p.pathEntry}} {
		if item.path != "" {
			fmt.Fprintf(stdout, "  • %s: %s\n", item.label, item.path)
		}
	}
	if p.leftAlone != "" {
		fmt.Fprintf(stdout, "  • Leave %s, which wasn't installed by the install script; delete it yourself\n", p.leftAlone)
	}
	if !confirm("\nRemove harness-bridge?") {
		fmt.Fprintln(stdout, "Nothing was changed.")
		return cli.ExitFailed
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
		return cli.ExitFailed
	}
	fmt.Fprintln(stdout, "\nharness-bridge is removed. Your agent CLIs are still installed.")
	return cli.ExitOK
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
	switch base := filepath.Base(path); base {
	case "jobs":
		// HARNESS_BRIDGE_HOME may hold the user's own files next to the job
		// folders, so delete only the job folders themselves.
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() && job.IDPattern.MatchString(e.Name()) {
				if err := os.RemoveAll(filepath.Join(path, e.Name())); err != nil {
					return err
				}
			}
		}
		if left, err := os.ReadDir(path); err != nil {
			return err
		} else if len(left) == 0 {
			return os.Remove(path)
		}
		return nil
	case "harness-bridge":
		return os.RemoveAll(path)
	default:
		return fmt.Errorf("refusing to delete %s: it isn't a harness-bridge folder", path)
	}
}

// removeFromUserPath drops dir from the user PATH the Windows installer added
// it to. Tests replace it.
var removeFromUserPath = func(dir string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	return runQuiet(time.Minute, "powershell", "-NoProfile", "-NonInteractive", "-Command", removeFromUserPathScript(dir))
}

// removeFromUserPathScript builds the PowerShell that edits the user PATH. It
// reads and writes the raw registry value so entries like %USERPROFILE%\...
// stay unexpanded, compares entries case-insensitively ignoring a trailing
// backslash, and broadcasts the change so new processes see it.
func removeFromUserPathScript(dir string) string {
	quoted := "'" + strings.ReplaceAll(dir, "'", "''") + "'"
	return "$d = " + quoted + "; $p = (Get-Item HKCU:\\Environment).GetValue('Path', '', 'DoNotExpandEnvironmentNames'); " +
		"if ($p) { $new = (($p -split ';') | Where-Object { $_ -and $_.TrimEnd('\\') -ine $d.TrimEnd('\\') }) -join ';'; " +
		"Set-ItemProperty -Path HKCU:\\Environment -Name Path -Value $new -Type ExpandString; " +
		"try { if (-not ('Win32.Settings' -as [type])) { Add-Type -Namespace Win32 -Name Settings -MemberDefinition '[DllImport(\"user32.dll\", SetLastError = true, CharSet = CharSet.Auto)] public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);' }; " +
		"$r = [UIntPtr]::Zero; [void][Win32.Settings]::SendMessageTimeout([IntPtr]0xffff, 0x1A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$r) } catch { } }"
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
	// The end marker is looked for after the begin marker, so a stray end
	// marker from damaged text doesn't cut the file short.
	begin := strings.Index(text, blockBegin)
	if begin < 0 {
		return false, nil
	}
	rel := strings.Index(text[begin:], blockEnd)
	if rel < 0 {
		return false, fmt.Errorf("%s: the harness-bridge block's end marker is missing, so the file was left unchanged", path)
	}
	end := begin + rel
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
