"""Real Claude CLI and permission engine, with a local simulated Anthropic model.

This is native integration evidence, not authenticated model-behavior evidence.
Run in an isolated container without external network access or account state.
"""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import uuid

from terminal_driver import Terminal

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--binary", required=True)
args = parser.parse_args()


class Model(BaseHTTPRequestHandler):
    tool_command = ""
    requests = 0

    def log_message(self, *args):
        pass

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        if "count_tokens" in self.path:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"input_tokens":10}')
            return
        type(self).requests += 1
        tool = type(self).requests == 1
        block = {"type": "tool_use", "id": "toolu_" + uuid.uuid4().hex, "name": "Bash", "input": {"command": self.tool_command, "description": "Exercise the selected permission mode"}} if tool else {"type": "text", "text": "Native permission check completed."}
        message = {"id": "msg_" + uuid.uuid4().hex, "type": "message", "role": "assistant", "model": request.get("model", "claude-sonnet-4-6"), "content": [block], "stop_reason": "tool_use" if tool else "end_turn", "stop_sequence": None, "usage": {"input_tokens": 10, "output_tokens": 10}}
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream" if request.get("stream") else "application/json")
        self.end_headers()
        if not request.get("stream"):
            self.wfile.write(json.dumps(message).encode())
            return
        start = {**message, "content": [], "stop_reason": None}
        events = [
            ("message_start", {"message": start}),
            ("content_block_start", {"index": 0, "content_block": {**block, **({"input": {}} if tool else {"text": ""})}}),
            ("content_block_delta", {"index": 0, "delta": {"type": "input_json_delta", "partial_json": json.dumps(block["input"])} if tool else {"type": "text_delta", "text": block["text"]}}),
            ("content_block_stop", {"index": 0}),
            ("message_delta", {"delta": {"stop_reason": message["stop_reason"], "stop_sequence": None}, "usage": {"output_tokens": 10}}),
            ("message_stop", {}),
        ]
        for event, payload in events:
            self.wfile.write(("event: " + event + "\ndata: " + json.dumps({"type": event, **payload}) + "\n\n").encode())
        self.wfile.flush()


