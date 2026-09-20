"""Real bundled CLIs, fresh state: incomplete mixed workflows never start a task."""
import errno
import json
import os
from pathlib import Path
import pty
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
settings.write_text(json.dumps({"version": 1, "mode": "team", "color": "never",
    "planner": {"harness": "codex"},
    "implementer": {"harness": "opencode", "model": "missing-provider/missing-model"},
    "reviewer": {"harness": "claude"}, "fallback": {"mode": "disabled"}}))
os.environ.update(TERM="dumb", NO_COLOR="1")
pid, fd = pty.fork()
if pid == 0:
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
    assert os.waitstatus_to_exitcode(status) == 0, text
    for role, agent in (("planner", "codex"), ("implementer", "opencode"), ("reviewer", "claude")):
        assert f"[NEEDS SETUP] {role} · {agent}" in text, text
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
