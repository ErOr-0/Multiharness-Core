//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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
		p.AgentActivity(activity.Event{Agent: activity.Codex, Kind: activity.ToolFailed, Summary: "command exited 7", Text: "build failed"})
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
		if err != nil || *paused != *original {
			t.Fatal("approval prompt would not receive canonical input")
		}
		resume()
		p.stop()
		restored, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), secretGetTermios)
		if err != nil || *restored != *original {
			t.Fatal("terminal mode not restored after task")
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
os.close(slave);output=b'';opened=False;closed=False;opened_at=0;deadline=time.monotonic()+8
try:
 while time.monotonic()<deadline:
  if select.select([master],[],[],.1)[0]:
   try:data=os.read(master,65536)
   except OSError:break
   if not data:break
   output+=data
   if not opened and b'\x1b[?1000h' in output:
    os.write(master,b'\x1b[<0;3;20M');opened=True;opened_at=time.monotonic()
  elif process.poll() is not None:break
  if opened and not closed and b'\x1b[?1049h' in output and time.monotonic()-opened_at>.2:
   os.write(master,b'\x1b[<0;3;2M');closed=True
 process.wait(timeout=1)
 assert process.returncode==0 and b'LIVE-OK' in output and opened and closed,(process.returncode,output.decode(errors='replace'))
 assert b'build failed' in output and b'\x1b[?1049l' in output,output
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
