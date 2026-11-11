package setup

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/cli"
	"multiharness-core/internal/config"
	"multiharness-core/internal/testkit"
)

const fakeExe = "/opt/harness-bridge/harness-bridge"

func runWizard(t *testing.T, input string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	w := &wizard{in: bufio.NewReader(strings.NewReader(input)), out: &out, exe: fakeExe}
	code := w.run()
	return out.String(), code
}

// Answers by name, so the test does not depend on which real CLIs exist here.
// Extra blank lines accept the defaults of any sign-in questions.
const claudeThenCodex = "claude\ncodex\n\n\n1\n\n\n\n\n"

func TestWizardConnectsClaudeToCodex(t *testing.T) {
	log := testkit.FakeCLIs(t, "claude", "codex")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, code := runWizard(t, claudeThenCodex)
	if code != cli.ExitOK || !strings.Contains(out, "All set. Open Claude Code in any project") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	got := testkit.Calls(t, log)
	for _, want := range []string{
		"claude mcp remove harness-bridge --scope user",
		"claude mcp add --scope user harness-bridge -- " + fakeExe + " mcp",
		"claude auth status",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing call %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "npm") {
		t.Errorf("installed something although both agents were installed:\n%s", got)
	}
	if settings := testkit.Read(t, filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "settings.json")); !strings.Contains(settings, `"mcp__harness-bridge"`) {
		t.Errorf("settings:\n%s", settings)
	}
	cfg, _ := config.Load()
	if cfg.Planner != "claude" || cfg.Implementer != "codex" || cfg.Router != "" {
		t.Errorf("config %+v", cfg)
	}
}

func TestWizardInstallsWhatIsMissing(t *testing.T) {
	log := testkit.FakeCLIs(t, "claude", "npm")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, code := runWizard(t, claudeThenCodex)
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, want := range []string{"2) Codex CLI     will be installed", "Install Codex CLI:  npm install --global @openai/codex", "✓ Installed Codex CLI"} {
		if !strings.Contains(out, want) && !strings.Contains(out, strings.Replace(want, "2)", "1)", 1)) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if got := testkit.Calls(t, log); !strings.Contains(got, "npm install --global @openai/codex") {
		t.Errorf("npm was not run:\n%s", got)
	}
}

func TestWizardSignsInWhenSignedOut(t *testing.T) {
	log := testkit.FakeCLIs(t, "claude", "codex")
	t.Setenv(testkit.FakeEnv+"_SIGNED_OUT", "claude")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, _ := runWizard(t, claudeThenCodex)
	if !strings.Contains(testkit.Calls(t, log), "claude auth login") || !strings.Contains(out, "✓ Signed in to Claude Code") {
		t.Fatalf("no sign-in:\n%s\n%s", out, testkit.Calls(t, log))
	}
}

func TestWizardChangesNothingWhenDeclined(t *testing.T) {
	log := testkit.FakeCLIs(t, "claude", "codex")
	out, code := runWizard(t, "claude\ncodex\n\n\n1\nn\n")
	if code != cli.ExitFailed || !strings.Contains(out, "Nothing was changed.") || testkit.Calls(t, log) != "" {
		t.Fatalf("exit %d:\n%s\ncalls: %s", code, out, testkit.Calls(t, log))
	}
	if path, _ := config.Path(); fileExists(path) {
		t.Fatal("a declined setup saved its choices")
	}
}

func TestWizardAsksAgainForABadAnswer(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex")
	out, _ := runWizard(t, "42\nclaude\ncodex\n\n\n7\n1\nn\n")
	if strings.Count(out, "Please type one of the numbers.") != 1 || !strings.Contains(out, "Please type 1, 2 or 3.") {
		t.Fatalf("%s", out)
	}
}

func TestWizardNeverOffersClaudeTwice(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex")
	out, _ := runWizard(t, "claude\nclaude\ncodex\n\n\n1\nn\n")
	write := out[strings.Index(out, "Which agent should write the code?"):]
	if strings.Contains(write[:strings.Index(write, "Choose")], "Claude Code") {
		t.Fatalf("Claude Code was offered as implementer for itself:\n%s", out)
	}
}

func TestWizardConnectsOtherPlanners(t *testing.T) {
	for _, tc := range []struct {
		planner, version string
		want             []string // calls made
		file, contains   string   // a file the planner reads, relative to home
	}{
		{"codex", "", []string{"codex mcp add harness-bridge -- " + fakeExe + " mcp", "codex login status"}, "", ""},
		{"opencode", "2.2.5", []string{"opencode mcp add harness-bridge --global -- " + fakeExe + " mcp"}, "", ""},
		{"opencode", "1.19.5", nil, ".config/opencode/opencode.json", `"command": [` + "\n" + `        "` + fakeExe + `",`},
		{"commandcode", "", []string{"command-code mcp remove --scope user harness-bridge", "command-code mcp add --scope user harness-bridge -- " + fakeExe + " mcp", "command-code status --json"},
			".commandcode/AGENTS.md", "Call delegate with the plan as task"},
	} {
		t.Run(tc.planner+tc.version, func(t *testing.T) {
			log := testkit.FakeCLIs(t, "claude", "codex", "opencode", "command-code")
			t.Setenv(testkit.FakeEnv+"_VERSION", tc.version)
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("OPENCODE_CONFIG_DIR", "")
			t.Cleanup(func() {
				if path, err := config.Path(); err == nil {
					os.Remove(path)
				}
			})
			implementer := "claude"
			out, code := runWizard(t, tc.planner+"\n"+implementer+"\n\n\n1\n\n\n\n\n")
			if code != cli.ExitOK {
				t.Fatalf("exit %d:\n%s", code, out)
			}
			got := testkit.Calls(t, log)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing call %q in:\n%s", want, got)
				}
			}
			if tc.file != "" {
				if text := testkit.Read(t, filepath.Join(os.Getenv("HOME"), filepath.FromSlash(tc.file))); !strings.Contains(text, tc.contains) {
					t.Errorf("%s lacks %q:\n%s", tc.file, tc.contains, text)
				}
			}
		})
	}
}

