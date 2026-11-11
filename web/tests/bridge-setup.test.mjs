import assert from "node:assert/strict";
import test from "node:test";
import {
  agents,
  implementersFor,
  platforms,
  setupCommand,
  setupError,
  signInCommand,
  sourceInstallCommand,
  wizardPlanners,
} from "../src/bridge-setup.js";

test("the five supported agents use their --to names", () => {
  assert.deepEqual(
    agents.map((agent) => agent.name),
    ["codex", "claude", "muse", "opencode", "commandcode"],
  );
  assert.deepEqual(
    agents.filter((agent) => !agent.resume).map((agent) => agent.name),
    ["muse"],
  );
});

test("four agents can plan; Muse Code only implements", () => {
  assert.deepEqual(
    wizardPlanners.map((agent) => agent.name),
    ["codex", "claude", "opencode", "commandcode"],
  );
});

test("every agent implements for every planner, itself included", () => {
  assert.deepEqual(
    implementersFor("claude").map((agent) => agent.name),
    ["codex", "claude", "muse", "opencode", "commandcode"],
  );
  for (const planner of ["codex", "claude", "opencode", "commandcode"])
    assert.equal(implementersFor(planner).length, 5, planner);
  assert.equal(setupError({ planner: "claude", implementer: "claude" }), "");
  assert.equal(
    setupCommand({ planner: "claude", implementer: "claude" }),
    "harness-bridge setup --planner claude --implementer claude",
  );
});

test("one install line per platform, from the main branch installers", () => {
  const raw = "https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/main";
  assert.deepEqual(
    Object.fromEntries(platforms.map((item) => [item.id, item.install])),
    {
      unix: `curl -fsSL ${raw}/install.sh | sh`,
      windows: `irm ${raw}/install.ps1 | iex`,
    },
  );
  for (const item of platforms)
    assert.doesNotMatch(item.install, /releases|go install/);
});

test("setup flags connect an MCP planner for every project", () => {
  assert.equal(
    setupCommand({ planner: "claude", implementer: "codex" }),
    "harness-bridge setup --planner claude --implementer codex",
  );
  for (const planner of wizardPlanners)
    for (const implementer of implementersFor(planner.name))
      assert.equal(
        setupCommand({ planner: planner.name, implementer: implementer.name }),
        `harness-bridge setup --planner ${planner.name} --implementer ${implementer.name}`,
      );
  assert.equal(
    setupCommand({ planner: "claude", implementer: "muse", router: "laya" }),
    "harness-bridge setup --planner claude --implementer muse --router laya",
  );
});

test("a planner without MCP needs the advanced --dir form", () => {
  assert.equal(
    setupCommand({ planner: "muse", implementer: "codex", router: "jev" }),
    "harness-bridge setup --planner muse --implementer codex --router jev --dir path/to/project",
  );
  assert.equal(
    setupCommand({ planner: "codex", implementer: "claude", router: "jev" }),
    "harness-bridge setup --planner codex --implementer claude --router jev",
  );
});

test("setup command is empty for choices setup itself rejects", () => {
  const rejected = [
    { planner: "codex", implementer: "claude", router: "other" },
    { planner: "nope", implementer: "codex" },
    { planner: "codex", implementer: "codex; rm -rf ~" },
  ];
  for (const choice of rejected) {
    assert.notEqual(setupError(choice), "", JSON.stringify(choice));
    assert.equal(setupCommand(choice), "", JSON.stringify(choice));
  }
});

test("sign-in lists each distinct CLI once", () => {
  assert.equal(
    signInCommand({ planner: "claude", implementer: "codex" }),
    "claude auth login\ncodex login",
  );
  assert.equal(
    signInCommand({ planner: "commandcode", implementer: "commandcode" }),
    "command-code login",
  );
});

test("building from source ends by starting the wizard", () => {
  assert.equal(
    sourceInstallCommand,
    "git clone https://github.com/ErOr-0/Multiharness-Core.git\ncd Multiharness-Core\ngo install ./cmd/harness-bridge\nharness-bridge",
  );
});
