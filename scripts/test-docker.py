#!/usr/bin/env python3
"""Offline checks of the actual image. No accounts, host credentials or models."""
import pathlib
import json
import os
import subprocess
import sys
import tempfile
import uuid

image = sys.argv[1] if len(sys.argv) > 1 else "multiharness-core:dev"
volume = "magent-test-" + uuid.uuid4().hex
workspace_root = pathlib.Path(__file__).resolve().parents[1]
scratch = workspace_root / ".coverage"
scratch.mkdir(exist_ok=True)


def docker(*args, input=None, expected=0, timeout=120):
    result = subprocess.run(
        ["docker", *args], input=input.encode("utf-8") if input is not None else None,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout,
    )
    if result.returncode != expected:
        raise AssertionError(
            f"docker {args[0]} exited {result.returncode}, expected {expected}\n"
            + result.stdout.decode("utf-8", errors="replace") + result.stderr.decode("utf-8", errors="replace")
        )
    return (result.stdout + result.stderr).decode("utf-8", errors="replace")


try:
    with tempfile.TemporaryDirectory(prefix="docker project ", dir=scratch) as folder:
        project = pathlib.Path(folder).resolve()
        assert project.parent == scratch.resolve()
        base = [
            "run", "--rm", "--init", "--interactive",
            "--cap-drop", "ALL", "--security-opt", "no-new-privileges=true",
            "--security-opt", f"seccomp={workspace_root / 'docker' / 'seccomp.json'}",
            "--mount", f"type=bind,src={project},dst=/workspace",
            "--mount", f"type=volume,src={volume},dst=/state",
        ]
        if sys.platform.startswith("linux"):
            base += ["--user", f"{os.getuid()}:{os.getgid()}"]
        apparmor = "name=apparmor" in docker("info", "--format", "{{json .SecurityOptions}}")
        if apparmor:
            base += ["--security-opt", "apparmor=magent-container-v1"]

        def run(*args, **kwargs):
            return docker(*base, image, *args, **kwargs)

        def check_intake():
            # Reach planning using the real binary and host bind mount, with a
            # deliberately absent provider. No credentials or models are used.
            output = run("--quiet", "--planner-executable", "/magent-missing-cli",
                         "--task", "offline folder intake check", expected=1)
            result = json.loads(output)
            assert result["status"] == "failed", result
            assert result["failure"]["stage"] == "planning", result

        assert "linux/" in docker("run", "--rm", image, "--version")
        assert "Missing persistent state" in docker("run", "--rm", image, expected=2)
        assert "Mount your project folder" in docker(
            "run", "--rm", "--mount", f"type=volume,src={volume},dst=/state",
            image, expected=2,
        )
        # A plain parent folder works without initializing Git.
        assert "Git optional" in run("doctor")
        # Exercise the exact downloadable Compose configuration with a host
        # bind and isolated state, including Docker's seccomp file loading.
        with tempfile.TemporaryDirectory(prefix="compose-", dir=scratch) as package:
            package = pathlib.Path(package)
            template = (workspace_root / "web/public/compose.yaml").read_text(encoding="utf-8")
            definition = json.loads("\n".join(line for line in template.splitlines() if not line.startswith("#")))
            service = definition["services"]["magent"]
            service["image"] = image
            service["user"] = f"{os.getuid()}:{os.getgid()}" if sys.platform.startswith("linux") else "1000:1000"
            service["volumes"][0]["source"] = str(project)
            definition["volumes"]["state"] = {"name": volume, "external": True}
            if apparmor:
                service["security_opt"].append("apparmor=magent-container-v1")
            config_file = package / "compose.yaml"
            config_file.write_text(json.dumps(definition), encoding="utf-8")
            (package / "seccomp.json").write_bytes((workspace_root / "docker/seccomp.json").read_bytes())
            compose = ["compose", "-p", volume, "-f", str(config_file)]
            try:
                assert "Codex read-only sandbox: available" in docker(*compose, "run", "--rm", "-T", "magent", "doctor")
            finally:
                docker(*compose, "down")  # Only this unique test network; retain state for later checks.
        if sys.platform != "win32":
            launched = subprocess.run(
                ["sh", str(workspace_root / "scripts" / "magent-docker.sh"),
                 "--project", str(project), "--image", image, "--state", volume,
                 "--no-tty", "doctor"], capture_output=True, text=True, timeout=120,
            )
            assert launched.returncode == 0, launched.stdout + launched.stderr
            assert "Codex read-only sandbox: available" in launched.stdout
        if apparmor:
            assert "magent-container-v1 (enforce)" in run(
                "shell", input='cat /proc/self/attr/current\n')
        # Namespace permissions must not enable mounts in the outer container.
        run("shell", input='set -eu\nmkdir "$HOME/outer-mount"\n'
            'if mount -t tmpfs tmpfs "$HOME/outer-mount"; then exit 1; fi\n'
            'test "$(awk \'/CapEff:/ {print $2}\' /proc/self/status)" = 0000000000000000\n'
            'test "$(awk \'/NoNewPrivs:/ {print $2}\' /proc/self/status)" = 1\n')
        check_intake()
        assert not (project / ".git").exists()
        # Setup of child repositories in our disposable fixture only.
        docker(*base, "--entrypoint", "/bin/sh", image, "-eu", "-c",
               "mkdir /workspace/api /workspace/web; "
               "git init -q /workspace/api; git init -q /workspace/web")
        check_intake()
        (project / "user-note.txt").write_text("preserve me\n", encoding="utf-8")
        run("shell", input='set -eu\n'
            'test "$(id -u)" != 0\n'
            'git -C /workspace/api status --porcelain\n'
            'git -C /workspace/web status --porcelain\n'
            'codex sandbox -c sandbox_mode=\'"read-only"\' -- git -C /workspace/api status --porcelain\n'
            'test "$(stat -c %a "$HOME")" = 700\n'
            'printf saved > "$HOME/persistence-check"\n'
            'printf changed > /workspace/container-edit.txt\n')
        run("shell", input='set -eu\ntest "$(cat "$HOME/persistence-check")" = saved\n')
        assert (project / "container-edit.txt").read_text() == "changed"
        assert (project / "user-note.txt").read_text() == "preserve me\n"
        # An arbitrary non-root Linux UID can create its private persistent home.
        docker(*base, "--user", "12345:12345", image, "codex", "--version")
        assert "Missing project tool" in run("doctor", "magent-deliberately-missing-tool", expected=2)
        assert "interactive terminal" in run("setup", expected=2)
        assert "No model calls" in run("doctor")
        run("codex", "sandbox", "-c", 'sandbox_mode="read-only"', "--",
            "/bin/sh", "-eu", "-c",
            'cat /workspace/user-note.txt; '
            'if touch /workspace/forbidden; then exit 1; fi; test ! -e /workspace/forbidden')
        run("codex", "sandbox", "-c", 'sandbox_mode="workspace-write"', "--",
            "/bin/sh", "-eu", "-c", 'printf sandboxed > /workspace/sandbox-edit.txt')
        assert (project / "sandbox-edit.txt").read_text() == "sandboxed"
        # Exercise the real interactive setup and /save through a container PTY.
        terminal_check = r'''
import errno, json, os, pathlib, pty, select, signal, time
def session(args, exchanges):
    pid, fd = pty.fork()
    if pid == 0:
        os.execv('/usr/local/bin/magent-container', ['magent-container', *args])
    output = b''
    deadline = time.monotonic() + 40
    try:
        while time.monotonic() < deadline:
            if select.select([fd], [], [], 0.2)[0]:
                try:
                    chunk = os.read(fd, 8192)
                except OSError as error:
                    if error.errno == errno.EIO:
                        break
                    raise
                if not chunk:
                    break
                output += chunk
                if exchanges and exchanges[0][0].encode() in output:
                    _, answer = exchanges.pop(0)
                    os.write(fd, answer.encode())
        else:
            raise AssertionError('interactive session timed out')
        _, status = os.waitpid(pid, 0)
        assert os.waitstatus_to_exitcode(status) == 0, output.decode(errors='replace')
        assert not exchanges, output.decode(errors='replace')
        return output
    finally:
        os.close(fd)
        try:
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
session(['setup'], [('Sign in to Codex', 'n\n'), ('Configure an OpenCode', 'n\n')])
session([], [('Folder > ', '0\n/set implementer-model opencode/container-test\n/save\n/quit\n')])
settings = pathlib.Path('/state') / str(os.getuid()) / '.config/magent/config.json'
assert json.loads(settings.read_text())['implementer']['model'] == 'opencode/container-test'
output = session([], [('Folder > ', '0\n/settings\n/quit\n')])
assert b'opencode/container-test' in output
output = session([], [('Folder > ', 'cd api\nmkdir browser-created\ncd browser-created\n\n/settings\n/quit\n')])
assert b'Workspace selected: /workspace/api/browser-created' in output
assert pathlib.Path('/workspace/api/browser-created').is_dir()
'''
        docker(*base, "--entrypoint", "python3", image, "-c", terminal_check)
        print("PASS: plain/multi-repository intake, nested Git ownership in the Codex sandbox, startup, private state, mounted edits, tooling and interactive setup/settings")
finally:
    # Only the unique disposable volume created by this script is removed.
    subprocess.run(["docker", "volume", "rm", volume], stdout=subprocess.DEVNULL, check=False)
