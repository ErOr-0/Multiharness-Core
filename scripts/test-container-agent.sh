#!/bin/sh
# Offline test of the packaged entrypoint, real CLI, agent JSONL and login shell.
# Run only in a disposable container with empty /workspace and /state tmpfs.
set -eu
[ "$(id -u)" = 0 ]
mountpoint -q /workspace
mountpoint -q /state
mkdir /tmp/agent-fixtures
cat > /tmp/agent-fixtures/codex <<'PY'
#!/usr/bin/env python3
import json, pathlib, subprocess, sys
args = sys.argv[1:]
if args == ['--version']:
    print('codex-cli 0.153.0'); sys.exit(0)
assert args[0] == 'exec'
assert args[args.index('--sandbox')+1] == 'read-only'
assert 'model_reasoning_effort="medium"' in args
sys.stdin.read()
command = 'set -eu; set -o pipefail; go version; printf "package fixture\\n" | gofmt'
print(json.dumps({'type':'item.started','item':{'type':'command_execution','command':command}}), flush=True)
r = subprocess.run(['/bin/bash','-lc',command], text=True, capture_output=True)
assert r.returncode == 0, r.stderr
assert 'go1.' in r.stdout and 'package fixture' in r.stdout
print(json.dumps({'type':'item.completed','item':{'type':'command_execution','command':command,'aggregated_output':r.stdout,'exit_code':r.returncode}}), flush=True)
# Opaque tool data must not be confused with an error contract.
print('{"type":"item.completed","item":{"type":"mcp_tool_call","result":{"value":1,"value":2}}}', flush=True)
pathlib.Path(args[args.index('--output-last-message')+1]).write_text(json.dumps({'schema_version':'2','action':'answer','answer':'Container toolchain checked.','summary':'offline answer','steps':[],'acceptance_criteria':[]}))
print('{"type":"turn.completed"}', flush=True)
PY
chmod 755 /tmp/agent-fixtures/codex
setpriv --reuid=1000 --regid=1000 --clear-groups /usr/local/bin/magent-container \
 --task 'Check the container toolchain without changing files' \
 --planner-executable /tmp/agent-fixtures/codex --planner-reasoning medium \
 --implementer-executable /missing-implementer --reviewer-executable /missing-reviewer \
 --progress plain --color never > /tmp/agent-result.json 2> /tmp/agent-progress.txt
python3 - <<'PY'
import json
from pathlib import Path
result=json.loads(Path('/tmp/agent-result.json').read_text())
assert result['status']=='answered', result
progress=Path('/tmp/agent-progress.txt').read_text()
assert 'go version' in progress and 'package fixture' in progress and 'shell exit 0' in progress, progress
assert 'malformed_error_event' not in progress
assert 'implementing' not in progress
print('PASS: packaged CLI answer, configured reasoning, real Go login-shell command, streamed output and opaque tool event')
PY
