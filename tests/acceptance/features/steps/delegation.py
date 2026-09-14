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


@given('the configured OpenCode permission mode is "{policy}"')
def permission_mode(context, policy):
    context.settings["implementer"]["permission_policy"] = policy
    save_config(context)


@then('the native auto-approve flag is "{state}"')
def native_permission_flag(context, state):
    recorded = calls(context)
    assert len(recorded) == 1, recorded
    assert ("--auto" in recorded[0]["args"]) == (state == "present"), recorded


@given('the provider will "{behavior}"')
@then('the provider will "{behavior}"')
def behavior(context, behavior):
    context.env["BDD_BEHAVIOR"] = behavior


@given('the provider replays the recorded native OpenCode permission denial')
def replay_denial(context):
    context.env["BDD_BEHAVIOR"] = "permission-replay"
    context.env["BDD_REPLAY"] = str(context.repo / "internal/adapter/agent/directexec/testdata/opencode-permission-denied.jsonl")


@then('the result identifies the blocked read and preserves the conversation')
def blocked_read(context):
    direct = context.result["direct"]
    assert direct["needs_input"] and direct["session_id"] and direct["text"], direct
    assert direct["blocked_action"] == {"tool": "read", "target": "/fixtures/outside.txt"}, direct
    assert "/fixtures/outside.txt" in context.result["summary"], context.result
    assert "continue without that action" in context.result["summary"], context.result
    assert "failure" not in context.result, context.result


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


@given('a real OpenCode provider and a harmless file outside the selected project')
def live_permission_config(context):
    live_config(context)
    settings = json.loads(context.config_path.read_text())
    assert settings.get("implementer", {}).get("harness", "opencode") == "opencode"
    assert settings.get("implementer", {}).get("permission_policy", "reject_on_prompt") == "reject_on_prompt"
    context.outside = context.root / "outside.txt"
    context.outside.write_text("public acceptance fixture", encoding="utf-8")


@when('I ask the native agent to read that outside file using only its read tool')
def request_denied_read(context):
    invoke(context, f"Use only the read tool to read {context.outside}. Do not use bash or other tools. "
           "Then write completed.txt with that content. If read permission is rejected, stop immediately.")


@then('the denied native read identifies the synthetic file and keeps the session')
def native_denial(context):
    direct = context.result["direct"]
    assert direct["blocked_action"] == {"tool": "read", "target": str(context.outside)}, direct
    assert direct["session_id"] and direct["needs_input"], direct
    assert not (context.workspace / "completed.txt").exists()
    context.first_session = direct["session_id"]


@when('I tell the same session to skip that file and write only inside the project')
def continue_in_project(context):
    invoke(context, "Skip the outside file entirely. Do not read it or any parent instruction file. "
           "Create completed.txt in the current project with exactly the text: continued safely. "
           "Only use tools within the current project. Then give a brief final response.", context.first_session)


@then('the native agent writes the requested file without changing permissions')
def native_recovery(context):
    assert (context.workspace / "completed.txt").read_text().strip() == "continued safely"
    assert context.result["direct"]["session_id"] == context.first_session
    assert "--implementer-permission-policy" not in context.overrides
    assert context.outside.read_text() == "public acceptance fixture"


@when('I ask for a minimal Genkit Go scaffold using current documentation')
def genkit_task(context):
    invoke(context, "Can you add a bare minimum architecture of Google's Genkit using Go here? "
           "Fetch the current public Genkit Go documentation before implementing. Create a small "
           "modular project with a runnable entry point and a separate package defining one "
           "deterministic input/output flow, plus a Go test. Use the real Genkit Go SDK. "
           "No model backend or API credentials should be necessary. Keep all file reads/writes "
           "inside the current project; do not read parent instruction files. Run go test ./... "
           "and go build ./... before finishing.")


@then('the generated project imports Genkit and passes independent Go checks')
def genkit_checks(context):
    module = context.workspace / "go.mod"
    assert module.is_file(), "Agent did not create go.mod"
    assert "github.com/firebase/genkit/go" in module.read_text(), module.read_text()
    sources = list(context.workspace.rglob("*.go"))
    assert any(p.name.endswith("_test.go") for p in sources), "No executable Go test was generated"
    assert any('"github.com/firebase/genkit/go/genkit"' in p.read_text() for p in sources), "No Genkit SDK import"
    subprocess.run(["go", "test", "-count=1", "./..."], cwd=context.workspace, check=True, timeout=180)
    subprocess.run(["go", "build", "./..."], cwd=context.workspace, check=True, timeout=180)


@when('I change OpenCode permissions through the real interactive terminal')
def permissions_terminal(context):
    ensure_binary(context)
    process = subprocess.run([sys.executable, str(context.repo / "tests/acceptance/terminal_permissions.py"),
                              "--binary", context.binary, "--config", str(context.config_path)],
                             capture_output=True, text=True, timeout=660)
    assert process.returncode == 0, process.stdout + process.stderr
    context.permissions_terminal_result = process.stdout


@then('the native CLI grants and revokes access in the same conversation')
def terminal_granted_and_revoked(context):
    assert "PASS: real terminal denied -> /permissions auto" in context.permissions_terminal_result


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