with tempfile.TemporaryDirectory(prefix="magent-native-claude-") as temp:
    root = Path(temp)
    project = root / "project"
    project.mkdir()
    home = root / "home"
    home.mkdir()
    settings = home / ".config/magent/config.json"
    settings.parent.mkdir(parents=True)
    record = root / "native-calls.jsonl"
    wrapper = root / "claude-recorder"
    # Pass stdin straight through: the native approval protocol is duplex and
    # cannot wait for EOF before starting Claude. Record before forwarding the
    # result because Multiharness then stops the completed process tree.
    wrapper.write_text("#!/usr/bin/python3\nimport json,pathlib,subprocess,sys\n"
        "p=subprocess.Popen(['claude',*sys.argv[1:]],stdin=sys.stdin.buffer,stdout=subprocess.PIPE)\n"
        "events=[]\n"
        "for line in p.stdout:\n"
        " try: event=json.loads(line)\n"
        " except ValueError: event={}\n"
        " events.append(event)\n"
        " if event.get('type')=='result':\n"
        f"  with pathlib.Path({str(record)!r}).open('a') as f: f.write(json.dumps({{'args':sys.argv[1:],'events':events}})+'\\n')\n"
        " sys.stdout.buffer.write(line);sys.stdout.buffer.flush()\n"
        "sys.exit(p.wait())\n")
    wrapper.chmod(0o755)
    settings.write_text(json.dumps({"version": 1, "mode": "direct", "working_dir": str(project), "color": "never", "progress": "off", "timeout": "90s", "implementer": {"harness": "claude", "executable": str(wrapper), "model": "claude-sonnet-4-6", "reasoning": "low", "permission_policy": "reject_on_prompt", "timeout": "90s"}}))
    server = ThreadingHTTPServer(("127.0.0.1", 0), Model)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    # Deliberately omit real account and provider environment variables.
    env = {k: os.environ[k] for k in ("PATH", "LANG", "LD_LIBRARY_PATH") if k in os.environ}
    env.update(HOME=str(home), XDG_CONFIG_HOME=str(home / ".config"), CLAUDE_CONFIG_DIR=str(home / ".claude"),
               ANTHROPIC_BASE_URL=f"http://127.0.0.1:{server.server_port}", ANTHROPIC_API_KEY="local-fixture-not-a-real-key",
               CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1", DISABLE_AUTOUPDATER="1", MULTIHARNESS_CONFIG=str(settings),
               MULTIHARNESS_INSTALL_MODE="disabled", MULTIHARNESS_RUNTIME_CHECK="0", TERM="dumb", NO_COLOR="1")
    terminal = Terminal(args.binary, project, env)
    try:
        terminal.until()
        terminal.send("/permissions")
        menu = terminal.until(b"Choose 1 to 4: ")
        assert "CLAUDE PERMISSIONS" in menu and "Automatic review" in menu and "Bypass permission prompts" in menu, menu
        terminal.command("/cancel")
        cases = [
            ("native", "reject_on_prompt", f"printf denied > '{project / 'denied.txt'}'", project / "denied.txt", False, ""),
            ("native", "reject_on_prompt", f"printf granted > '{project / 'once-approved.txt'}'", project / "once-approved.txt", True, "1"),
            ("native", "reject_on_prompt", f"mkdir -p '{project / 'rule-approved'}'", project / "rule-approved", True, "2"),
            ("native", "reject_on_prompt", f"mkdir -p '{project / 'rule-approved'}'", project / "rule-approved", True, None),
            ("edits", "accept_edits", f"mkdir '{project / 'edits-approved'}'", project / "edits-approved", True, None),
            ("full", "bypass_permissions", f"printf granted > '{root / 'outside-approved.txt'}'", root / "outside-approved.txt", True, None),
            ("native", "reject_on_prompt", f"printf denied > '{project / 'revoked.txt'}'", project / "revoked.txt", False, ""),
        ]
        for mode, policy, command, path, allowed, choice in cases:
            if mode == "native" and choice is None:
                path.rmdir()  # Prove the saved rule permits the same command again.
            terminal.command("/permissions " + mode)
            assert json.loads(settings.read_text())["implementer"]["permission_policy"] == policy
            Model.tool_command, Model.requests = command, 0
            terminal.send("Execute the permission test command provided by the local model fixture.")
            if choice is not None:
                prompt = terminal.until(b"Choose a number (Enter denies): ")
                assert "Bash" in prompt and "Allow once" in prompt, prompt
                assert not path.exists(), "Tool ran before user approval"
                if choice == "2":
                    assert "localSettings" in prompt and "mkdir" in prompt, prompt
                terminal.send(choice)
            result = terminal.until()
            assert "Choose a number" not in result, result
            assert ("RESPONDED" if allowed else "NEEDS_INPUT") in result, result
            assert path.exists() == allowed, (mode, result)
            assert Model.requests >= 1, "Native CLI did not contact the local model"
        assert (root / "outside-approved.txt").read_text() == "granted"
        assert (project / "once-approved.txt").read_text() == "granted"
        assert (project / "rule-approved").is_dir()
        native_settings = json.loads((project / ".claude/settings.local.json").read_text())
        assert any("mkdir" in rule for rule in native_settings["permissions"]["allow"]), native_settings
        calls = [json.loads(line) for line in record.read_text().splitlines()]
        assert len(calls) == len(cases), "An approval must continue the waiting native turn without replay"
        modes = [call["args"][call["args"].index("--permission-mode") + 1] for call in calls]
        assert modes == ["default", "default", "default", "default", "acceptEdits", "bypassPermissions", "default"], modes
        sessions = [{e["session_id"] for e in call["events"] if e.get("session_id")} for call in calls]
        assert len(sessions[0]) == 1 and all(s == sessions[0] for s in sessions), sessions
        for call in calls[1:]:
            assert any(arg == "--resume=" + next(iter(sessions[0])) for arg in call["args"])
        # Parse compatibility only: auto availability depends on the real account/model.
        check = subprocess.run(["claude", "--permission-mode", "auto", "--help"], env=env, capture_output=True)
        assert check.returncode == 0
        print("PASS: real Claude permission engine paused for Multiharness approval, denied Bash, allowed once, saved and reused a native rule, accepted file operations, allowed full-mode outside write, then denied after revocation; same native session without replay; local simulated model, no account")
    except Exception:
        if record.exists():
            print(record.read_text())
        raise
    finally:
        terminal.close()
        server.shutdown()
        server.server_close()
