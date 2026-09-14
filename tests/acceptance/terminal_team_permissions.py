"""Opt-in real OpenCode implementation; fixture planning/review and actual file validation."""
import argparse
import json
import os
from pathlib import Path
import tempfile

from terminal_driver import Terminal

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--binary", required=True)
parser.add_argument("--fixture", required=True)
parser.add_argument("--config", required=True)
args = parser.parse_args()
assert not os.environ.get("CI"), "Live account tests must be selected locally"
account_settings = Path(args.config)
account_settings_before = account_settings.read_bytes()
original = json.loads(account_settings_before)
assert original["implementer"]["harness"] == "opencode"
original = {"version": 1, "implementer": original["implementer"], "max_repair_attempts": 0}

with tempfile.TemporaryDirectory(prefix="magent-team-permissions-") as temp:
    root = Path(temp)
    project = root / "project"
    project.mkdir()
    outside = root / "reference.txt"
    outside.write_text("completed")
    config_home = root / "config"
    (config_home / "magent").mkdir(parents=True)
    native_config = Path(os.environ.get("XDG_CONFIG_HOME", str(Path.home() / ".config"))) / "opencode"
    if native_config.exists():
        (config_home / "opencode").symlink_to(native_config, target_is_directory=True)
    settings = config_home / "magent/config.json"
    calls = root / "native.jsonl"
    wrapper = root / "observe-opencode"
    native = original["implementer"].get("executable", "opencode")
    wrapper.write_text("#!/usr/bin/python3\nimport json,pathlib,subprocess,sys\n"
                      f"r=subprocess.run([{native!r},*sys.argv[1:]],input=sys.stdin.buffer.read(),capture_output=True)\n"
                      "events=[json.loads(line) for line in r.stdout.decode().splitlines()]\n"
                      f"with pathlib.Path({str(calls)!r}).open('a') as f: f.write(json.dumps({{'args':sys.argv[1:],'events':events}})+'\\n')\n"
                      "sys.stdout.buffer.write(r.stdout);sys.stderr.buffer.write(r.stderr);sys.exit(r.returncode)\n")
    wrapper.chmod(0o755)
    original.update(mode="team", working_dir=str(project), timeout="3m", color="never", progress="plain")
    original["workspace"] = {"recovery_dir": str(root / "recovery")}
    original["implementer"].update(executable=str(wrapper), timeout="3m", permission_policy="reject_on_prompt")
    for role in ("planner", "reviewer"):
        original[role] = {"harness": "opencode", "executable": args.fixture, "model": "fixture/model"}
    original["validation"] = {"checks": [{"executable": args.fixture, "args": ["verify"]}]}
    settings.write_text(json.dumps(original))
    env = {k:v for k,v in os.environ.items() if not k.startswith(("MULTIHARNESS_", "MAGENT_"))}
    env.update(XDG_CONFIG_HOME=str(config_home), MULTIHARNESS_CONFIG=str(settings), MULTIHARNESS_INSTALL_MODE="disabled", MULTIHARNESS_RUNTIME_CHECK="0",
               BDD_TEAM="1", BDD_RECORD=str(root / "fixtures.jsonl"), TERM="dumb", NO_COLOR="1")
    overrides = json.loads(env.get("OPENCODE_CONFIG_CONTENT", "{}"))
    overrides["permission"] = {"external_directory": "ask"}
    env["OPENCODE_CONFIG_CONTENT"] = json.dumps(overrides)
    terminal = Terminal(args.binary, str(project), env)
    try:
        terminal.until(timeout=30)
        task = f"Use only the read tool to read {outside}. Do not use bash or another tool to read it. Write its exact contents to provider-edit.txt in this project. If permission is denied, stop immediately."
        terminal.send(task)
        denied = terminal.until(timeout=210)
        assert "needs_input" in denied and "permission_denied" in denied and "/permissions" in denied, denied
        assert "malformed structured JSON" not in denied and "Validation: not run" in denied, denied
        assert not (project / "provider-edit.txt").exists()
        first = json.loads(calls.read_text().splitlines()[0])
        denials = [e for e in first["events"] if e.get("part", {}).get("state", {}).get("error") == "The user rejected permission to use this specific tool call."]
        assert any(e["part"]["state"]["input"].get("filePath") == str(outside) for e in denials), first
        terminal.send("/permissions")
        menu = terminal.until(b"Choose 1 to 2: ", timeout=30)
        assert "Auto-approve" in menu, menu
        terminal.send("2")
        terminal.until(timeout=30)
        assert json.loads(settings.read_text())["implementer"]["permission_policy"] == "auto_approve"
        terminal.send(task)
        allowed = terminal.until(timeout=210)
        assert "approved" in allowed, allowed
        assert (project / "provider-edit.txt").read_text().strip() == "completed"
        native_calls = [json.loads(line) for line in calls.read_text().splitlines()]
        assert len(native_calls) == 2 and "--auto" not in native_calls[0]["args"] and "--auto" in native_calls[1]["args"], native_calls
        fixture_calls = [json.loads(line) for line in (root / "fixtures.jsonl").read_text().splitlines()]
        assert len(fixture_calls) == 4 and fixture_calls[2]["args"] == ["verify"], fixture_calls
        terminal.send("/quit")
        print("PASS: authenticated OpenCode Team implementation denied -> terminal permission change -> actual outside read and file write -> deterministic validation and fixture review")
    finally:
        terminal.close()
        assert account_settings.read_bytes() == account_settings_before, "Saved user configuration changed during the isolated test"
