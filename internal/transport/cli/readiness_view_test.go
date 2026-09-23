package cli

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
)

var styleSequence = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func readinessPreview(t *testing.T, view *interactiveView, ready bool) {
	t.Helper()
	cfg := config.Defaults()
	cfg.Mode = "team"
	cfg.Implementer = config.DefaultImplementer("codex")
	cfg.Implementer.Model = "gpt-5.6-terra"
	cfg.Decision.Enabled = true
	h := &Handler{stdout: view.writer}
	h.SetReadiness(func(_ context.Context, r account.Request) account.Status {
		if !ready && r.Model == "gpt-5.6-terra" {
			return account.Status{Detail: "Account is not signed in. Use /login codex, then /configuration to check again."}
		}
		return account.Status{Ready: true, Detail: "CLI reports signed in"}
	}, func(context.Context, config.Config, bool) account.Status {
		return account.Status{Ready: true, Detail: "OpenRouter key accepted; model access and account credits can still limit requests"}
	})
	got, err := h.readiness(t.Context(), cfg, view, false)
	if err != nil || got != ready {
		t.Fatal(got, err)
	}
}

func TestReadinessLayoutFitsTerminalAndRetainsStatusWithoutColor(t *testing.T) {
	for _, columns := range []int{24, 40, 80, 120} {
		for _, color := range []bool{false, true} {
			for _, ready := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/color=%t/ready=%t", columns, color, ready), func(t *testing.T) {
					var out bytes.Buffer
					view := &interactiveView{writer: &out, width: columns - 3, color: color}
					readinessPreview(t, view, ready)
					plain := styleSequence.ReplaceAllString(out.String(), "")
					for _, line := range strings.Split(plain, "\n") {
						if runewidth.StringWidth(line) >= columns || (line != "" && !strings.HasPrefix(line, "  ")) {
							t.Fatalf("lost indentation or overflow at %d columns: %q", columns, line)
						}
					}
					for _, label := range []string{"AGENTS", "Planner", "Implementer", "Reviewer", "OPTIONAL SERVICES", "Jev routing", "Fallbacks", "✓ READY"} {
						if !strings.Contains(plain, label) {
							t.Fatalf("missing %s: %s", label, plain)
						}
					}
					if !ready && (!strings.Contains(plain, "! NEEDS SETUP") || !strings.Contains(plain, "/setup")) {
						t.Fatal("missing status or next action", plain)
					}
					if color {
						if !strings.Contains(out.String(), "\x1b[1;38;5;117mPlanner") || !strings.Contains(out.String(), "\x1b[38;5;114m✓ READY") || (!ready && !strings.Contains(out.String(), "\x1b[38;5;221m! NEEDS SETUP")) {
							t.Fatal("missing semantic colors", out.String())
						}
					} else if strings.Contains(out.String(), "\x1b") {
						t.Fatal("plain output contains escapes")
					}
				})
			}
		}
	}
}

func TestWrappedSettingsAndNoticesPreserveUnicodeAndLongValues(t *testing.T) {
	for _, columns := range []int{24, 40, 80} {
		var out bytes.Buffer
		view := &interactiveView{writer: &out, width: columns - 3}
		value := strings.Repeat("界e\u0301", 35)
		cfg := config.Defaults()
		cfg.WorkingDir = "/workspace/" + value
		if err := view.settings(cfg); err != nil {
			t.Fatal(err)
		}
		if err := view.notice(value, true); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(out.String(), "\n") {
			if !utf8.ValidString(line) || runewidth.StringWidth(line) >= columns || (line != "" && !strings.HasPrefix(line, "  ")) {
				t.Fatal(columns, line)
			}
		}
		lines := wrapTerminal(value, columns-5)
		if strings.Join(lines, "") != value {
			t.Fatal("Unicode value changed during wrapping")
		}
	}
}
