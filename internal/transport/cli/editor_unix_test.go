//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/transport/cli/console"
	"multiharness-core/internal/transport/cli/screen"
	"multiharness-core/internal/transport/cli/term"
)

func TestCommandEditorPTY(t *testing.T) {
	if mode := os.Getenv("MULTIHARNESS_EDITOR_TEST"); mode != "" {
		input := console.NewReader(os.Stdin, os.Stdout)
		view := &screen.View{Writer: os.Stdout, Color: mode == "complete", Width: 77}
		input.SetCommandView(view)
		var failures []activity.Event
		if strings.HasPrefix(mode, "failure") {
			failures = []activity.Event{{Agent: activity.Codex, Kind: activity.ToolFailed, Summary: "command exited 7", Detailed: true, Command: "build", Error: "build failed", Output: strings.Repeat("long code output\n", 120) + "last diagnostic"}}
			input.SetFailures(failures, 1)
		}
		original, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), term.GetTermios)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		limit := 1024
		if mode == "overflow" {
			limit = 4
		}
		var line string
		if mode == "failure-command" {
			err = view.FailureDetails(ctx, input, failures, uint64(len(failures)))
		} else if strings.HasPrefix(mode, "model") {
			models := modelChoices{harness: "codex", checked: true, models: []account.Model{{ID: "gpt-6-astra"}, {ID: "gpt-6-sol"}, {ID: "gpt-5.6-sol"}}}
			line, err = input.ReadChoice(ctx, limit, models.suggestions)
		} else {
			line, err = input.ReadCommand(ctx, limit, CommandSuggestions)
		}
		restored, restoreErr := unix.IoctlGetTermios(int(os.Stdin.Fd()), term.GetTermios)
		// BSD sets PENDIN when restoring canonical mode with buffered input.
		original.Lflag &^= unix.PENDIN
		if restored != nil {
			restored.Lflag &^= unix.PENDIN
		}
		if restoreErr != nil || *original != *restored {
			t.Fatalf("terminal mode not restored: before=%+v after=%+v", original, restored)
		}
		switch mode {
		case "complete":
			if err != nil || line != "/configuration" {
				t.Fatal(line, err)
			}
		case "choices":
			if err != nil || line != "/set mode team" {
				t.Fatal(line, err)
			}
		case "model":
			if err != nil || line != "gpt-5.6-sol" {
				t.Fatal(line, err)
			}
		case "model-number":
			if err != nil || line != "2" {
				t.Fatal(line, err)
			}
		case "exact":
			if err != nil || line != "/config" {
				t.Fatal(line, err)
			}
		case "paste":
			if err != nil || line != "explain this\n/quit" {
				t.Fatal(line, err)
			}
		case "unicode":
			if err != nil || line != "hé!" {
				t.Fatal(line, err)
			}
		case "wide", "input-resize":
			if err != nil || line != strings.Repeat("x", 70) {
				t.Fatal(line, err)
			}
		case "overflow":
			if !errors.Is(err, term.ErrInputTooLong) {
				t.Fatal("overflow accepted", line, err)
			}
		case "eof":
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
		case "cancel":
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		case "failure", "failure-resize":
			if err != nil || line != "next" {
				t.Fatal(line, err)
			}
		case "failure-command":
			if err != nil {
				t.Fatal(err)
			}
		}
		fmt.Println("EDITOR-OK")
		return
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 needed for PTY integration")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const script = `
import os,pty,select,subprocess,sys,time,fcntl,termios,struct
# Enter reaches the raw-mode editor as CR, or as LF after cooked-mode translation; both submit.
cases={'complete':b'/conf\t\r','choices':b'/set mode \x1b[B\t\r','exact':b'/config\r','paste':b'\x1b[200~explain this\n/quit\x1b[201~\r','unicode':'héx'.encode()+b'\x7f!\r','wide':b'x'*70+b'\r','overflow':b'abcde\r','eof':b'\x04','cancel':b'','failure':b'next\x1b[<0;3;20M\x1b[<0;3;4M\x1b[6~\x1b[F\x1b[<0;3;10M\x1b[<0;3;1M\r','failure-command':b'o\x1b[F\n','model':b'sol\x1b[B\t\r','model-number':b'2\r'}
cases['failure-resize']=b'next\x1b[<0;3;20M'
cases['input-resize']=b'x'*70
for mode,keys in cases.items():
 master,slave=pty.openpty()
 resized=False;resize_sent=False;submitted=False
 if mode=='failure-resize':fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,80,0,0))
 if mode=='input-resize':fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,40,0,0))
 env=dict(os.environ,MULTIHARNESS_EDITOR_TEST=mode,TERM='xterm-256color',CI='')
 process=subprocess.Popen([sys.argv[1],'-test.run=^TestCommandEditorPTY$','-test.v'],stdin=slave,stdout=slave,stderr=slave,env=env)
 os.close(slave);output=b'';sent=False;deadline=time.monotonic()+8
 try:
  while time.monotonic()<deadline:
   if select.select([master],[],[],.1)[0]:
    try:data=os.read(master,65536)
    except OSError:break
    if not data:break
    output+=data
    if not sent and b'\x1b[?2004h' in output:
     os.write(master,keys);sent=True
    if mode=='failure-resize' and not resized and b'Enter/Esc: close' in output:
     fcntl.ioctl(master,termios.TIOCSWINSZ,struct.pack('HHHH',12,40,0,0));resized=True
    if mode=='failure-resize' and resized and not resize_sent and b'\x1b[12;1H' in output:
     os.write(master,b'o\x1b[F\x1b[<0;3;1M\r');resize_sent=True
    if mode=='input-resize' and not resized and b'x'*35+b'\r\r\n    '+b'x'*35 in output:
     fcntl.ioctl(master,termios.TIOCSWINSZ,struct.pack('HHHH',24,120,0,0));resized=True;output=b''
    if mode=='input-resize' and resized and not resize_sent and b'x'*70 in output:
     fcntl.ioctl(master,termios.TIOCSWINSZ,struct.pack('HHHH',24,40,0,0));resize_sent=True;output=b''
    if mode=='input-resize' and resize_sent and not submitted and b'x'*35+b'\r\r\n    '+b'x'*35 in output:
     os.write(master,b'\r');submitted=True
   elif process.poll() is not None:break
  process.wait(timeout=1)
  assert process.returncode==0 and b'EDITOR-OK' in output,(mode,output.decode(errors='replace'))
  if mode=='complete':
   assert b'/configuration' in output and b'/config' in output
   assert b'\r\x1b[J  \x1b[1;38;5;117m' in output, output
   assert b'\r\n    \x1b[1;38;5;117m> /config' in output, output
  if mode=='choices':assert b'/set mode direct' in output and b'/set mode team' in output
  if mode=='model':assert b'sol\r\r\n    > gpt-6-sol\r\r\n      gpt-5.6-sol\r\r\n  ' in output and b'> gpt-5.6-sol' in output,output
  if mode=='model-number':assert b'gpt-' not in output,output
  if mode=='wide':
   assert b'\r\x1b[J  \xe2\x9d\xaf '+b'x'*70+b'\r\x1b[74C' in output,output
  if mode=='input-resize':assert resized and resize_sent,output
  if mode.startswith('failure'):
   assert b'build failed' in output and b'\x1b[?1049h' in output and b'\x1b[?1049l' in output,output
   assert b'last diagnostic' in output and b'Output lines' in output,output
   first=output.split(b'\x1b[H\x1b[2J',1)[1].split(b'\x1b[H\x1b[2J',1)[0]
   assert b'last diagnostic' not in first and b'long code output' not in first and b'REPORTED ERROR' in first and b'Enter/Esc: close' in first,first
   if mode=='failure-resize':assert resize_sent,output
 finally:
  if process.poll() is None:process.kill();process.wait()
  os.close(master)
`
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, python, "-c", script, binary).CombinedOutput()
	if err != nil {
		t.Fatalf("PTY integration: %v\n%s", err, output)
	}
}
