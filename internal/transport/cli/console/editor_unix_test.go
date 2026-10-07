//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package console

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"multiharness-core/internal/transport/cli/term"
)

// The editor needs a real terminal for raw mode, so the test re-runs itself
// under a Python PTY that types each step once the editor has enabled
// bracketed paste, which it does right after switching the terminal to raw mode.
func TestMultilineEditorPTY(t *testing.T) {
	if os.Getenv("MULTIHARNESS_EDITOR_TEST") == "1" {
		original, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), term.GetTermios)
		if err != nil {
			t.Fatal("terminal unavailable")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		reader := NewReader(os.Stdin, os.Stdout)
		read := func(name, want string, choice bool) {
			t.Helper()
			var got string
			var err error
			if choice {
				got, err = reader.ReadChoice(ctx, 1<<20, nil)
			} else {
				got, err = reader.ReadCommand(ctx, 1<<20, nil)
			}
			if err != nil || got != want {
				t.Fatalf("%s: got %q (%v), want %q", name, got, err, want)
			}
		}
		read("typed line breaks", "first line\nsecond\nthird", false)
		read("pasted line breaks", "pasted one\n\tpasted two", false)
		// The first ↑ recalls the two-row paste, the second moves to its first
		// row, and only the third recalls the older entry.
		read("history recall", "first line\nsecond\nthird", false)
		read("word editing", "done", false)
		read("single-line choice", "model one two", true)
		// Twenty pasted lines exceed the ten-row terminal: the editor scrolls,
		// counts the hidden rows, and still returns the whole text.
		tall := make([]string, 20)
		for i := range tall {
			tall[i] = fmt.Sprintf("line %d", i+1)
		}
		read("tall input", strings.Join(tall, "\n"), false)
		// The driver writes the next line while the terminal is still cooked, so
		// its Enter reaches the editor as LF and must still submit.
		time.Sleep(500 * time.Millisecond)
		read("typed ahead", "typed ahead", false)
		after, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), term.GetTermios)
		if err != nil || !sameRestoredTerminal(after, original) {
			t.Fatal("terminal settings were not restored")
		}
		return
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 is required for the terminal regression test")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const script = `
import fcntl, os, pty, re, select, struct, subprocess, sys, termios, time
steps = [
 b'first line\x1b\rsecond\\\rthird\r',
 b'\x1b[200~pasted one\r\n\tpasted two\x1b[201~\r',
 b'\x1b[A\x1b[A\x1b[A\r',
 b'xx done extra\x17\x7f\x01\x1b[3~\x1b[3~\x1b[3~\r',
 b'\x1b[200~model one\ntwo\x1b[201~\n',
 b'\x1b[200~' + b'\n'.join(b'line %d' % i for i in range(1, 21)) + b'\x1b[201~' + b'\x1b[A' * 9 + b'\r',
]
# Scripted drivers and fast typists queue input before the prompt switches the
# terminal to raw mode, where the line discipline turns CR into LF.
typed_ahead = b'typed ahead\r'
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 10, 80, 0, 0))
env = os.environ.copy()
env['MULTIHARNESS_EDITOR_TEST'] = '1'
env['TERM'] = 'xterm-256color'
process = subprocess.Popen([sys.argv[1], '-test.run=^TestMultilineEditorPTY$', '-test.v'], stdin=slave, stdout=slave, stderr=slave, env=env)
os.close(slave)
output = b''
sent = 0
deadline = time.monotonic() + 20
try:
 while time.monotonic() < deadline:
  if select.select([master], [], [], .1)[0]:
   try: data = os.read(master, 4096)
   except OSError: break
   if not data: break
   output += data
   if sent < len(steps) and output.count(b'\x1b[?2004h') > sent:
    os.write(master, steps[sent])
    sent += 1
   if typed_ahead and output.count(b'\x1b[?2004l') == len(steps):
    os.write(master, typed_ahead)
    typed_ahead = None
  elif process.poll() is not None: break
 process.wait(timeout=2)
 text = output.decode(errors='replace')
 assert process.returncode == 0, text
 assert b'PASS' in output, text
 assert re.search(r'❯ first line\r*\n\s+second\r*\n\s+third', text), 'submitted rows were not kept in the transcript: ' + repr(text)
 assert 'Enter sends' in text, 'multi-line hint missing: ' + repr(text)
 assert '\u2191 12 more \u00b7 Enter sends' in text, 'rows above were not counted: ' + repr(text)
 assert '\u2191 10 more \u00b7 \u2193 2 more \u00b7 Enter sends' in text, 'scrolling did not follow the cursor: ' + repr(text)
 assert re.search(r'\u276f line 1\r*\n(\s+line \d+\r*\n){18}\s+line 20', text), 'tall input was not kept in the transcript: ' + repr(text)
finally:
 if process.poll() is None: process.kill(); process.wait()
 os.close(master)
`
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, python, "-c", script, binary).CombinedOutput()
	if err != nil {
		t.Fatalf("editor terminal regression failed: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "FAIL") {
		t.Fatalf("editor terminal regression reported a failure:\n%s", output)
	}
}

func TestEditorHintFitsTheTerminalWidth(t *testing.T) {
	if hint := editorHint(80, 0, 0); !strings.Contains(hint, "Shift+Enter") || !strings.Contains(hint, "\\+Enter") {
		t.Fatalf("wide hint: %q", hint)
	}
	if hint := editorHint(40, 0, 0); hint != "  Enter sends · \\+Enter adds a line" {
		t.Fatalf("narrow hint: %q", hint)
	}
	if hint := editorHint(24, 0, 0); hint != "" {
		t.Fatalf("hint on a tiny terminal: %q", hint)
	}
	if hint := editorHint(80, 3, 1); !strings.HasPrefix(hint, "  ↑ 3 more · ↓ 1 more · ") {
		t.Fatalf("scrolled hint: %q", hint)
	}
	if hint := editorHint(24, 3, 0); hint != "  ↑ 3 more" {
		t.Fatalf("scrolled hint on a tiny terminal: %q", hint)
	}
}
