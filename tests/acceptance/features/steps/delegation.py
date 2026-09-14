import json
import os
from pathlib import Path
import subprocess
import sys
import time
import uuid

from behave import given, when, then


def save_config(context):
    context.config_path.write_text(json.dumps(context.settings), encoding="utf-8")


def ensure_binary(context):
    if context.build_state["binary_needed"]:
        subprocess.run(["go", "build", "-o", context.binary, "./cmd/multiharness"], cwd=context.repo, check=True, timeout=180)
        context.build_state["binary_needed"] = False


def invoke(context, prompt, session=None):
    ensure_binary(context)
    context.prompt = prompt
    command = [context.binary, "--config", str(context.config_path),
               "--workdir", str(context.workspace), "--install-mode", "disabled",
               "--progress", "off", *context.overrides]
    if session:
        command += ["--session-id", session]
    start = time.monotonic()
    context.process = subprocess.run(command + ["--task", prompt], env=context.env,
                                     capture_output=True, text=True, encoding="utf-8", timeout=330)
    context.elapsed = time.monotonic() - start
    try:
        context.result = json.loads(context.process.stdout)
    except json.JSONDecodeError:
        context.result = None


def calls(context):
    path = Path(context.env["BDD_RECORD"])
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()] if path.exists() else []


@given('a disposable workspace configured for "{provider}"')
def configured(context, provider):
    if not context.build_state["fixture_built"]:
        subprocess.run(["go", "build", "-o", context.fixture, "./tests/acceptance/fixture"], cwd=context.repo, check=True, timeout=180)
        context.build_state["fixture_built"] = True
    context.provider = provider
    context.env["BDD_PROVIDER"] = provider
    role = {"harness": provider, "executable": context.fixture, "model": "fixture/model", "timeout": "30s"}
    role["variant" if provider == "opencode" else "reasoning"] = "high"
    context.settings = {"version": 1, "timeout": "30s", "implementer": role,
                        "planner": {"executable": str(context.root / "missing-planner")},
                        "reviewer": {"executable": str(context.root / "missing-reviewer")}}
    save_config(context)


@given('the provider will "{behavior}"')
@then('the provider will "{behavior}"')
def behavior(context, behavior):
    context.env["BDD_BEHAVIOR"] = behavior


@given('the "{setting}" deadline is 2 seconds')
def deadline(context, setting):
    if setting == "timeout":
        context.settings["timeout"] = "2s"
    else:
        context.settings["implementer"]["timeout"] = "2s"
    save_config(context)


@given('the configured mode is "{mode}"')
def mode(context, mode):
    context.settings["mode"] = mode
    save_config(context)


@when('I submit "{prompt}"')
def submit(context, prompt):
    invoke(context, prompt)


@then('the command exits with {code:d} and status "{status}"')
def outcome(context, code, status):
    assert context.process.returncode == code, (context.process.returncode, context.process.stdout, context.process.stderr)
    assert context.result and context.result["status"] == status, context.result


@then('exactly one provider process received the original task and configured settings')
def invocation(context):
    recorded = calls(context)
    assert len(recorded) == 1, recorded
    call = recorded[0]
    assert call["task"] == context.prompt, call
    assert Path(call["cwd"]).resolve() == context.workspace.resolve(), call
    args = call["args"]
    assert args[args.index("--model") + 1] == "fixture/model", args
    assert "--output-schema" not in args and "--json-schema" not in args and "--ephemeral" not in args, args
    if context.provider == "codex":
        assert args[0] == "exec" and args[-1] == "-" and "--json" in args, args
        assert 'model_reasoning_effort="high"' in args, args
        assert 'sandbox_mode="workspace-write"' in args and 'approval_policy="never"' in args, args
    elif context.provider == "opencode":
        assert args[:3] == ["run", "--format", "json"] and "--auto" not in args, args
        assert args[args.index("--dir") + 1] == str(context.workspace), args
        assert args[args.index("--variant") + 1] == "high", args
    else:
        assert "--print" in args and "--dangerously-skip-permissions" not in args, args
        assert args[args.index("--output-format") + 1] == "stream-json", args
        assert args[args.index("--effort") + 1] == "high", args
        assert args[args.index("--permission-mode") + 1] == "dontAsk", args


@then('the provider edit exists in the chosen workspace')
def edit(context):
    assert (context.workspace / "provider-edit.txt").read_text() == "native edit"


@then('no retry or team workflow ran')
def no_team(context):
    assert len(calls(context)) == 1, calls(context)
    assert context.result["agent_invocations"] == 1, context.result
    assert not any(k in context.result for k in ["plan", "implementation", "validation", "repository", "last_review", "repair_rounds"]), context.result


@then('the result contains the native response without team evidence')
def native_response(context):
    no_team(context)
    assert context.result["direct"]["text"] == "Native response", context.result
    assert context.result["direct"]["session_id"] == "session_fixture_123", context.result


@when('I submit a follow-up using the returned session')
def followup(context):
    context.first_session = context.result["direct"]["session_id"]
    invoke(context, "continue this exact conversation", context.first_session)


