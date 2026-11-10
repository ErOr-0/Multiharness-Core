package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func disconnect(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := cli(append([]string{"disconnect"}, args...), strings.NewReader(""), &out, &out)
	return out.String(), code
}

func block(body string) string { return blockBegin + "\n" + body + blockEnd }

func TestRemoveBlock(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, text, want string
		gone             bool
	}{
		{"only the block", block("x\n") + "\n", "", true},
		{"before and after", "# Mine\n\n" + block("x\n") + "\n\nAlso mine.\n", "# Mine\n\nAlso mine.\n", false},
		{"before only", "# Mine\n\n" + block("x\n") + "\n", "# Mine\n", false},
		{"windows newlines", "# Mine\r\n\r\n" + strings.ReplaceAll(block("x\n"), "\n", "\r\n") + "\r\n", "# Mine\r\n", false},
		{"no block", "# Mine\n", "# Mine\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "-")+".md")
			write(t, path, tc.text)
			if _, err := removeBlock(path); err != nil {
				t.Fatal(err)
			}
			if tc.gone {
				if fileExists(path) {
					t.Fatalf("a file that held only the block is still there: %q", read(t, path))
				}
				return
			}
			if got := read(t, path); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDisconnectClaude(t *testing.T) {
	log := fakeCLIs(t, "claude")
	settings := claudeSettingsPath()
	write(t, settings, `{"model":"opus","permissions":{"allow":["Bash(ls)","mcp__harness-bridge"]}}`)
	useConfig(t, Config{Planner: "claude", Implementer: "codex"})
	out, code := disconnect(t)
	if code != exitOK || !strings.Contains(calls(t, log), "claude mcp remove harness-bridge --scope user") {
		t.Fatalf("exit %d:\n%s\n%s", code, out, calls(t, log))
	}
	var s struct {
		Model       string `json:"model"`
		Permissions struct{ Allow []string }
	}
	if err := json.Unmarshal([]byte(read(t, settings)), &s); err != nil {
		t.Fatal(err)
	}
	if s.Model != "opus" || !slices.Equal(s.Permissions.Allow, []string{"Bash(ls)"}) {
		t.Fatalf("settings %+v", s)
	}
	if cfg, _ := loadConfig(); cfg.Planner != "" || cfg.Implementer != "codex" {
		t.Fatalf("config %+v", cfg)
	}
}

func TestClaudePlannerGetsInstructions(t *testing.T) {
	log := fakeCLIs(t, "claude")
	cli, _ := findBin("claude")
	write(t, claudeInstructionsPath(), "# Mine\n")
	for _, want := range []string{blockBegin, "Call delegate with the plan as task", "hands coding work to Command Code", "mcp__harness-bridge__delegate", blockEnd} {
		if err := claudePlanner.register(cli, fakeExe, Config{Planner: "claude", Implementer: "commandcode"}); err != nil {
			t.Fatal(err)
		}
		if text := read(t, claudeInstructionsPath()); !strings.Contains(text, "# Mine\n") || !strings.Contains(text, want) {
			t.Fatalf("CLAUDE.md missing %q:\n%s", want, text)
		}
	}
	if text := read(t, claudeInstructionsPath()); strings.Count(text, blockBegin) != 1 {
		t.Fatalf("CLAUDE.md has more than one block:\n%s", text)
	}
	if !strings.Contains(calls(t, log), "claude mcp add --scope user harness-bridge") {
		t.Errorf("missing mcp add call")
	}
}

func TestDisconnectClaudeRemovesInstructions(t *testing.T) {
	log := fakeCLIs(t, "claude")
	write(t, claudeSettingsPath(), `{"permissions":{"allow":["Bash(ls)","mcp__harness-bridge"]}}`)
	write(t, claudeInstructionsPath(), "# Mine\n\n"+block("x\n")+"\n")
	useConfig(t, Config{Planner: "claude", Implementer: "codex"})
	out, code := disconnect(t, "--planner", "claude")
	if code != exitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if text := read(t, claudeInstructionsPath()); text != "# Mine\n" {
		t.Fatalf("CLAUDE.md: %q", text)
	}
	if text := read(t, claudeSettingsPath()); strings.Contains(text, "mcp__harness-bridge") {
		t.Fatalf("settings still allow harness-bridge:\n%s", text)
	}
	if !strings.Contains(calls(t, log), "claude mcp remove harness-bridge --scope user") {
		t.Errorf("missing mcp remove call in:\n%s", calls(t, log))
	}

	write(t, claudeInstructionsPath(), block("x\n")+"\n")
	if out, code := disconnect(t, "--planner", "claude"); code != exitOK || fileExists(claudeInstructionsPath()) {
		t.Fatalf("exit %d: a CLAUDE.md that held only the block is still there\n%s", code, out)
	}
}

func TestDisconnectOtherPlanners(t *testing.T) {
	log := fakeCLIs(t, "codex", "opencode", "command-code")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	home := os.Getenv("HOME")
	opencodeJSON := filepath.Join(home, ".config", "opencode", "opencode.json")
	write(t, opencodeJSON, `{"theme":"dark","mcp":{"other":{"type":"local"},"harness-bridge":{"type":"local"},"servers":{"harness-bridge":{"type":"local"}}}}`)
	write(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"), "# Mine\n\n"+block("x\n")+"\n")
	write(t, commandCodeInstructionsPath(), block("x\n")+"\n")

	out, code := disconnect(t, "--all")
	if code != exitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	got := calls(t, log)
	for _, want := range []string{"codex mcp remove harness-bridge", "command-code mcp remove --scope user harness-bridge"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing call %q in:\n%s", want, got)
		}
	}
	if text := read(t, opencodeJSON); strings.Contains(text, "harness-bridge") || !strings.Contains(text, `"other"`) || !strings.Contains(text, `"theme": "dark"`) {
		t.Errorf("opencode.json:\n%s", text)
	}
	if text := read(t, filepath.Join(home, ".config", "opencode", "AGENTS.md")); text != "# Mine\n" {
		t.Errorf("OpenCode AGENTS.md: %q", text)
	}
	if fileExists(commandCodeInstructionsPath()) {
		t.Error("Command Code's AGENTS.md held only the block but was kept")
	}
}

func TestDisconnectNeedsAPlanner(t *testing.T) {
	emptyPath(t)
	if out, code := disconnect(t); code != exitUsage || !strings.Contains(out, "pass --planner NAME or --all") {
		t.Fatalf("exit %d: %s", code, out)
	}
	if out, code := disconnect(t, "--planner", "muse"); code != exitUsage || !strings.Contains(out, "use one of codex, claude, opencode, commandcode") {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestOpenCodePlannerGetsInstructions(t *testing.T) {
	fakeCLIs(t, "opencode")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	cli, _ := findBin("opencode")
	if err := opencodePlanner.register(cli, fakeExe, Config{Planner: "opencode", Implementer: "claude"}); err != nil {
		t.Fatal(err)
	}
	if text := read(t, filepath.Join(os.Getenv("HOME"), ".config", "opencode", "AGENTS.md")); !strings.Contains(text, "hands coding work to Claude Code") {
		t.Fatalf("AGENTS.md:\n%s", text)
	}
}

func TestUninstallRemovesEverything(t *testing.T) {
	fakeCLIs(t, "claude", "codex", "opencode", "command-code")
	root := t.TempDir()
	p := uninstallPlan{
		config:    filepath.Join(root, "config", "harness-bridge"),
		jobs:      filepath.Join(root, "cache", "harness-bridge"),
		pathEntry: filepath.Join(root, "Programs", "harness-bridge"),
	}
	write(t, filepath.Join(p.config, "config.json"), `{"implementer":"codex"}`)
	write(t, filepath.Join(p.jobs, "jobs", "20261106-000000-abcdef", "request.json"), "{}")
	if runtime.GOOS == "windows" {
		p.programDir = p.pathEntry
		p.program = filepath.Join(p.programDir, "harness-bridge.exe")
	} else {
		p.program = filepath.Join(root, "bin", "harness-bridge")
	}
	write(t, p.program, "not really a program")
	var removedFromPath string
	old := removeFromUserPath
	removeFromUserPath = func(dir string) error { removedFromPath = dir; return nil }
	t.Cleanup(func() { removeFromUserPath = old })

	var out bytes.Buffer
	if code := uninstall(p, func(string) bool { return true }, &out); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	if fileExists(p.config) || fileExists(p.jobs) || removedFromPath != p.pathEntry {
		t.Fatalf("left behind: config %v, jobs %v, PATH %q\n%s", fileExists(p.config), fileExists(p.jobs), removedFromPath, out.String())
	}
	// On Windows the program's folder goes a moment after the process exits.
	deadline := time.Now().Add(20 * time.Second)
	for fileExists(p.program) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if fileExists(p.program) {
		t.Fatalf("the program is still there:\n%s", out.String())
	}
}

func TestUninstallChangesNothingWhenDeclined(t *testing.T) {
	log := fakeCLIs(t, "claude")
	dir := filepath.Join(t.TempDir(), "harness-bridge")
	write(t, filepath.Join(dir, "config.json"), "{}")
	var out bytes.Buffer
	if code := uninstall(uninstallPlan{config: dir}, func(string) bool { return false }, &out); code != exitFailed || !strings.Contains(out.String(), "Nothing was changed.") {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	if !fileExists(dir) || calls(t, log) != "" {
		t.Fatalf("a declined uninstall changed something: %s", calls(t, log))
	}
}

// A failed step must not stop the rest of the disconnect, and a planner still
// connected must stay the saved one.
func TestDisconnectContinuesAfterAFailedStep(t *testing.T) {
	fakeCLIs(t, "claude")
	t.Setenv(fakeEnv+"_FAIL", "mcp-remove")
	settings := claudeSettingsPath()
	write(t, settings, `{"permissions":{"allow":["Bash(ls)","mcp__harness-bridge"]}}`)
	write(t, claudeInstructionsPath(), "# Mine\n\n"+block("x\n")+"\n")
	useConfig(t, Config{Planner: "claude", Implementer: "codex"})
	out, code := disconnect(t, "--planner", "claude")
	if code != exitFailed {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if text := read(t, claudeInstructionsPath()); text != "# Mine\n" {
		t.Fatalf("CLAUDE.md block left behind: %q", text)
	}
	if text := read(t, settings); strings.Contains(text, "mcp__harness-bridge") {
		t.Fatalf("allow rule left behind:\n%s", text)
	}
	if cfg, _ := loadConfig(); cfg.Planner != "claude" {
		t.Fatalf("config %+v", cfg)
	}
}

// With only a comment-carrying opencode.jsonc, disconnect still removes the
// AGENTS.md block, and complains about the entry only when it is still there.
func TestDisconnectOpenCodeJSONC(t *testing.T) {
	fakeCLIs(t, "opencode")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	home := os.Getenv("HOME")
	jsonc := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	agents := filepath.Join(home, ".config", "opencode", "AGENTS.md")

	write(t, jsonc, "// mine\n{\"theme\":\"dark\"}\n")
	write(t, agents, "# Mine\n\n"+block("x\n")+"\n")
	if out, code := disconnect(t, "--planner", "opencode"); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if text := read(t, agents); text != "# Mine\n" {
		t.Fatalf("AGENTS.md: %q", text)
	}
	if text := read(t, jsonc); text != "// mine\n{\"theme\":\"dark\"}\n" {
		t.Fatalf("opencode.jsonc was changed:\n%s", text)
	}

	write(t, jsonc, "// mine\n{\"mcp\":{\"harness-bridge\":{\"type\":\"local\"}}}\n")
	write(t, agents, "# Mine\n\n"+block("x\n")+"\n")
	if out, code := disconnect(t, "--planner", "opencode"); code != exitFailed || !strings.Contains(out, "opencode.jsonc") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if text := read(t, agents); text != "# Mine\n" {
		t.Fatalf("AGENTS.md block left behind: %q", text)
	}
}

// TestRemoveFromUserPathScriptParses checks the generated PowerShell is valid
// without running it.
func TestRemoveFromUserPathScriptParses(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the PowerShell parser check runs on Windows only")
	}
	script := removeFromUserPathScript(filepath.Join(t.TempDir(), "Programs", "harness-bridge"))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`$e=$null; [void][System.Management.Automation.Language.Parser]::ParseInput($args[0], [ref]$null, [ref]$e); if ($e) { $e; exit 1 }`, script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the removeFromUserPath script does not parse: %v\n%s", err, out)
	}
}

func TestRemoveOursRefusesOtherFolders(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Documents")
	write(t, filepath.Join(dir, "keep.txt"), "x")
	if err := removeOurs(dir); err == nil || !fileExists(dir) {
		t.Fatalf("removeOurs deleted %s (err %v)", dir, err)
	}
}

// Uninstalling a HARNESS_BRIDGE_HOME jobs folder must take only harness-bridge's
// own job folders, not the user's files next to them.
func TestRemoveOursJobsKeepsForeignFiles(t *testing.T) {
	jobs := filepath.Join(t.TempDir(), "jobs")
	write(t, filepath.Join(jobs, "20261106-000000-abcdef", "request.json"), "{}")
	write(t, filepath.Join(jobs, "mine.txt"), "keep me")
	write(t, filepath.Join(jobs, "not-a-job", "x.txt"), "keep me too")
	if err := removeOurs(jobs); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(jobs, "20261106-000000-abcdef")) {
		t.Fatal("the job folder was kept")
	}
	if !fileExists(filepath.Join(jobs, "mine.txt")) || !fileExists(filepath.Join(jobs, "not-a-job")) {
		t.Fatal("the user's own files were deleted")
	}
}

func TestCommandCodeRefusedEditsNeedPermission(t *testing.T) {
	t.Setenv(fakeEnv, "commandcode-yolo")
	r, code := bridge(t, "run", "--to", "commandcode", "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "edit the readme")
	if code != exitFailed || r.Status != statusNeedsPermission || !slices.Equal(r.Denied, []string{"edit_file (needs --yolo)"}) || !strings.Contains(r.Error, "Ask the user before") {
		t.Fatalf("exit %d, report %+v", code, r)
	}
}

func TestCommandCodeRepeatedRefusalIsNotOne(t *testing.T) {
	t.Setenv(fakeEnv, "commandcode-resumed")
	r, code := bridge(t, "run", "--to", "commandcode", "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "edit the readme")
	if code != exitOK || r.Status != statusCompleted || len(r.Denied) != 0 {
		t.Fatalf("exit %d, report %+v", code, r)
	}
}
