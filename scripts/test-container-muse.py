"""Contract check: the bundled Muse CLI must report a turn over `muse serve`.

Magent drives Muse through this protocol and waits for `turn/completed`. Muse
1.4.0 accepted turns but never reported them, which held tasks until the role
deadline. The echo provider needs no account and makes no model call.
"""
import json
import os
import select
import subprocess
import time

assert os.getuid() == 0
os.setgroups([])
os.setgid(1000)
os.setuid(1000)
os.environ["HOME"] = "/state/1000"
os.makedirs("/state/1000", exist_ok=True)


def uuid7():
    stamp = int(time.time() * 1000).to_bytes(6, "big")
    rest = bytearray(os.urandom(10))
    rest[0] = (rest[0] & 0x0F) | 0x70
    rest[2] = (rest[2] & 0x3F) | 0x80
    value = (stamp + bytes(rest)).hex()
    return f"{value[:8]}-{value[8:12]}-{value[12:16]}-{value[16:20]}-{value[20:]}"


host = subprocess.Popen(["muse", "serve", "--no-session-log", "--disable-write", "--disable-shell"],
                        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, cwd="/workspace")
seen = []


def send(message):
    host.stdin.write((json.dumps(message) + "\n").encode())
    host.stdin.flush()


def wait(match, seconds=30):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        for message in seen:
            if match(message):
                return message
        if select.select([host.stdout], [], [], 0.2)[0]:
            line = host.stdout.readline()
            if not line:
                break
            seen.append(json.loads(line))
    raise AssertionError("Muse serve did not report the expected message: " + json.dumps(seen)[-2000:])


try:
    send({"jsonrpc": "2.0", "id": 1, "method": "initialize",
          "params": {"clientInfo": {"name": "multiharness", "version": "1"}, "capabilities": {"userInputDialogs": False}}})
    assert wait(lambda m: m.get("id") == 1)["result"]["serverInfo"]["name"] == "muse"
    send({"jsonrpc": "2.0", "method": "initialized", "params": {}})
    send({"jsonrpc": "2.0", "id": 2, "method": "session/start",
          "params": {"commandId": uuid7(), "workspaceRoot": "/workspace", "modelId": "echo", "providerId": "echo",
                     "approvalMode": "promptUnmatched"}})
    session = wait(lambda m: m.get("id") == 2)["result"]["session"]["sessionId"]
    send({"jsonrpc": "2.0", "id": 3, "method": "turn/start",
          "params": {"commandId": uuid7(), "sessionId": session, "input": [{"type": "text", "text": "contract check"}],
                     "reasoningEffort": "high"}})
    turn = wait(lambda m: m.get("id") == 3)["result"]["turnId"]
    done = wait(lambda m: m.get("method") == "turn/completed" and m["params"].get("turnId") == turn)
    assert done["params"]["sessionId"] == session and done["params"].get("terminal"), done
finally:
    host.kill()
    host.wait()
print("PASS: bundled Muse reports turn completion over serve")
