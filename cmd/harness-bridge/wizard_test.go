package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeExe = "/opt/harness-bridge/harness-bridge"

func runWizard(t *testing.T, input string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	w := &wizard{in: bufio.NewReader(strings.NewReader(input)), out: &out, exe: fakeExe}
	code := w.run()
	return out.String(), code
}

func calls(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// Answers by name, so the test does not depend on which real CLIs exist here.
// Extra blank lines accept the defaults of any sign-in questions.
const claudeThenCodex = "claude\ncodex\n1\n\n\n\n\n"

func TestWizardConnectsClaudeToCodex(t *testing.T) {
	log := fakeCLIs(t, "claude", "codex")
	t.Cleanup(func() {
		if path, err := configPath(); err == nil {
			os.Remove(path)
		}
	})
	out, code := runWizard(t, claudeThenCodex)
	if code != exitOK || !strings.Contains(out, "All set. Open Claude Code in any project") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	got := calls(t, log)
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
	if settings := read(t, filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "settings.json")); !strings.Contains(settings, `"mcp__harness-bridge"`) {
		t.Errorf("settings:\n%s", settings)
	}
	cfg, _ := loadConfig()
	if cfg.Planner != "claude" || cfg.Implementer != "codex" || cfg.Router != "" {
		t.Errorf("config %+v", cfg)
	}
}

func TestWizardInstallsWhatIsMissing(t *testing.T) {
	log := fakeCLIs(t, "claude", "npm")
	t.Cleanup(func() {
		if path, err := configPath(); err == nil {
			os.Remove(path)
		}
	})
	out, code := runWizard(t, claudeThenCodex)
	if code != exitOK {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, want := range []string{"2) Codex CLI     will be installed", "Install Codex CLI:  npm install --global @openai/codex", "✓ Installed Codex CLI"} {
		if !strings.Contains(out, want) && !strings.Contains(out, strings.Replace(want, "2)", "1)", 1)) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if got := calls(t, log); !strings.Contains(got, "npm install --global @openai/codex") {
		t.Errorf("npm was not run:\n%s", got)
	}
}

func TestWizardSignsInWhenSignedOut(t *testing.T) {
	log := fakeCLIs(t, "claude", "codex")
	t.Setenv(fakeEnv+"_SIGNED_OUT", "claude")
	t.Cleanup(func() {
		if path, err := configPath(); err == nil {
			os.Remove(path)
		}
	})
	out, _ := runWizard(t, claudeThenCodex)
	if !strings.Contains(calls(t, log), "claude auth login") || !strings.Contains(out, "✓ Signed in to Claude Code") {
		t.Fatalf("no sign-in:\n%s\n%s", out, calls(t, log))
	}
}

func TestWizardChangesNothingWhenDeclined(t *testing.T) {
	log := fakeCLIs(t, "claude", "codex")
	out, code := runWizard(t, "claude\ncodex\n1\nn\n")
	if code != exitFailed || !strings.Contains(out, "Nothing was changed.") || calls(t, log) != "" {
		t.Fatalf("exit %d:\n%s\ncalls: %s", code, out, calls(t, log))
	}
	if path, _ := configPath(); fileExists(path) {
		t.Fatal("a declined setup saved its choices")
	}
}

func TestWizardAsksAgainForABadAnswer(t *testing.T) {
	fakeCLIs(t, "claude", "codex")
	out, _ := runWizard(t, "42\nclaude\ncodex\n7\n1\nn\n")
	if strings.Count(out, "Please type one of the numbers.") != 1 || !strings.Contains(out, "Please type 1, 2 or 3.") {
		t.Fatalf("%s", out)
	}
}

func TestWizardNeverOffersClaudeTwice(t *testing.T) {
	fakeCLIs(t, "claude", "codex")
	out, _ := runWizard(t, "claude\nclaude\ncodex\n1\nn\n")
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
			log := fakeCLIs(t, "claude", "codex", "opencode", "command-code")
			t.Setenv(fakeEnv+"_VERSION", tc.version)
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("OPENCODE_CONFIG_DIR", "")
			t.Cleanup(func() {
				if path, err := configPath(); err == nil {
					os.Remove(path)
				}
			})
			implementer := "claude"
			out, code := runWizard(t, tc.planner+"\n"+implementer+"\n1\n\n\n\n\n")
			if code != exitOK {
				t.Fatalf("exit %d:\n%s", code, out)
			}
			got := calls(t, log)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing call %q in:\n%s", want, got)
				}
			}
			if tc.file != "" {
				if text := read(t, filepath.Join(os.Getenv("HOME"), filepath.FromSlash(tc.file))); !strings.Contains(text, tc.contains) {
					t.Errorf("%s lacks %q:\n%s", tc.file, tc.contains, text)
				}
			}
		})
	}
}

func TestWizardOffersOnlyPlannersItCanConnect(t *testing.T) {
	fakeCLIs(t, "claude", "codex")
	out, _ := runWizard(t, "claude\ncodex\n1\nn\n")
	plan := out[:strings.Index(out, "Which agent should write the code?")]
	if strings.Contains(plan, "Muse Code") || !strings.Contains(plan, "Command Code") {
		t.Fatalf("planners offered:\n%s", plan)
	}
}

func TestSetupFlagsConnectWithoutAsking(t *testing.T) {
	log := fakeCLIs(t, "claude", "codex")
	t.Cleanup(func() {
		if path, err := configPath(); err == nil {
			os.Remove(path)
		}
	})
	out, code := setup(t, "--planner", "claude", "--implementer", "codex", "--router", "laya")
	if code != exitOK || !strings.Contains(calls(t, log), "claude mcp add --scope user harness-bridge -- ") {
		t.Fatalf("exit %d:\n%s\n%s", code, out, calls(t, log))
	}
	if cfg, _ := loadConfig(); cfg.Router != "laya" || cfg.Implementer != "codex" {
		t.Fatalf("config %+v", cfg)
	}
}
