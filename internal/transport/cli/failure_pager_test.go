package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"multiharness-core/internal/adapter/agent/activity"
)

func TestFailurePagerKeepsLongOutputAndControlsAccessible(t *testing.T) {
	var output strings.Builder
	for i := range 50 {
		fmt.Fprintf(&output, "line %03d: %s\n", i, strings.Repeat("界", 40))
	}
	output.WriteString("last error: permission denied")
	p := newFailurePager([]activity.Event{
		{Agent: activity.Codex, Summary: "command exited 2", Text: output.String()},
		{Agent: activity.Codex, Summary: "command exited 1", Text: "second error"},
	}, 3)
	v := &interactiveView{}
	first := p.render(v, 60, 15)
	if !strings.Contains(first, "line 000") || strings.Contains(first, "last error") {
		t.Fatal(first)
	}
	for _, size := range [][2]int{{60, 15}, {35, 10}, {120, 40}, {12, 5}} {
		p.input("\x1b[F")
		frame := p.render(v, size[0], size[1])
		if strings.ContainsAny(frame, "\r\n") {
			t.Fatal("frame can scroll the terminal", frame)
		}
		matches := regexp.MustCompile(`\x1b\[(\d+);1H([^\x1b]*)`).FindAllStringSubmatch(frame, -1)
		for _, match := range matches {
			row, _ := strconv.Atoi(match[1])
			if row > size[1] || runewidth.StringWidth(match[2]) >= size[0] {
				t.Fatalf("frame exceeds terminal: %q", match)
			}
		}
		if size[0] >= 60 && (!strings.Contains(frame, "Tool failure details") || !strings.Contains(frame, "command exited 2") || !strings.Contains(frame, "permission denied") || !strings.Contains(frame, "collapse")) {
			t.Fatal(frame)
		}
	}
	p.input("\x1b[H")
	if frame := p.render(v, 60, 15); !strings.Contains(frame, "line 000") {
		t.Fatal(frame)
	}
	p.input("\x1b[6~")
	if frame := p.render(v, 60, 15); strings.Contains(frame, "line 000") {
		t.Fatal("page down did not move")
	}
	p.input("\x1b[C")
	if frame := p.render(v, 60, 15); !strings.Contains(frame, "second error") || !strings.Contains(frame, "2/2") {
		t.Fatal(frame)
	}
	p.input("\x1b[D")
	if frame := p.render(v, 60, 15); !strings.Contains(frame, "line 000") {
		t.Fatal(frame)
	}
}

func TestFailurePagerMouseAndCloseControls(t *testing.T) {
	p := newFailurePager(nil, 0)
	for _, key := range []string{"\x1b[<0;3;10M", "\x1b[<0;4;1M", "\x1b[<0;3;1m"} {
		if p.input(key) {
			t.Fatalf("unrelated click closed details: %q", key)
		}
	}
	p.input("\x1b[<65;10;10M")
	if p.offset != 3 {
		t.Fatal("mouse wheel did not scroll")
	}
	p.input("\x1b[<64;10;10M")
	if p.offset != 0 {
		t.Fatal("mouse wheel did not scroll back")
	}
	for _, key := range []string{"\x1b[<0;3;1M", "\r", "\n", "\x1b", "d"} {
		if !p.input(key) {
			t.Fatalf("close control ignored: %q", key)
		}
	}
}
