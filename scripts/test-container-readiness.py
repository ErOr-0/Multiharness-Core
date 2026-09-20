"""Real bundled CLIs, fresh state: incomplete mixed workflows never start a task."""
import errno
import fcntl
import json
import os
from pathlib import Path
import pty
import re
import struct
import termios
import select
import signal
import subprocess
import time

assert os.getuid() == 0
for directory in ("/workspace", "/state"):
    subprocess.run(["mountpoint", "-q", directory], check=True)
os.setgroups([])
os.setgid(1000)
os.setuid(1000)
settings = Path("/state/1000/.config/magent/config.json")
settings.parent.mkdir(parents=True)
settings.write_text(json.dumps({"version": 1, "mode": "team", "color": "auto",
    "planner": {"harness": "codex"},
    "implementer": {"harness": "opencode", "model": "missing-provider/missing-model"},
    "reviewer": {"harness": "claude"}, "fallback": {"mode": "disabled"}}))
os.environ.pop("NO_COLOR", None)
os.environ.pop("COLORTERM", None)
os.environ.update(TERM="xterm-256color", CI="")
pid, fd = pty.fork()
if pid == 0:
    fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 80, 0, 0))
    os.execv("/usr/local/bin/magent-container", ["magent-container"])
output = b""
sent = False
try:
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        if not sent and b"Folder >" in output:
            os.write(fd, b"\n/configuration\ncreate a file named MUST-NOT-EXIST\n/quit\n")
            sent = True
        if select.select([fd], [], [], 0.1)[0]:
            try:
                data = os.read(fd, 65536)
            except OSError as error:
                if error.errno == errno.EIO:
                    break
                raise
            if not data:
                break
            output += data
    else:
        raise AssertionError("Readiness check timed out")
    _, status = os.waitpid(pid, 0)
    text = output.decode(errors="replace")
    assert "\x1b[1;38;5;117mPlanner" in text and "\x1b[38;5;221m! NEEDS SETUP" in text, text
    text = re.sub(r"\x1b\[[0-9;]*m", "", text)
    assert os.waitstatus_to_exitcode(status) == 0, text
    for role, agent in (("planner", "codex"), ("implementer", "opencode"), ("reviewer", "claude")):
        name = {"codex": "Codex", "opencode": "OpenCode", "claude": "Claude"}[agent]
        assert f"{role.title():16}{name}" in text, text
    assert text.count("! NEEDS SETUP") >= 3, text
    for agent in ("codex", "opencode", "claude"):
        assert f"/login {agent}" in text, text
    assert "Tasks are blocked" in text, text
    assert "Workflow failed" not in text and "── Progress" not in text, text
    assert not Path("/workspace/MUST-NOT-EXIST").exists()
    print("PASS: real signed-out Codex/Claude and unavailable OpenCode provider block a mixed workflow before task execution")
finally:
    os.close(fd)
    try:
        os.kill(pid, signal.SIGKILL)
        os.waitpid(pid, 0)
    except (ProcessLookupError, ChildProcessError):
        pass
