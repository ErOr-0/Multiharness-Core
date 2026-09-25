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
	"multiharness-core/internal/adapter/agent/activity"
)

func TestCommandEditorPTY(t *testing.T) {
	if mode := os.Getenv("MULTIHARNESS_EDITOR_TEST"); mode != "" {
		input := &terminalConfirmation{file: os.Stdin, output: os.Stdout}
		input.setCommandView(&interactiveView{writer: os.Stdout, color: mode == "complete", width: 77})
		if strings.HasPrefix(mode, "failure") {
			input.setFailures([]activity.Event{{Agent: activity.Codex, Kind: activity.ToolFailed, Summary: "command exited 7", Detailed: true, Command: "build", Error: "build failed", Output: strings.Repeat("long code output\n", 120) + "last diagnostic"}}, 1)
		}
		original, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), secretGetTermios)
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
			err = input.commandView.failureDetails(ctx, input, input.failures, input.failureCount)
		} else {
			line, err = input.ReadCommand(ctx, limit)
		}
		restored, restoreErr := unix.IoctlGetTermios(int(os.Stdin.Fd()), secretGetTermios)
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
		case "exact":
			if err != nil || line != "/config" {
				t.Fatal(line, err)
			}
		case "paste":
			if err != nil || line != "explain this /quit" {
				t.Fatal(line, err)
			}
		case "unicode":
			if err != nil || line != "hé!" {
				t.Fatal(line, err)
			}
		case "wide":
			if err != nil || line != strings.Repeat("x", 70) {
				t.Fatal(line, err)
			}
		case "overflow":
			if !errors.Is(err, errInputTooLong) {
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
cases={'complete':b'/conf\t\n','choices':b'/set mode \x1b[B\t\n','exact':b'/config\n','paste':b'\x1b[200~explain this\n/quit\x1b[201~\n','unicode':'héx'.encode()+b'\x7f!\n','wide':b'x'*70+b'\n','overflow':b'abcde\n','eof':b'\x04','cancel':b'','failure':b'next\x1b[<0;3;20M\x1b[<0;3;4M\x1b[6~\x1b[F\x1b[<0;3;10M\x1b[<0;3;1M\n','failure-command':b'o\x1b[F\n'}
cases['failure-resize']=b'next\x1b[<0;3;20M'
for mode,keys in cases.items():
 master,slave=pty.openpty()
 resized=False;resize_sent=False
 if mode=='failure-resize':fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,80,0,0))
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
     os.write(master,b'o\x1b[F\x1b[<0;3;1M\n');resize_sent=True
   elif process.poll() is not None:break
  process.wait(timeout=1)
  assert process.returncode==0 and b'EDITOR-OK' in output,(mode,output.decode(errors='replace'))
  if mode=='complete':
   assert b'/configuration' in output and b'/config' in output
   assert b'\r\x1b[J  \x1b[1;38;5;117m' in output, output
   assert b'\r\n    \x1b[1;38;5;117m> /config' in output, output
  if mode=='choices':assert b'/set mode direct' in output and b'/set mode team' in output
  if mode=='wide':
   assert b'\r\x1b[J  \xe2\x9d\xaf '+b'x'*70+b'\r\x1b[4C' in output,output
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