func TestWizardOffersOnlyPlannersItCanConnect(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex")
	out, _ := runWizard(t, "claude\ncodex\n\n\n1\nn\n")
	plan := out[:strings.Index(out, "Which agent should write the code?")]
	if strings.Contains(plan, "Muse Code") || !strings.Contains(plan, "Command Code") {
		t.Fatalf("planners offered:\n%s", plan)
	}
}

func TestSetupFlagsConnectWithoutAsking(t *testing.T) {
	log := testkit.FakeCLIs(t, "claude", "codex")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, code := setup(t, "--planner", "claude", "--implementer", "codex", "--router", "laya")
	if code != cli.ExitOK || !strings.Contains(testkit.Calls(t, log), "claude mcp add --scope user harness-bridge -- ") {
		t.Fatalf("exit %d:\n%s\n%s", code, out, testkit.Calls(t, log))
	}
	if cfg, _ := config.Load(); cfg.Router != "laya" || cfg.Implementer != "codex" {
		t.Fatalf("config %+v", cfg)
	}
}

func TestWizardSavesTheModel(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, code := runWizard(t, "claude\ncodex\ngpt-5.2-codex\n\n1\n\n\n\n\n")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "hands the coding to Codex CLI (model gpt-5.2-codex)") {
		t.Errorf("no model in the final message:\n%s", out)
	}
	if cfg, _ := config.Load(); cfg.Model != "gpt-5.2-codex" {
		t.Errorf("config %+v", cfg)
	}
}

func TestWizardKeepsOrClearsASavedModel(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	if _, err := config.Save(config.Config{Planner: "claude", Implementer: "codex", Model: "old-model"}); err != nil {
		t.Fatal(err)
	}
	out, code := runWizard(t, claudeThenCodex)
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "Press Enter to keep old-model") {
		t.Errorf("no keep hint:\n%s", out)
	}
	if cfg, _ := config.Load(); cfg.Model != "old-model" {
		t.Errorf("config %+v", cfg)
	}
	out, code = runWizard(t, "claude\ncodex\ndefault\n\n1\n\n\n\n\n")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if cfg, _ := config.Load(); cfg.Model != "" {
		t.Errorf("config %+v", cfg)
	}
}

