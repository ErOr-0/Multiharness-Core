//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"multiharness-core/internal/config"
)

func TestTerminalWidthTracksResize(t *testing.T) {
	if os.Getenv("MULTIHARNESS_RESIZE_PTY") != "" {
		view := &interactiveView{writer: os.Stdout}
		view.configure(config.Defaults(), os.LookupEnv)
		fmt.Printf("width-before=%d\n", view.contentWidth())
		var signal [1]byte
		if _, err := os.Stdin.Read(signal[:]); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("width-after=%d\n", view.contentWidth())
		return
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 needed for PTY resize integration")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const script = `
import fcntl,os,pty,select,struct,subprocess,sys,termios,time
master,slave=pty.openpty()
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,40,0,0))
env=dict(os.environ,MULTIHARNESS_RESIZE_PTY='1')
process=subprocess.Popen([sys.argv[1],'-test.run=^TestTerminalWidthTracksResize$'],stdin=slave,stdout=slave,stderr=slave,env=env)
os.close(slave)
output=b''
deadline=time.monotonic()+5
try:
 while b'width-before=37' not in output and time.monotonic()<deadline:
  if select.select([master],[],[],.1)[0]: output+=os.read(master,65536)
 assert b'width-before=37' in output,output
 fcntl.ioctl(master,termios.TIOCSWINSZ,struct.pack('HHHH',24,100,0,0))
 os.write(master,b'x\n')
 while b'width-after=96' not in output and time.monotonic()<deadline:
  if select.select([master],[],[],.1)[0]: output+=os.read(master,65536)
 process.wait(timeout=1)
 assert process.returncode==0 and b'width-after=96' in output,output
finally:
 if process.poll() is None:process.kill();process.wait()
 os.close(master)
`
	output, err := exec.Command(python, "-c", script, binary).CombinedOutput()
	if err != nil {
		t.Fatalf("PTY resize: %v\n%s", err, output)
	}
}
