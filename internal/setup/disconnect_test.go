package setup

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

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/config"
	"multiharness-core/internal/testkit"
)

func disconnect(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := CmdDisconnect(args, &out, &out)
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
			testkit.Write(t, path, tc.text)
			if _, err := removeBlock(path); err != nil {
				t.Fatal(err)
			}
			if tc.gone {
				if fileExists(path) {
					t.Fatalf("a file that held only the block is still there: %q", testkit.Read(t, path))
				}
				return
			}
			if got := testkit.Read(t, path); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDisconnectClaude(t *testing.T) {
	log := testkit.FakeCLIs(t, "claude")
	settings := claudeSettingsPath()
	testkit.Write(t, settings, `{"model":"opus","permissions":{"allow":["Bash(ls)","mcp__harness-bridge"]}}`)
	testkit.UseConfig(t, config.Config{Planner: "claude", Implementer: "codex"})
	out, code := disconnect(t)
	if code != cli.ExitOK || !strings.Contains(testkit.Calls(t, log), "claude mcp remove harness-bridge --scope user") {
		t.Fatalf("exit %d:\n%s\n%s", code, out, testkit.Calls(t, log))
	}
	var s struct {
		Model       string `json:"model"`
		Permissions struct{ Allow []string }
	}
	if err := json.Unmarshal([]byte(testkit.Read(t, settings)), &s); err != nil {
		t.Fatal(err)
	}
	if s.Model != "opus" || !slices.Equal(s.Permissions.Allow, []string{"Bash(ls)"}) {
		t.Fatalf("settings %+v", s)
	}
	if cfg, _ := config.Load(); cfg.Planner != "" || cfg.Implementer != "codex" {
		t.Fatalf("config %+v", cfg)
	}
}

func TestClaudePlannerGetsInstructions(t *testing.T) {
	log := testkit.FakeCLIs(t, "claude")
	cli, _ := agent.FindBin("claude")
	testkit.Write(t, claudeInstructionsPath(), "# Mine\n")
	for _, want := range []string{blockBegin, "Call delegate with the plan as task", "hands coding work to Command Code", "mcp__harness-bridge__delegate", blockEnd} {
		if err := claudePlanner.Register(cli, fakeExe, config.Config{Planner: "claude", Implementer: "commandcode"}); err != nil {
			t.Fatal(err)
		}
		if text := testkit.Read(t, claudeInstructionsPath()); !strings.Contains(text, "# Mine\n") || !strings.Contains(text, want) {
			t.Fatalf("CLAUDE.md missing %q:\n%s", want, text)
		}
	}
	if text := testkit.Read(t, claudeInstructionsPath()); strings.Count(text, blockBegin) != 1 {
		t.Fatalf("CLAUDE.md has more than one block:\n%s", text)
	}
	if !strings.Contains(testkit.Calls(t, log), "claude mcp add --scope user harness-bridge") {
		t.Errorf("missing mcp add call")
	}
}

func TestDisconnectClaudeRemovesInstructions(t *testing.T) {
	log := testkit.FakeCLIs(t, "claude")
	testkit.Write(t, claudeSettingsPath(), `{"permissions":{"allow":["Bash(ls)","mcp__harness-bridge"]}}`)
	testkit.Write(t, claudeInstructionsPath(), "# Mine\n\n"+block("x\n")+"\n")
	testkit.UseConfig(t, config.Config{Planner: "claude", Implementer: "codex"})
	out, code := disconnect(t, "--planner", "claude")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if text := testkit.Read(t, claudeInstructionsPath()); text != "# Mine\n" {
		t.Fatalf("CLAUDE.md: %q", text)
	}
	if text := testkit.Read(t, claudeSettingsPath()); strings.Contains(text, "mcp__harness-bridge") {
		t.Fatalf("settings still allow harness-bridge:\n%s", text)
	}
	if !strings.Contains(testkit.Calls(t, log), "claude mcp remove harness-bridge --scope user") {
		t.Errorf("missing mcp remove call in:\n%s", testkit.Calls(t, log))
	}

	testkit.Write(t, claudeInstructionsPath(), block("x\n")+"\n")
	if out, code := disconnect(t, "--planner", "claude"); code != cli.ExitOK || fileExists(claudeInstructionsPath()) {
		t.Fatalf("exit %d: a CLAUDE.md that held only the block is still there\n%s", code, out)
	}
}

func TestDisconnectOtherPlanners(t *testing.T) {
	log := testkit.FakeCLIs(t, "codex", "opencode", "command-code")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	home := os.Getenv("HOME")
	opencodeJSON := filepath.Join(home, ".config", "opencode", "opencode.json")
	testkit.Write(t, opencodeJSON, `{"theme":"dark","mcp":{"other":{"type":"local"},"harness-bridge":{"type":"local"},"servers":{"harness-bridge":{"type":"local"}}}}`)
	testkit.Write(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"), "# Mine\n\n"+block("x\n")+"\n")
	testkit.Write(t, commandCodeInstructionsPath(), block("x\n")+"\n")

	out, code := disconnect(t, "--all")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	got := testkit.Calls(t, log)
	for _, want := range []string{"codex mcp remove harness-bridge", "command-code mcp remove --scope user harness-bridge"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing call %q in:\n%s", want, got)
		}
	}
	if text := testkit.Read(t, opencodeJSON); strings.Contains(text, "harness-bridge") || !strings.Contains(text, `"other"`) || !strings.Contains(text, `"theme": "dark"`) {
		t.Errorf("opencode.json:\n%s", text)
	}
	if text := testkit.Read(t, filepath.Join(home, ".config", "opencode", "AGENTS.md")); text != "# Mine\n" {
		t.Errorf("OpenCode AGENTS.md: %q", text)
	}
	if fileExists(commandCodeInstructionsPath()) {
		t.Error("Command Code's AGENTS.md held only the block but was kept")
	}
}

func TestDisconnectNeedsAPlanner(t *testing.T) {
	testkit.EmptyPath(t)
	if out, code := disconnect(t); code != cli.ExitUsage || !strings.Contains(out, "pass --planner NAME or --all") {
		t.Fatalf("exit %d: %s", code, out)
	}
	if out, code := disconnect(t, "--planner", "muse"); code != cli.ExitUsage || !strings.Contains(out, "use one of codex, claude, opencode, commandcode") {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestOpenCodePlannerGetsInstructions(t *testing.T) {
	testkit.FakeCLIs(t, "opencode")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	cli, _ := agent.FindBin("opencode")
	if err := opencodePlanner.Register(cli, fakeExe, config.Config{Planner: "opencode", Implementer: "claude"}); err != nil {
		t.Fatal(err)
	}
	if text := testkit.Read(t, filepath.Join(os.Getenv("HOME"), ".config", "opencode", "AGENTS.md")); !strings.Contains(text, "hands coding work to Claude Code") {
		t.Fatalf("AGENTS.md:\n%s", text)
	}
}

func TestUninstallRemovesEverything(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex", "opencode", "command-code")
	root := t.TempDir()
	p := uninstallPlan{
		config:    filepath.Join(root, "config", "harness-bridge"),
		jobs:      filepath.Join(root, "cache", "harness-bridge"),
		pathEntry: filepath.Join(root, "Programs", "harness-bridge"),
	}
	testkit.Write(t, filepath.Join(p.config, "config.json"), `{"implementer":"codex"}`)
	testkit.Write(t, filepath.Join(p.jobs, "jobs", "20261106-000000-abcdef", "request.json"), "{}")
	if runtime.GOOS == "windows" {
		p.programDir = p.pathEntry
		p.program = filepath.Join(p.programDir, "harness-bridge.exe")
	} else {
		p.program = filepath.Join(root, "bin", "harness-bridge")
	}
	testkit.Write(t, p.program, "not really a program")
	var removedFromPath string
	old := removeFromUserPath
	removeFromUserPath = func(dir string) error { removedFromPath = dir; return nil }
	t.Cleanup(func() { removeFromUserPath = old })

	var out bytes.Buffer
	if code := uninstall(p, func(string) bool { return true }, &out); code != cli.ExitOK {
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
	log := testkit.FakeCLIs(t, "claude")
	dir := filepath.Join(t.TempDir(), "harness-bridge")
	testkit.Write(t, filepath.Join(dir, "config.json"), "{}")
	var out bytes.Buffer
	if code := uninstall(uninstallPlan{config: dir}, func(string) bool { return false }, &out); code != cli.ExitFailed || !strings.Contains(out.String(), "Nothing was changed.") {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	if !fileExists(dir) || testkit.Calls(t, log) != "" {
		t.Fatalf("a declined uninstall changed something: %s", testkit.Calls(t, log))
	}
}

// A failed step must not stop the rest of the disconnect, and a planner still
// connected must stay the saved one.
func TestDisconnectContinuesAfterAFailedStep(t *testing.T) {
	testkit.FakeCLIs(t, "claude")
	t.Setenv(testkit.FakeEnv+"_FAIL", "mcp-remove")
	settings := claudeSettingsPath()
	testkit.Write(t, settings, `{"permissions":{"allow":["Bash(ls)","mcp__harness-bridge"]}}`)
	testkit.Write(t, claudeInstructionsPath(), "# Mine\n\n"+block("x\n")+"\n")
	testkit.UseConfig(t, config.Config{Planner: "claude", Implementer: "codex"})
	out, code := disconnect(t, "--planner", "claude")
	if code != cli.ExitFailed {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if text := testkit.Read(t, claudeInstructionsPath()); text != "# Mine\n" {
		t.Fatalf("CLAUDE.md block left behind: %q", text)
	}
	if text := testkit.Read(t, settings); strings.Contains(text, "mcp__harness-bridge") {
		t.Fatalf("allow rule left behind:\n%s", text)
	}
	if cfg, _ := config.Load(); cfg.Planner != "claude" {
		t.Fatalf("config %+v", cfg)
	}
}

// With only a comment-carrying opencode.jsonc, disconnect still removes the
// AGENTS.md block, and complains about the entry only when it is still there.
func TestDisconnectOpenCodeJSONC(t *testing.T) {
	testkit.FakeCLIs(t, "opencode")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	home := os.Getenv("HOME")
	jsonc := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	agents := filepath.Join(home, ".config", "opencode", "AGENTS.md")

	testkit.Write(t, jsonc, "// mine\n{\"theme\":\"dark\"}\n")
	testkit.Write(t, agents, "# Mine\n\n"+block("x\n")+"\n")
	if out, code := disconnect(t, "--planner", "opencode"); code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if text := testkit.Read(t, agents); text != "# Mine\n" {
		t.Fatalf("AGENTS.md: %q", text)
	}
	if text := testkit.Read(t, jsonc); text != "// mine\n{\"theme\":\"dark\"}\n" {
		t.Fatalf("opencode.jsonc was changed:\n%s", text)
	}

	testkit.Write(t, jsonc, "// mine\n{\"mcp\":{\"harness-bridge\":{\"type\":\"local\"}}}\n")
	testkit.Write(t, agents, "# Mine\n\n"+block("x\n")+"\n")
	if out, code := disconnect(t, "--planner", "opencode"); code != cli.ExitFailed || !strings.Contains(out, "opencode.jsonc") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if text := testkit.Read(t, agents); text != "# Mine\n" {
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
	testkit.Write(t, filepath.Join(dir, "keep.txt"), "x")
	if err := removeOurs(dir); err == nil || !fileExists(dir) {
		t.Fatalf("removeOurs deleted %s (err %v)", dir, err)
	}
}

// Uninstalling a HARNESS_BRIDGE_HOME jobs folder must take only harness-bridge's
// own job folders, not the user's files next to them.
func TestRemoveOursJobsKeepsForeignFiles(t *testing.T) {
	jobs := filepath.Join(t.TempDir(), "jobs")
	testkit.Write(t, filepath.Join(jobs, "20261106-000000-abcdef", "request.json"), "{}")
	testkit.Write(t, filepath.Join(jobs, "mine.txt"), "keep me")
	testkit.Write(t, filepath.Join(jobs, "not-a-job", "x.txt"), "keep me too")
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
