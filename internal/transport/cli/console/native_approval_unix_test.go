//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"multiharness-core/internal/adapter/agent/native"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/approval"
	"multiharness-core/internal/transport/cli/progress"
	"multiharness-core/internal/workflow"
)

// This is a real subprocess peer, intentionally blocked until the UI sends the
// native decision. It writes only the disposable test workspace after approval.
func TestNativeApprovalProcessPeer(t *testing.T) {
	if os.Getenv("MULTIHARNESS_NATIVE_APPROVAL_PEER") != "1" {
		return
	}
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	read := func() map[string]any {
		var m map[string]any
		if dec.Decode(&m) != nil {
			os.Exit(21)
		}
		return m
	}
	write := func(m any) {
		if enc.Encode(m) != nil {
			os.Exit(22)
		}
	}
	for _, step := range []struct {
		method string
		result any
	}{{"initialize", map[string]any{}}, {"thread/start", map[string]any{"thread": map[string]any{"id": "thread"}}}, {"turn/start", map[string]any{"turn": map[string]any{"id": "turn"}}}} {
		m := read()
		if m["method"] != step.method {
			os.Exit(23)
		}
		write(map[string]any{"id": m["id"], "result": step.result})
		if step.method == "initialize" {
			if read()["method"] != "initialized" {
				os.Exit(24)
			}
		}
	}
	write(map[string]any{"id": 42, "method": "item/fileChange/requestApproval", "params": map[string]any{"threadId": "thread", "turnId": "turn", "itemId": "write", "reason": "Write approved.txt in the test workspace"}})
	m := read()
	result, ok := m["result"].(map[string]any)
	if !ok || m["id"] != float64(42) || result["decision"] != "acceptForSession" {
		os.Exit(25)
	}
	if os.WriteFile("approved.txt", []byte("native decision received"), 0600) != nil {
		os.Exit(26)
	}
	write(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "thread", "turnId": "turn", "item": map[string]any{"type": "agentMessage", "text": "continued same process"}}})
	write(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread", "turn": map[string]any{"id": "turn", "status": "completed"}}})
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

type nativePTYRunner struct{}

func (nativePTYRunner) Run(ctx context.Context, c process.Command) (process.Result, error) {
	c.Name, _ = os.Executable()
	c.Args = []string{"-test.run=^TestNativeApprovalProcessPeer$"}
	c.EnvOverrides = map[string]string{"MULTIHARNESS_NATIVE_APPROVAL_PEER": "1"}
	return process.NewOSRunner().Run(ctx, c)
}

func TestNativeApprovalRoundTripPTY(t *testing.T) {
	if os.Getenv("MULTIHARNESS_NATIVE_APPROVAL_PTY") == "1" {
		p := progress.New(os.Stdout)
		p.Configure(config.Defaults(), os.LookupEnv)
		p.Control = &Reader{file: os.Stdin, output: os.Stdout}
		ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
		defer cancel()
		p.Publish(workflow.Event{Type: workflow.EventTypeStageStarted, Stage: contract.WorkflowStageImplementation})
		p.Start(ctx)
		defer p.Stop()
		a := NewNativeApprover(os.Stdin, os.Stdout)
		if a == nil {
			t.Fatal("missing interactive approver")
		}
		dir := t.TempDir()
		r, err := native.Codex(ctx, nativePTYRunner{}, native.Config{Executable: "fixture", CanWrite: true, Sandbox: "workspace-write", Approver: approval.WithProgressNativeApproval(a, p)}, native.Request{Directory: dir, Prompt: "write the test file"})
		if err != nil || r.Text != "continued same process" {
			t.Fatal(r, err)
		}
		if data, err := os.ReadFile(filepath.Join(dir, "approved.txt")); err != nil || string(data) != "native decision received" {
			t.Fatal(string(data), err)
		}
		p.Stop()
		fmt.Println("NATIVE-APPROVAL-OK")
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
master,slave=pty.openpty()
env=dict(os.environ,MULTIHARNESS_NATIVE_APPROVAL_PTY='1',TERM='xterm-256color',CI='')
p=subprocess.Popen([sys.argv[1],'-test.run=^TestNativeApprovalRoundTripPTY$'],stdin=slave,stdout=slave,stderr=slave,env=env)
os.close(slave);output=b'';answered=False;deadline=time.monotonic()+9
try:
 while time.monotonic()<deadline:
  if select.select([master],[],[],.1)[0]:
   try:data=os.read(master,65536)
   except OSError:break
   if not data:break
   output+=data
   if not answered and b'Choose a number (Enter denies):' in output:
    fcntl.ioctl(master,termios.TIOCSWINSZ,struct.pack('HHHH',24,45,0,0))
    os.write(master,b'2\n');answered=True
  elif p.poll() is not None:break
 p.wait(timeout=2)
 assert p.returncode==0 and answered and b'NATIVE-APPROVAL-OK' in output,(p.returncode,output.decode(errors='replace'))
 assert output.count(b'Codex needs permission:')==1,output
finally:
 if p.poll() is None:p.kill();p.wait()
 os.close(master)
`
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, python, "-c", script, binary).CombinedOutput(); err != nil {
		t.Fatalf("native approval PTY: %v\n%s", err, out)
	}
}
