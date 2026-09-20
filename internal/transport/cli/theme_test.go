package cli

import (
	"bytes"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

func TestThemeUsesReferencePaletteAndResetsEveryLine(t *testing.T) {
	var out bytes.Buffer
	view := &interactiveView{writer: &out, color: true, trueColor: true, width: 77}
	readinessPreview(t, view, false)
	text := out.String()
	for _, color := range []string{"48;2;24;26;32", "38;2;226;229;237", "38;2;115;218;242", "38;2;143;223;157", "38;2;255;203;107", "38;2;160;170;185"} {
		if !strings.Contains(text, "\x1b["+color+"m") && !strings.Contains(text, "\x1b[1;"+color+"m") {
			t.Fatalf("missing palette color %s", color)
		}
	}
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if !strings.HasPrefix(line, terminalBase(true)) || !strings.HasSuffix(line, "\x1b[0m") {
			t.Fatal("theme escaped its output line", line)
		}
	}
	if strings.Contains(text, "\x1b]") {
		t.Fatal("must not change host terminal settings")
	}
}

func TestResultThemePreservesCodeIndentation(t *testing.T) {
	const answer = "Example:\n```go\nfunc main() {\n    println(\"hello\")\n}\n```"
	for _, color := range []bool{false, true} {
		var out bytes.Buffer
		view := &interactiveView{writer: &out, color: color, width: 77}
		if err := view.result(store.TaskOutput{Status: store.TaskStatusResponded, Summary: answer}); err != nil {
			t.Fatal(err)
		}
		plain := terminalSGR.ReplaceAllString(out.String(), "")
		if !strings.Contains(plain, "\n        println(\"hello\")") {
			t.Fatal("code indentation lost", plain)
		}
		if !color && strings.Contains(out.String(), "\x1b") {
			t.Fatal("plain output contains styles")
		}
	}
}

func TestTerminalPaletteCapabilityAndNoColor(t *testing.T) {
	for _, capability := range []string{"", "truecolor", "24bit"} {
		for _, mode := range []string{"always", "never"} {
			var out bytes.Buffer
			lookup := func(key string) (string, bool) {
				if key == "COLORTERM" {
					return capability, true
				}
				return "", false
			}
			cfg := config.Defaults()
			cfg.Color = mode
			view := &interactiveView{writer: &out}
			view.configure(cfg, lookup)
			if err := view.notice("Saved settings", false); err != nil {
				t.Fatal(err)
			}
			output := out.String()
			if mode == "never" {
				if strings.Contains(output, "\x1b") {
					t.Fatal("color never ignored")
				}
				continue
			}
			if capability == "" {
				if !strings.Contains(output, "\x1b[38;5;114m") || strings.Contains(output, "38;2;") {
					t.Fatal("256-color fallback lost", output)
				}
			} else if !strings.Contains(output, "\x1b[38;2;143;223;157m") {
				t.Fatal("truecolor capability ignored", output)
			}
		}
	}
}
