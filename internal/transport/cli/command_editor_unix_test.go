//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCommandEditorPTY(t *testing.T) {
	if mode := os.Getenv("MULTIHARNESS_EDITOR_TEST"); mode != "" {
		input := &terminalConfirmation{file: os.Stdin, output: os.Stdout}
		input.setCommandView(&interactiveView{writer: os.Stdout, color: mode == "complete", width: 77})
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
		line, err := input.ReadCommand(ctx, limit)
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
import os,pty,select,subprocess,sys,time
cases={'complete':b'/conf\t\n','choices':b'/set mode \x1b[B\t\n','exact':b'/config\n','paste':b'\x1b[200~explain this\n/quit\x1b[201~\n','unicode':'héx'.encode()+b'\x7f!\n','overflow':b'abcde\n','eof':b'\x04','cancel':b''}
for mode,keys in cases.items():
 master,slave=pty.openpty()
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
   elif process.poll() is not None:break
  process.wait(timeout=1)
  assert process.returncode==0 and b'EDITOR-OK' in output,(mode,output.decode(errors='replace'))
  if mode=='complete':
   assert b'/configuration' in output and b'/config' in output
   assert b'\r\x1b[J  \x1b[1;38;5;117m' in output, output
   assert b'\r\n    \x1b[1;38;5;117m> /config' in output, output
  if mode=='choices':assert b'/set mode direct' in output and b'/set mode team' in output
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
