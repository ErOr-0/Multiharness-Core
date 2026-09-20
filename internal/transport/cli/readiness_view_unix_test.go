//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"multiharness-core/internal/config"
)

func TestReadinessTerminalColors(t *testing.T) {
	if mode := os.Getenv("MULTIHARNESS_READINESS_PTY"); mode != "" {
		view := &interactiveView{writer: os.Stdout}
		view.configure(config.Defaults(), os.LookupEnv)
		readinessPreview(t, view, mode == "ready")
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
import fcntl,os,pty,select,struct,subprocess,sys,termios,time
for mode in ('ready','missing','no-color','dumb'):
 master,slave=pty.openpty()
 fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,80,0,0))
 env=dict(os.environ,MULTIHARNESS_READINESS_PTY=mode,TERM='xterm-256color',CI='')
 env.pop('NO_COLOR',None)
 env.pop('COLORTERM',None)
 if mode=='no-color':env['NO_COLOR']='1'
 if mode=='dumb':env['TERM']='dumb'
 process=subprocess.Popen([sys.argv[1],'-test.run=^TestReadinessTerminalColors$'],stdin=slave,stdout=slave,stderr=slave,env=env)
 os.close(slave);output=b'';deadline=time.monotonic()+5
 try:
  while time.monotonic()<deadline:
   if select.select([master],[],[],.1)[0]:
    try:data=os.read(master,65536)
    except OSError:break
    if not data:break
    output+=data
   elif process.poll() is not None:break
  process.wait(timeout=1)
  assert process.returncode==0 and b'WORKFLOW READINESS' in output,output
  if mode in ('no-color','dumb'):assert b'\x1b[' not in output,output
  else:
   assert b'\x1b[1;38;5;117mPlanner' in output and b'\x1b[38;5;114m' in output,output
   if mode=='missing':assert b'\x1b[38;5;221m! NEEDS SETUP' in output,output
 finally:
  if process.poll() is None:process.kill();process.wait()
  os.close(master)
`
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, python, "-c", script, binary).CombinedOutput(); err != nil {
		t.Fatalf("PTY readiness: %v\n%s", err, output)
	}
}
