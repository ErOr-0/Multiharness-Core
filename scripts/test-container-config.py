"""Exercise the shipped terminal menu and persistence across real process restarts."""
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
os.environ["NO_COLOR"] = "1"


def terminal(lines, initial="❯ "):
    pid, fd = pty.fork()
    if pid == 0:
        os.execv("/usr/local/bin/magent-container", ["magent-container"])
    output = b""
    sent = False
    try:
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            if not sent and initial.encode() in output:
                os.write(fd, ("\n".join(lines) + "\n").encode())
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
            raise AssertionError("Configuration timed out: " + output.decode(errors="replace"))
        _, status = os.waitpid(pid, 0)
        assert sent and os.waitstatus_to_exitcode(status) == 0, output
        return output.decode(errors="replace")
    finally:
        os.close(fd)
        try:
            os.kill(pid, signal.SIGKILL)
            os.waitpid(pid, 0)
        except (ProcessLookupError, ChildProcessError):
            pass


settings = Path("/state/1000/.config/magent/config.json")
terminal(["", "opencode", "", "", "/quit"], initial="Folder >")
out = terminal(["/config", "4", "2", "/config", "2",
                "codex", "fixture-plan", "low",
                "opencode", "fixture/build", "",
                "claude", "fixture-review", "high",
                "n", "n", "n",
                "/config", "4", "/cancel", "/quit"])
for expected in ("Direct - one agent", "Team - separate", "1/9 · planner", "4/9 · implementer", "7/9 · reviewer",
                 "4. Mode", "All available controls", "Menu changes save automatically"):
    assert expected in out, (expected, out)
saved = json.loads(settings.read_text())
assert saved["mode"] == "team", saved
for role, harness, model in (("planner", "codex", "fixture-plan"),
                              ("implementer", "opencode", "fixture/build"),
                              ("reviewer", "claude", "fixture-review")):
    assert saved[role]["harness"] == harness and saved[role]["model"] == model, saved
assert saved["planner"]["reasoning"] == "low" and saved["reviewer"]["reasoning"] == "high", saved
before = settings.read_bytes()
out = terminal(["/config", "/cancel", "/config", "2", "opencode", "/cancel", "/quit"])
assert "Team - separate" in out and settings.read_bytes() == before, out
out = terminal(["/config", "4", "1", "/quit"])
assert "Mode saved: direct" in out, out
saved_direct = json.loads(settings.read_text())
assert saved_direct["mode"] == "direct", saved_direct
for role in ("planner", "implementer", "reviewer"):
    assert saved_direct[role] == saved[role], (saved_direct, saved)
out = terminal(["/config", "/cancel", "/quit"])
assert "Direct - one agent" in out and "Agent - one CLI handles the task" in out, out
print("PASS: packaged config discovers controls, persists independent roles, cancels edits and restores both modes")
