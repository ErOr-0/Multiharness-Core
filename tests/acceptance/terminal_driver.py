"""Small Linux PTY driver for native terminal acceptance tests."""
import errno
import os
import pty
import select
import signal
import time


class Terminal:
    prompt = "❯ ".encode()

    def __init__(self, binary, cwd, env):
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            os.chdir(cwd)
            os.execve(binary, [binary], env)
        self.output = b""
        self.cursor = 0

    def until(self, marker=None, timeout=180):
        marker = marker or self.prompt
        start = self.cursor
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            at = self.output.find(marker, self.cursor)
            if at >= 0:
                self.cursor = at + len(marker)
                return self.output[start:self.cursor].decode(errors="replace")
            if select.select([self.fd], [], [], 0.2)[0]:
                try:
                    data = os.read(self.fd, 65536)
                except OSError as error:
                    if error.errno == errno.EIO:
                        break
                    raise
                if not data:
                    break
                self.output += data
        raise AssertionError("Expected terminal output missing: " + self.output.decode(errors="replace"))

    def send(self, text):
        os.write(self.fd, (text + "\n").encode())

    def command(self, text):
        self.send(text)
        return self.until()

    def close(self):
        os.close(self.fd)
        try:
            os.kill(self.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        try:
            os.waitpid(self.pid, 0)
        except ChildProcessError:
            pass
