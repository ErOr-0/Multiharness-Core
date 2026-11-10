package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func setup(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := cli(append([]string{"setup"}, args...), strings.NewReader(""), &out, &out)
	return out.String(), code
}

// emptyPath leaves only dir on PATH and points the home and app-data folders
// at it, so no real CLI or installer is found.
func emptyPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	for _, v := range []string{"HOME", "USERPROFILE", "APPDATA"} {
		t.Setenv(v, dir)
	}
	return dir
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSetupClaudePlannerWritesAgentFiles(t *testing.T) {
	emptyPath(t)
	project := t.TempDir()
	claudeMD := filepath.Join(project, "CLAUDE.md")
	if err := os.WriteFile(claudeMD, []byte("# My project\n\nUse tabs.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(project, ".claude", "settings.json")
	_ = os.MkdirAll(filepath.Dir(settings), 0o755)
	if err := os.WriteFile(settings, []byte(`{"model":"opus","permissions":{"deny":["Edit"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := setup(t, "--planner", "claude", "--implementer", "codex", "--dir", project, "--lock-planner", "--router", "laya")
	// Nothing is installed here, so setup reports what is missing but still
	// writes the files.
	if code != exitFailed || !strings.Contains(out, "Claude Code (planner) is not installed") || !strings.Contains(out, "harness-bridge itself is not on PATH") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	sub := read(t, filepath.Join(project, ".claude", "agents", "codex-implementer.md"))
	if !strings.Contains(sub, "name: codex-implementer") || !strings.Contains(sub, "harness-bridge start --to codex --dir .") || !strings.Contains(sub, "--session <session_id>") {
		t.Fatalf("subagent:\n%s", sub)
	}
	md := read(t, claudeMD)
	for _, want := range []string{"# My project\n\nUse tabs.\n\n" + blockBegin, "`codex-implementer` subagent", "harness-bridge route --provider laya", "network_access=true", blockEnd} {
		if !strings.Contains(md, want) {
			t.Fatalf("CLAUDE.md lacks %q:\n%s", want, md)
		}
	}
	var s struct {
		Model       string `json:"model"`
		Permissions struct {
			Allow, Deny []string
		} `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(read(t, settings)), &s); err != nil {
		t.Fatal(err)
	}
	if s.Model != "opus" || strings.Join(s.Permissions.Deny, ",") != "Edit,Write,NotebookEdit" || s.Permissions.Allow[0] != "Bash(harness-bridge:*)" {
		t.Fatalf("settings %+v", s)
	}
	// Rerunning replaces the block instead of adding a second one.
	setup(t, "--planner", "claude", "--implementer", "codex", "--dir", project)
	if md := read(t, claudeMD); strings.Count(md, blockBegin) != 1 || strings.Contains(md, "harness-bridge route") {
		t.Fatalf("after rerun:\n%s", md)
	}
}

func TestSetupOtherPlannersWriteAgentsMD(t *testing.T) {
	emptyPath(t)
	for _, tc := range []struct {
		planner, implementer string
		want                 []string
	}{
		{"codex", "claude", []string{"harness-bridge start --to claude --dir . --allow 'Bash(go test:*)'", "outside the sandbox", "`--session <session_id>`"}},
		{"opencode", "muse", []string{"harness-bridge start --to muse --dir .", "cannot continue a session", "cannot run checks"}},
		{"commandcode", "opencode", []string{"harness-bridge start --to opencode --dir .", "pass `--auto` as an extra argument"}},
	} {
		t.Run(tc.planner, func(t *testing.T) {
			project := t.TempDir()
			setup(t, "--planner", tc.planner, "--implementer", tc.implementer, "--dir", project)
			md := read(t, filepath.Join(project, "AGENTS.md"))
			for _, want := range tc.want {
				if !strings.Contains(md, want) {
					t.Fatalf("AGENTS.md lacks %q:\n%s", want, md)
				}
			}
			if _, err := os.Stat(filepath.Join(project, ".claude")); err == nil {
				t.Fatal("a non-Claude planner got Claude files")
			}
		})
	}
}

func TestSetupAsksBeforeInstalling(t *testing.T) {
	bin := emptyPath(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	npm := filepath.Join(bin, "npm")
	if runtime.GOOS == "windows" {
		npm += ".exe"
	}
	if err := os.WriteFile(npm, data, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeEnv, "npm-installer")

	// No terminal and no --yes: show the installer but do not run it.
	out, code := setup(t, "--implementer", "codex")
	if code != exitFailed || !strings.Contains(out, "npm install --global @openai/codex") || !strings.Contains(out, "not asked: no terminal") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if entries, _ := os.ReadDir(bin); len(entries) != 1 {
		t.Fatalf("the installer ran without consent: %v", entries)
	}

	out, _ = setup(t, "--implementer", "codex", "--yes")
	if !strings.Contains(out, "added 1 package: install --global @openai/codex") || !strings.Contains(out, "✓ Codex CLI is installed. Sign in with `codex login`") {
		t.Fatalf("with --yes:\n%s", out)
	}
	out, _ = setup(t, "--implementer", "codex")
	if !strings.Contains(out, "✓ Codex CLI (implementer) is installed") {
		t.Fatalf("after installing:\n%s", out)
	}
}

func TestConfirmerReadsTheTerminalAnswer(t *testing.T) {
	var out bytes.Buffer
	if confirmer(strings.NewReader("y\n"), &out, false)("Run it now?") {
		t.Fatal("a non-terminal reader must never count as consent")
	}
	if !confirmer(nil, io.Discard, true)("Run it now?") {
		t.Fatal("--yes must accept")
	}
}

func TestSetupRejects(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "give --planner, --implementer or both"},
		{[]string{"--planner", "claude", "--implementer", "claude"}, "use its own subagents"},
		{[]string{"--planner", "gemini"}, "use one of"},
		{[]string{"--planner", "codex", "--implementer", "claude", "--lock-planner"}, "only with --planner claude"},
		{[]string{"--planner", "claude", "--dir", ".", "--lock-planner"}, "only with --planner claude"},
		{[]string{"--implementer", "codex", "--router", "gpt"}, "use jev or laya"},
	} {
		if _, err := parseSetup(tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want %q", tc.args, err, tc.want)
		}
	}
}

// A damaged block must be reported, not duplicated around or eaten together
// with the user's own text.
func TestUpsertBlockWithDamagedMarkers(t *testing.T) {
	dir := t.TempDir()
	damaged := filepath.Join(dir, "damaged.md")
	text := "# Mine\n" + blockBegin + "\nno end marker anywhere"
	write(t, damaged, text)
	if err := upsertBlock(damaged, "body"); err == nil || !strings.Contains(err.Error(), "end marker") {
		t.Fatalf("upsertBlock on a begin without end: %v", err)
	}
	if got := read(t, damaged); got != text {
		t.Fatalf("a damaged file was changed:\n%q", got)
	}
	if _, err := removeBlock(damaged); err == nil || !strings.Contains(err.Error(), "end marker") {
		t.Fatalf("removeBlock on a begin without end: %v", err)
	}
	if got := read(t, damaged); got != text {
		t.Fatalf("removeBlock changed a damaged file:\n%q", got)
	}

	// A stray end marker before a real block is ignored.
	stray := filepath.Join(dir, "stray.md")
	write(t, stray, "Stray: "+blockEnd+"\n\n"+block("old\n")+"\n")
	if err := upsertBlock(stray, "new\n"); err != nil {
		t.Fatal(err)
	}
	got := read(t, stray)
	if !strings.HasPrefix(got, "Stray: "+blockEnd+"\n\n") || !strings.Contains(got, blockBegin+"\nnew\n"+blockEnd) || strings.Contains(got, "old") {
		t.Fatalf("the stray text or the old block survived:\n%q", got)
	}
}

func TestUpsertBlockKeepsCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(path, []byte("a\r\nb\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := upsertBlock(path, "one\ntwo\n"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); strings.Count(got, "\r\n") != strings.Count(got, "\n") || !strings.HasPrefix(got, "a\r\nb\r\n\r\n") {
		t.Fatalf("%q", got)
	}
}
