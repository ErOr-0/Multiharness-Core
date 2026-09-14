#!/usr/bin/env python3
"""Live Linux PTY check: configure the real app and observe the real OpenCode CLI.

Uses the selected account with disposable app settings and public scratch files.
The transparent executable wrapper records argv/events without replacing OpenCode.
"""
import argparse
import errno
import json
import os
from pathlib import Path
import pty
import select
import signal
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--binary", required=True)
parser.add_argument("--config", required=True)
args = parser.parse_args()
assert not os.environ.get("CI"), "Live account tests must be selected locally"
original = json.loads(Path(args.config).read_text())
assert original["implementer"]["harness"] == "opencode"

with tempfile.TemporaryDirectory(prefix="magent-permissions-terminal-") as temp:
    root = Path(temp)
    project = root / "project"
    project.mkdir()
    outside = root / "outside.txt"
    outside.write_text("approved through the terminal")
    later = root / "another-outside.txt"
    later.write_text("this read must need approval again")
    config_home = root / "config"
    (config_home / "magent").mkdir(parents=True)
    native_config = Path(os.environ.get("XDG_CONFIG_HOME", str(Path.home() / ".config"))) / "opencode"
    if native_config.exists():
        (config_home / "opencode").symlink_to(native_config, target_is_directory=True)
    record = root / "invocations.jsonl"
    native = original["implementer"].get("executable", "opencode")
    wrapper = root / "observe-opencode"
    wrapper.write_text("#!/usr/bin/python3\n"
        "import json,pathlib,subprocess,sys\n"
        f"r=subprocess.run([{native!r},*sys.argv[1:]],input=sys.stdin.buffer.read(),capture_output=True)\n"
        "events=[json.loads(line) for line in r.stdout.decode().splitlines()]\n"
        f"with pathlib.Path({str(record)!r}).open('a') as f: f.write(json.dumps({{'args':sys.argv[1:],'events':events}})+'\\n')\n"
        "sys.stdout.buffer.write(r.stdout);sys.stderr.buffer.write(r.stderr);sys.exit(r.returncode)\n")
    wrapper.chmod(0o755)
    original.update(mode="direct", working_dir=str(project), timeout="3m", color="never", progress="off")
    original["implementer"].update(executable=str(wrapper), timeout="3m", permission_policy="reject_on_prompt")
    settings = config_home / "magent/config.json"
    settings.write_text(json.dumps(original))
    env = {k: v for k, v in os.environ.items() if not k.startswith(("MULTIHARNESS_", "MAGENT_"))}
    env.update(XDG_CONFIG_HOME=str(config_home), MULTIHARNESS_CONFIG=str(settings),
               MULTIHARNESS_INSTALL_MODE="disabled", MULTIHARNESS_RUNTIME_CHECK="0", TERM="dumb", NO_COLOR="1")
    # Establish an ask boundary in this disposable test only; provider/account
    # settings otherwise remain intact. No permission config is written globally.
    native_overrides = json.loads(env.get("OPENCODE_CONFIG_CONTENT", "{}"))
    native_overrides["permission"] = {"external_directory": "ask"}
    env["OPENCODE_CONFIG_CONTENT"] = json.dumps(native_overrides)
    pid, fd = pty.fork()
    if pid == 0:
        os.chdir(project)
        os.execve(args.binary, [args.binary], env)
    output = b""
    cursor = 0
    prompt = "❯ ".encode()

    def until(marker, timeout=210):
        global output, cursor
        start = cursor
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            at = output.find(marker, cursor)
            if at >= 0:
                cursor = at + len(marker)
                return output[start:cursor].decode(errors="replace")
            if select.select([fd], [], [], 0.2)[0]:
                try:
                    data = os.read(fd, 65536)
                except OSError as error:
                    if error.errno == errno.EIO:
                        break
                    raise
                if not data:
                    break
                output += data
        raise AssertionError("terminal did not reach expected prompt: " + output.decode(errors="replace"))

    def send(line):
        os.write(fd, (line + "\n").encode())

    try:
        until(prompt)
        task = f"Use only the read tool to read {outside}. Do not use bash or other tools to read it. Then write authorized.txt inside this project with the exact file content. If permission is rejected, stop immediately."
        send(task)
        denied = until(prompt)
        assert "needs_input" in denied and "/permissions" in denied, denied
        assert not (project / "authorized.txt").exists()
        send("/permissions")
        menu = until(b"Choose 1 or 2: ")
        assert "including paths outside the project" in menu and "Explicit deny" in menu, menu
        send("2")
        saved = until(prompt)
        assert "permissions saved" in saved, saved
        persisted = json.loads(settings.read_text())
        assert persisted["implementer"]["permission_policy"] == "auto_approve"
        assert not persisted.get("session_id"), "Native session must not be persisted in personal defaults"
        send(task + " I enabled auto-approval here; retry the read now.")
        allowed = until(prompt)
        assert "responded" in allowed, allowed
        assert (project / "authorized.txt").read_text().strip() == outside.read_text()
        send("/permissions native")
        until(prompt)
        assert json.loads(settings.read_text())["implementer"]["permission_policy"] == "reject_on_prompt"
        send(f"Use only the read tool to read {later}. Do not use bash or other tools. If permission is rejected, stop immediately.")
        denied_again = until(prompt)
        assert "needs_input" in denied_again, denied_again
        calls = [json.loads(line) for line in record.read_text().splitlines()]
        assert len(calls) == 3, len(calls)
        assert ["--auto" in call["args"] for call in calls] == [False, True, False]
        sessions = [{event["sessionID"] for event in call["events"] if event.get("sessionID")} for call in calls]
        assert len(sessions[0]) == 1 and sessions[0] == sessions[1] == sessions[2], sessions
        for call in calls[1:]:
            assert call["args"][call["args"].index("--session") + 1] in sessions[0]
        send("/quit")
        _, status = os.waitpid(pid, 0)
        assert os.waitstatus_to_exitcode(status) == 0
        print("PASS: real terminal denied -> /permissions auto -> actual outside-file read -> /permissions native -> denied again; saved settings, native flags and same session verified")
    finally:
        os.close(fd)
        try:
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