func TestWizardDropsTheModelOfAnotherImplementer(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex", "opencode")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	if _, err := config.Save(config.Config{Planner: "claude", Implementer: "opencode", Model: "x/y"}); err != nil {
		t.Fatal(err)
	}
	out, code := runWizard(t, claudeThenCodex)
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if cfg, _ := config.Load(); cfg.Implementer != "codex" || cfg.Model != "" {
		t.Errorf("config %+v", cfg)
	}
}

func TestWizardAsksAgainForABadModel(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex")
	out, _ := runWizard(t, "claude\ncodex\nbad name;rm\ngood-model\n\n1\nn\n")
	if got := strings.Count(out, "characters it can't contain"); got != 1 {
		t.Fatalf("rejection printed %d times:\n%s", got, out)
	}
}

func TestSetupFlagsSaveTheModel(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, code := setup(t, "--planner", "claude", "--implementer", "codex", "--model", "gpt-5.2-codex", "--reasoning", "high")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if cfg, _ := config.Load(); cfg.Model != "gpt-5.2-codex" || cfg.Reasoning != "high" {
		t.Fatalf("config %+v", cfg)
	}
	out, code = setup(t, "--planner", "claude", "--implementer", "codex")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if cfg, _ := config.Load(); cfg.Model != "gpt-5.2-codex" || cfg.Reasoning != "high" {
		t.Errorf("rerun dropped the model: %+v", cfg)
	}
}

func TestWizardListsCommandCodeModels(t *testing.T) {
	log := testkit.FakeCLIs(t, "claude", "command-code")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, code := runWizard(t, "claude\ncommandcode\n2\nhigh\n1\n\n\n\n\n")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, want := range []string{"From Command Code's own list", "  2) claude-sonnet-5-5", "(model claude-sonnet-5-5, reasoning high)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "typesafe/jev") {
		t.Errorf("decision models must not be offered:\n%s", out)
	}
	if !strings.Contains(testkit.Calls(t, log), "command-code --list-models") {
		t.Errorf("the CLI's own list was not used:\n%s", testkit.Calls(t, log))
	}
	if cfg, _ := config.Load(); cfg.Model != "claude-sonnet-5-5" || cfg.Reasoning != "high" {
		t.Errorf("config %+v", cfg)
	}
}

func TestWizardOffersClaudeAliases(t *testing.T) {
	testkit.FakeCLIs(t, "codex", "claude")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, code := runWizard(t, "codex\nclaude\nopus\n\n1\n\n\n\n\n")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, want := range []string{"always mean the newest model", "  2) opus"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if cfg, _ := config.Load(); cfg.Model != "opus" || cfg.Reasoning != "" {
		t.Errorf("config %+v", cfg)
	}
}

func TestWizardSkipsReasoningForOpenCode(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "opencode")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, code := runWizard(t, "claude\nopencode\ngpt\n1\n1\n\n\n\n\n")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "  1) openai/gpt-5.5") {
		t.Errorf("a partial name did not narrow the list:\n%s", out)
	}
	if strings.Contains(out, "How hard should") {
		t.Errorf("OpenCode takes no reasoning answer:\n%s", out)
	}
	if cfg, _ := config.Load(); cfg.Model != "openai/gpt-5.5" {
		t.Errorf("config %+v", cfg)
	}
}

func TestWizardTakesAnUnlistedModelAsTyped(t *testing.T) {
	testkit.FakeCLIs(t, "claude", "codex")
	t.Cleanup(func() {
		if path, err := config.Path(); err == nil {
			os.Remove(path)
		}
	})
	out, code := runWizard(t, "claude\ncodex\ngpt-9-preview\n\n1\n\n\n\n\n")
	if code != cli.ExitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "has no command that lists its models") {
		t.Errorf("no-list explanation missing:\n%s", out)
	}
	if cfg, _ := config.Load(); cfg.Model != "gpt-9-preview" {
		t.Errorf("config %+v", cfg)
	}
}
