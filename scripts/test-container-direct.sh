#!/bin/sh
# Offline packaged direct-mode check; only use disposable tmpfs mounts.
set -eu
[ "$(id -u)" = 0 ]
mountpoint -q /workspace
mountpoint -q /state
mkdir /tmp/direct-fixtures
cat > /tmp/direct-fixtures/opencode <<'PY'
#!/usr/bin/env python3
import json, pathlib, sys
args = sys.argv[1:]
assert args[:3] == ['run', '--format', 'json'], args
assert args[args.index('--dir') + 1] == '/workspace'
assert '--auto' not in args and '--output-schema' not in args
assert sys.stdin.read() == 'write the fixture'
pathlib.Path('/workspace/direct-fixture.txt').write_text('native edit')
print(json.dumps({'type': 'step_start', 'sessionID': 'ses_fixture', 'part': {'type': 'step-start'}}))
print(json.dumps({'type': 'text', 'sessionID': 'ses_fixture', 'part': {'type': 'text', 'text': 'Native response'}}))
print(json.dumps({'type': 'step_finish', 'sessionID': 'ses_fixture', 'part': {'type': 'step-finish', 'reason': 'stop'}}))
PY
chmod 755 /tmp/direct-fixtures/opencode
setpriv --reuid=1000 --regid=1000 --clear-groups /usr/local/bin/magent-container \
 --task 'write the fixture' --implementer-executable /tmp/direct-fixtures/opencode \
 --planner-executable /missing-planner --reviewer-executable /missing-reviewer \
 --progress off > /tmp/direct-result.json
python3 - <<'PY'
import json
from pathlib import Path
r = json.loads(Path('/tmp/direct-result.json').read_text())
assert r['status'] == 'responded' and r['agent_invocations'] == 1, r
assert r['direct']['text'] == 'Native response' and r['direct']['session_id'] == 'ses_fixture'
assert not any(k in r for k in ['plan', 'validation', 'last_review']), r
assert Path('/workspace/direct-fixture.txt').read_text() == 'native edit'
assert Path('/workspace/direct-fixture.txt').stat().st_uid == 1000
print('PASS: packaged default direct mode, native edit, one invocation, no planner/reviewer')
PY
python3 - <<'PY'
import errno, os, pty, select, signal, time
pid, fd = pty.fork()
if pid == 0:
    os.setgroups([])
    os.setgid(1000)
    os.setuid(1000)
    os.execv('/usr/local/bin/magent-container', ['magent-container'])
output = b''
sent = False
try:
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if not sent and b'Folder >' in output:
            os.write(fd, b'\nopencode\n\n\n/config\n3\n2\n/settings\n/permissions native\n/new\n/quit\n')
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
        raise AssertionError('interactive setup timed out: ' + output.decode(errors='replace'))
    _, status = os.waitpid(pid, 0)
    assert os.waitstatus_to_exitcode(status) == 0, output
    assert sent and b'3/3' in output and b'Agent saved.' in output, output
    assert b'DIRECT' in output and b'New conversation.' in output, output
    assert b'OPENCODE PERMISSIONS' in output and b'Auto-approve requests (--auto)' in output, output
    assert b'OpenCode permissions saved: Native rules' in output, output
    import json
    from pathlib import Path
    saved = json.loads(Path('/state/1000/.config/magent/config.json').read_text())
    assert saved['implementer']['permission_policy'] == 'reject_on_prompt', saved
    assert b'1/9' not in output, output
    print('PASS: packaged interactive three-field setup, /config permissions menu, saved permission choices and /new')
finally:
    os.close(fd)
    try:
        os.kill(pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
PY
