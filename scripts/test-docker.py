#!/usr/bin/env python3
"""Offline image and single-container lifecycle checks using disposable resources."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import uuid
import pty
import select
import signal
import time
import errno

image = sys.argv[1] if len(sys.argv) > 1 else 'multiharness:check'
root = Path(__file__).resolve().parents[1]
name = 'multiharness-test-' + uuid.uuid4().hex
volume = name + '-state'
env = os.environ.copy()

def docker(*args, input=None, expected=0, timeout=120, cwd=None):
    result = subprocess.run(['docker', *args], input=input, text=True,
                            capture_output=True, timeout=timeout, env=env, cwd=cwd)
    if result.returncode != expected:
        raise AssertionError(f'docker {args[0]}: {result.returncode}, expected {expected}\n{result.stdout}{result.stderr}')
    return result.stdout + result.stderr

def terminal(command, lines, cwd=None):
    # Docker attach requires a real host TTY as well as the container TTY.
    pid, fd = pty.fork()
    if pid == 0:
        if cwd: os.chdir(cwd)
        os.execvpe('docker', ['docker', *command], env)
    output = b''
    sent = False
    started = time.monotonic()
    deadline = started + 120
    try:
        while time.monotonic() < deadline:
            if not sent and ((command[0] == 'attach' and time.monotonic() - started > 1) or b'Folder >' in output or b'Workspace restored:' in output):
                os.write(fd, lines.encode())
                sent = True
            if select.select([fd], [], [], 0.1)[0]:
                try: data = os.read(fd, 65536)
                except OSError as error:
                    if error.errno == errno.EIO: break
                    raise
                if not data: break
                output += data
        else: raise AssertionError('terminal timed out: ' + output.decode(errors='replace'))
        _, status = os.waitpid(pid, 0)
        assert os.waitstatus_to_exitcode(status) == 0, output.decode(errors='replace')
        assert sent, output.decode(errors='replace')
        return output.decode(errors='replace')
    finally:
        os.close(fd)
        try: os.kill(pid, signal.SIGKILL)
        except ProcessLookupError: pass


compose = None
try:
    with tempfile.TemporaryDirectory(prefix='multiharness-container-test-') as scratch:
        scratch = Path(scratch)
        project = scratch / 'project with spaces'
        project.mkdir()
        (project / 'api').mkdir()
        (project / 'user-note.txt').write_text('preserve me\n')
        env['MULTIHARNESS_WORKSPACE'] = str(project)
        env['MULTIHARNESS_UID'] = str(os.getuid()) if sys.platform.startswith('linux') else '1000'
        env['MULTIHARNESS_GID'] = str(os.getgid()) if sys.platform.startswith('linux') else '1000'
        assert env['MULTIHARNESS_UID'] != '0', 'Run container checks as a non-root user'
        override = scratch / 'override.json'
        override.write_text(json.dumps({'services': {'multiharness': {
            'image': image, 'container_name': name,
            'environment': {'PATH': '/tmp/account-fixtures:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin'},
        }}, 'volumes': {'state': {'name': volume}}}))
        ref = os.environ.get('MULTIHARNESS_TEST_COMPOSE_REF')
        source = f'https://github.com/ErOr-0/Multiharness-Core.git#{ref}' if ref else str(root / 'compose.yaml')
        compose = ['compose', '-p', name, '-f', source]
        if 'name=apparmor' in docker('info', '--format', '{{json .SecurityOptions}}'):
            compose += ['-f', f'{source}:docker/compose.linux.yaml' if ref else str(root / 'docker/compose.linux.yaml')]
        compose += ['-f', str(override)]
        docker(*compose, 'create')
        original_id = docker('inspect', '--format', '{{.Id}}', name).strip()
        assert docker('inspect', '--format', '{{.HostConfig.AutoRemove}}', name).strip() == 'false'
        assert json.loads(docker('inspect', '--format', '{{json .HostConfig.CapDrop}}', name)) == ['ALL']
        security = json.loads(docker('inspect', '--format', '{{json .HostConfig.SecurityOpt}}', name))
        assert 'no-new-privileges=true' in security
        profile = next(option.removeprefix('seccomp=') for option in security if option.startswith('seccomp='))
        assert json.loads(profile) == json.loads((root / 'docker/seccomp.json').read_text())
        if 'name=apparmor' in docker('info', '--format', '{{json .SecurityOptions}}'):
            assert 'apparmor=magent-container-v1' in security
        assert 'linux/' in docker('run', '--rm', image, '--version')
        # Main process remains idle at the prompt; exec never creates a container.
        docker('start', name)
        assert 'Codex read-only sandbox: available' in docker('exec', name, 'magent-container', 'doctor')
        script = '''set -eu
id -u | grep -v '^0$'
test "$(awk '/CapEff:/ {print $2}' /proc/self/status)" = 0000000000000000
test "$(awk '/NoNewPrivs:/ {print $2}' /proc/self/status)" = 1
mkdir /tmp/outer-mount
if mount -t tmpfs tmpfs /tmp/outer-mount; then exit 1; fi
mkdir /workspace/web
git init -q /workspace/api
git init -q /workspace/web
printf changed > /workspace/container-edit.txt
'''
        docker('exec', name, '/bin/sh', '-eu', '-c', script)
        output = terminal(['attach', name], 'api\n\n\n\n\nfixture/model\n\n/quit\n', cwd=scratch)
        assert 'Workspace selected: /workspace/api' in output
        assert 'Team saved automatically' in output
        assert docker('inspect', '--format', '{{.State.Status}}', name).strip() == 'exited'
        output = terminal(['start', '-ai', name], '/settings\n/quit\n', cwd='/')
        assert 'Workspace restored: /workspace/api' in output and 'fixture/model' in output
        assert 'CHOOSE A WORKSPACE' not in output and 'CONFIGURE YOUR TEAM' not in output
        assert docker('inspect', '--format', '{{.Id}}', name).strip() == original_id
        # Run the real intake path against nested repositories, without a model.
        docker('start', name)
        output = docker('exec', name, 'magent-container', '--quiet', '--planner-executable',
                        '/missing-provider', '--task', 'offline intake check', expected=1)
        result = json.loads(output)
        assert result['failure']['stage'] == 'planning', result
        docker('exec', name, 'magent-container', 'codex', 'sandbox', '-c',
               'sandbox_mode="read-only"', '--', '/bin/sh', '-eu', '-c',
               'cat /workspace/user-note.txt; if touch /workspace/forbidden; then exit 1; fi')
        docker('exec', name, 'magent-container', 'codex', 'sandbox', '-c',
               'sandbox_mode="workspace-write"', '--', '/bin/sh', '-eu', '-c',
               'printf sandboxed > /workspace/sandbox-edit.txt')
        # Fixed local fixtures verify /login invokes account setup in this same
        # container. They replace no installed provider and make no network call.
        docker('exec', name, '/bin/sh', '-eu', '-c', '''mkdir -p /tmp/account-fixtures
printf '#!/bin/sh\nprintf "account-fixture\\n"\n' > /tmp/account-fixtures/codex
cp /tmp/account-fixtures/codex /tmp/account-fixtures/opencode
chmod 755 /tmp/account-fixtures/*''')
        output = terminal(['attach', name], '/login codex\n/login opencode\n/quit\n')
        assert output.count('Account setup finished') == 2, output
        assert docker('inspect', '--format', '{{.Id}}', name).strip() == original_id
        # Recreate only this service as an update would; reuse the state volume.
        docker(*compose, 'create', '--force-recreate')
        replacement_id = docker('inspect', '--format', '{{.Id}}', name).strip()
        assert replacement_id != original_id
        output = terminal(['start', '-ai', name], '/settings\n/quit\n')
        assert 'Workspace restored: /workspace/api' in output and 'fixture/model' in output
        assert (project / 'user-note.txt').read_text() == 'preserve me\n'
        assert (project / 'container-edit.txt').read_text() == 'changed'
        assert (project / 'sandbox-edit.txt').read_text() == 'sandboxed'
        containers = docker('ps', '-a', '--filter', f'label=com.docker.compose.project={name}', '--format', '{{.ID}}').splitlines()
        assert len(containers) == 1, containers
        print('PASS: one reusable container, starts from unrelated folders, saved workspace/team, account setup, update persistence, mounts and sandbox')
finally:
    subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run(['docker', 'volume', 'rm', volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run(['docker', 'network', 'rm', name + '_default'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
