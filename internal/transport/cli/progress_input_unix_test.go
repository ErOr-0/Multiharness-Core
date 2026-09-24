//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func TestLiveFailureDisclosurePTY(t *testing.T) {
	if os.Getenv("MULTIHARNESS_LIVE_DISCLOSURE_TEST") == "1" {
		original, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), secretGetTermios)
		if err != nil {
			t.Fatal(err)
		}
		terminal := &terminalConfirmation{file: os.Stdin, output: os.Stdout}
		p := newPresentation(io.Discard, os.Stdout).progress
		p.view.size = func() (int, bool) { return 90, true }
		p.configure(config.Defaults(), os.LookupEnv)
		p.control = terminal
		p.Publish(workflow.Event{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageImplementation})
		ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
		defer cancel()
		p.start(ctx)
		defer p.stop()
		beforeFailure, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), secretGetTermios)
		if err != nil || *beforeFailure != *original {
			t.Fatal("terminal input changed before a failure was reported")
		}
		p.AgentActivity(activity.Event{Agent: activity.Codex, Kind: activity.ToolFailed, Summary: "command exited 7", Text: "build failed\n" + strings.Repeat("code output\n", 100) + "last diagnostic"})
		p.tick(time.Now())
		waitModal := func(want bool) {
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				p.mu.Lock()
				got := p.view.modal
				p.mu.Unlock()
				if got == want {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatalf("failure view did not reach modal=%v", want)
		}
		waitModal(true)
		waitModal(false)
		resume, err := p.PauseProgress()
		if err != nil {
			t.Fatal(err)
		}
		paused, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), secretGetTermios)
		if err != nil || !sameRestoredTerminal(paused, original) {
			t.Fatalf("terminal not restored before approval: got %#v, want %#v, error %v", paused, original, err)
		}
		confirmation := ValidationConfirmation{Input: terminal, Output: os.Stdout}
		action := store.ValidationAction{Executable: "go", Args: []string{"test", "./..."}, Reason: "Build cache needs write access"}
		if yes, err := confirmation.ConfirmValidation(ctx, "/workspace", action); err != nil || !yes {
			t.Fatalf("validation yes: %v, %v", yes, err)
		}
		if yes, err := confirmation.ConfirmValidation(ctx, "/workspace", action); err != nil || yes {
			t.Fatalf("validation no: %v, %v", yes, err)
		}
		resume()
		p.stop()
		restored, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), secretGetTermios)
		if err != nil || !sameRestoredTerminal(restored, original) {
			t.Fatalf("terminal not restored after task: got %#v, want %#v, error %v", restored, original, err)
		}
		fmt.Println("LIVE-OK")
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
master,slave=pty.openpty()
env=dict(os.environ,MULTIHARNESS_LIVE_DISCLOSURE_TEST='1',TERM='xterm-256color',CI='')
process=subprocess.Popen([sys.argv[1],'-test.run=^TestLiveFailureDisclosurePTY$','-test.v'],stdin=slave,stdout=slave,stderr=slave,env=env)
os.close(slave);output=b'';opened=False;closed=False;opened_at=0;answers=0;deadline=time.monotonic()+8
try:
 while time.monotonic()<deadline:
  if select.select([master],[],[],.1)[0]:
   try:data=os.read(master,65536)
   except OSError:break
   if not data:break
   output+=data
   prompts=output.count(b'Allow this command? [yes/No]:')
   if prompts>answers:
    os.write(master,b'yes\n' if answers==0 else b'no\n');answers+=1
   if not opened and b'\x1b[?1000h' in output:
    os.write(master,b'\x1b[<0;3;20M');opened=True;opened_at=time.monotonic()
  elif process.poll() is not None:break
  if opened and not closed and b'\x1b[?1049h' in output and time.monotonic()-opened_at>.2:
   os.write(master,b'\x1b[6~\x1b[F\x1b[<0;3;10M\x1b[<0;3;1M');closed=True
 process.wait(timeout=1)
 assert process.returncode==0 and b'LIVE-OK' in output and opened and closed and answers==2,(process.returncode,output.decode(errors='replace'))
 assert b'build failed' in output and b'\x1b[?1049l' in output,output
 assert b'last diagnostic' in output and b'Output lines' in output,output
finally:
 if process.poll() is None:process.kill();process.wait()
 os.close(master)
`
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, python, "-c", script, binary).CombinedOutput()
	if err != nil || bytes.Contains(output, []byte("Traceback")) {
		t.Fatalf("live disclosure PTY: %v\n%s", err, output)
	}
}

// Darwin sets PENDIN when switching back to ICANON so queued input is
// reprocessed. It is transient kernel state, not a changed terminal setting:
// https://github.com/apple-oss-distributions/xnu/blob/main/bsd/kern/tty.c
// Keep every other flag, control character and speed in the comparison.
func sameRestoredTerminal(got, want *unix.Termios) bool {
	if got == nil || want == nil {
		return false
	}
	a, b := *got, *want
	if runtime.GOOS == "darwin" {
		a.Lflag &^= unix.PENDIN
		b.Lflag &^= unix.PENDIN
	}
	return a == b
}

func TestRestoredTerminalComparisonPreservesInputChecks(t *testing.T) {
	original := unix.Termios{}
	original.Lflag = unix.ICANON | unix.ECHO | unix.ISIG | unix.IEXTEN
	for _, flag := range []uint64{uint64(unix.ICANON), uint64(unix.ECHO), uint64(unix.ISIG), uint64(unix.IEXTEN)} {
		changed := original
		// Termios flag widths differ across Unix platforms.
		if flag == uint64(unix.ICANON) {
			changed.Lflag &^= unix.ICANON
		}
		if flag == uint64(unix.ECHO) {
			changed.Lflag &^= unix.ECHO
		}
		if flag == uint64(unix.ISIG) {
			changed.Lflag &^= unix.ISIG
		}
		if flag == uint64(unix.IEXTEN) {
			changed.Lflag &^= unix.IEXTEN
		}
		if sameRestoredTerminal(&changed, &original) {
			t.Fatalf("ignored input flag %x", flag)
		}
	}
	changed := original
	changed.Cc[unix.VMIN]++
	if sameRestoredTerminal(&changed, &original) {
		t.Fatal("ignored changed control character")
	}
	changed = original
	changed.Lflag |= unix.PENDIN
	if sameRestoredTerminal(&changed, &original) != (runtime.GOOS == "darwin") {
		t.Fatal("incorrect platform handling of PENDIN")
	}
}
