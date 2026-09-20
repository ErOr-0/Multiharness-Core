"""Opt-in authenticated Codex permission changes through the real terminal."""
import argparse
import json
import os
from pathlib import Path
import tempfile

from terminal_driver import Terminal

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--binary", required=True)
parser.add_argument("--config", required=True)
args = parser.parse_args()
assert not os.environ.get("CI"), "Live account checks must be selected locally"
original = json.loads(Path(args.config).read_text())
assert original["implementer"]["harness"] == "codex"

# Keep the outside target out of /tmp, which Codex may allow in workspace mode.
with tempfile.TemporaryDirectory(prefix="magent-codex-permissions-", dir=Path.home()) as temp:
    root = Path(temp)
    project = root / "project"
    project.mkdir()
    settings = root / "config/magent/config.json"
    settings.parent.mkdir(parents=True)
    record = root / "native-calls.jsonl"
    wrapper = root / "codex-recorder"
    native = original["implementer"].get("executable", "codex")
    wrapper.write_text("#!/usr/bin/python3\nimport json,pathlib,subprocess,sys\n"
        f"r=subprocess.run([{native!r},*sys.argv[1:]],input=sys.stdin.buffer.read(),capture_output=True)\n"
        "if (sys.argv[1:] in (['login','status'],['auth','status','--json']) or sys.argv[1:2]==['models']): sys.stdout.buffer.write(r.stdout);sys.stderr.buffer.write(r.stderr);sys.exit(r.returncode)\n"
        "events=[json.loads(line) for line in r.stdout.decode().splitlines()]\n"
        f"with pathlib.Path({str(record)!r}).open('a') as f: f.write(json.dumps({{'args':sys.argv[1:],'events':events}})+'\\n')\n"
        "sys.stdout.buffer.write(r.stdout);sys.stderr.buffer.write(r.stderr);sys.exit(r.returncode)\n")
    wrapper.chmod(0o755)
    original.update(mode="direct", working_dir=str(project), color="never", progress="off", timeout="3m")
    original["implementer"].update(executable=str(wrapper), timeout="3m", sandbox="workspace-write", permission_policy="reject_on_prompt")
    settings.write_text(json.dumps(original))
    env = {k: v for k, v in os.environ.items() if not k.startswith(("MAGENT_", "MULTIHARNESS_"))}
    env.update(XDG_CONFIG_HOME=str(settings.parent.parent), MULTIHARNESS_CONFIG=str(settings),
               MULTIHARNESS_INSTALL_MODE="disabled", MULTIHARNESS_RUNTIME_CHECK="0", TERM="dumb", NO_COLOR="1")
    terminal = Terminal(args.binary, project, env)
    try:
        terminal.until()
        terminal.send("/permissions")
        menu = terminal.until(b"Choose 1 to 3: ")
        assert "CODEX PERMISSIONS" in menu and "Full access" in menu, menu
        terminal.command("/cancel")
        cases = [
            ("read-only", "read-only", project / "first-denied.txt", False),
            ("workspace", "workspace-write", project / "allowed.txt", True),
            ("full", "danger-full-access", root / "outside-allowed.txt", True),
            ("read-only", "read-only", project / "revoked.txt", False),
        ]
        for mode, sandbox, path, allowed in cases:
            terminal.command("/permissions " + mode)
            assert json.loads(settings.read_text())["implementer"]["sandbox"] == sandbox
            result = terminal.command(f"The sandbox setting has been changed for this turn. Try writing the exact text permission-test to {path} using a shell command. If current permissions prevent it, stop immediately and explain; do not attempt a workaround. Do not modify any other files.")
            assert "RESPONDED" in result, result
            assert path.exists() == allowed, (mode, result)
            if allowed:
                assert path.read_text().strip() == "permission-test", result
        calls = [json.loads(line) for line in record.read_text().splitlines()]
        assert len(calls) == len(cases), len(calls)
        sessions = [{e["thread_id"] for e in call["events"] if e.get("thread_id")} for call in calls]
        assert len(sessions[0]) == 1 and all(s == sessions[0] for s in sessions), sessions
        for call, (_, sandbox, _, _) in zip(calls, cases):
            assert f'sandbox_mode="{sandbox}"' in call["args"]
            assert 'approval_policy="never"' in call["args"]
        for call in calls[1:]:
            assert call["args"][:2] == ["exec", "resume"] and call["args"][-2] in sessions[0]
        assert not json.loads(settings.read_text()).get("session_id")
        print("PASS: authenticated Codex terminal kept the same session across read-only, workspace-write, full access and revocation; actual inside/outside files and saved/native settings checked")
    finally:
        terminal.close()