@then('the second process resumes the exact first session')
def resumed(context):
    recorded = calls(context)
    assert len(recorded) == 2, recorded
    args = recorded[1]["args"]
    if context.provider == "codex":
        assert args[:2] == ["exec", "resume"] and args[-2] == context.first_session, args
    else:
        flag = "--session" if context.provider == "opencode" else "--resume"
        assert args[args.index(flag) + 1] == context.first_session, args
    assert context.result["direct"]["session_id"] == context.first_session


@then('the result names the "{setting}" deadline and keeps the partial response')
def timeout_response(context, setting):
    assert setting + " deadline (2s)" in context.result["summary"], context.result
    assert context.result["direct"]["text"] == "Native response", context.result
    assert context.result["direct"]["session_id"] == "session_fixture_123", context.result
    assert context.elapsed < 15, context.elapsed


@then('the provider process stops producing filesystem activity')
def stopped(context):
    heartbeat = context.workspace / "heartbeat.txt"
    before = heartbeat.read_bytes()
    time.sleep(0.3)
    assert heartbeat.read_bytes() == before, "timed-out provider is still running"


@then('configuration is rejected before a provider process starts')
def rejected(context):
    assert context.process.returncode == 2, context.process.stdout
    assert not calls(context), calls(context)
    assert not (context.workspace / "provider-edit.txt").exists()
    assert context.result and context.result["status"] == "failed", context.result
    assert "mode" in context.result["failure"]["message"].lower(), context.result


@given('a real native provider selected by an explicit configuration file')
def live_config(context):
    assert not os.environ.get("CI"), "Live acceptance must run in an explicitly selected local account environment"
    path = context.config.userdata.get("live_config")
    assert path, "@live requires -D live_config=/absolute/path/config.json; it never silently skips"
    context.config_path = Path(path).resolve(strict=True)
    context.overrides = ["--mode", "direct", "--timeout", "5m", "--implementer-timeout", "5m"]
    context.token = "conversation-" + uuid.uuid4().hex


@when('I ask the agent to implement integer addition and remember a random token')
def real_task(context):
    invoke(context, "In this empty scratch folder, create add.py with add(a, b) returning a+b, "
           "and test_add.py containing runnable Python unittest tests for positive and negative integers. "
           "Run the tests if your permissions allow. Do not create other files. "
           f"Remember this conversation token without writing it to any file: {context.token}. "
           "Give a brief final response.")


@then('the real command succeeds with one agent invocation')
def real_success(context):
    outcome(context, 0, "responded")
    assert context.result["agent_invocations"] == 1, context.result
    assert context.result["direct"]["session_id"], context.result
    assert not any(k in context.result for k in ["plan", "validation", "last_review"]), context.result


@then('the generated code passes independent positive negative and zero checks')
def independent_tests(context):
    assert (context.workspace / "add.py").is_file() and (context.workspace / "test_add.py").is_file()
    independent = "from add import add\nfor a,b,want in [(2,3,5),(-2,-3,-5),(-2,3,1),(0,0,0),(2147483647,1,2147483648)]:\n assert add(a,b)==want,(a,b,want)\n"
    subprocess.run([sys.executable, "-c", independent], cwd=context.workspace, check=True, timeout=30)
    subprocess.run([sys.executable, "-m", "unittest", "discover", "-v"], cwd=context.workspace, check=True, timeout=30)


@then('the token is absent from project files')
def no_token_file(context):
    for path in context.workspace.rglob("*"):
        if path.is_file():
            assert context.token.encode() not in path.read_bytes(), str(path)


@when('I ask the same native session to recall the token without reading files')
def real_followup(context):
    context.first_session = context.result["direct"]["session_id"]
    invoke(context, "What exact conversation token did I ask you to remember? Reply with only the token. "
           "Do not inspect or change any files.", context.first_session)


@then('the exact token and session are retained')
def real_recalled(context):
    assert context.token in context.result["direct"]["text"], context.result
    assert context.result["direct"]["session_id"] == context.first_session, context.result


@given('a locally built Docker image selected for acceptance testing')
def image_selected(context):
    context.image = context.config.userdata.get("image")
    assert context.image, "@packaged requires -D image=locally-built-image"


@when('I exercise delegation and interactive setup in disposable container mounts')
def packaged(context):
    script = context.repo / "scripts/test-container-direct.sh"
    process = subprocess.run(["docker", "run", "--rm", "--user", "0", "--tmpfs", "/workspace:mode=1777",
                              "--tmpfs", "/state:mode=1777", "--mount", f"type=bind,src={script},dst=/tmp/test-direct.sh,readonly",
                              "--entrypoint", "/bin/sh", context.image, "/tmp/test-direct.sh"],
                             capture_output=True, text=True, timeout=90)
    assert process.returncode == 0, process.stdout + process.stderr
    context.packaged_output = process.stdout


@then('the packaged edit has the application user ownership')
def packaged_edit(context):
    assert "PASS: packaged default direct mode" in context.packaged_output, context.packaged_output


@then('the terminal completes three-field setup and starts a new conversation')
def packaged_terminal(context):
    assert "PASS: packaged interactive three-field setup" in context.packaged_output, context.packaged_output
