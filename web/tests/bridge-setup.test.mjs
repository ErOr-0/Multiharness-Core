import assert from "node:assert/strict";
import test from "node:test";
import {
  agents,
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

test("the wizard connects Claude Code as the planner for all projects", () => {
  assert.deepEqual(
    wizardPlanners.map((agent) => agent.name),
    ["claude"],
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
  assert.equal(
    setupCommand({ planner: "claude", implementer: "muse", router: "laya" }),
    "harness-bridge setup --planner claude --implementer muse --router laya",
  );
});

test("other planners get instruction files in one project", () => {
  for (const planner of agents.filter((agent) => !agent.mcpPlanner))
    for (const implementer of agents) {
      assert.equal(
        setupCommand({ planner: planner.name, implementer: implementer.name }),
        `harness-bridge setup --planner ${planner.name} --implementer ${implementer.name} --dir path/to/project`,
      );
    }
  assert.equal(
    setupCommand({ planner: "codex", implementer: "claude", router: "jev" }),
    "harness-bridge setup --planner codex --implementer claude --router jev --dir path/to/project",
  );
});

test("setup command is empty for choices setup itself rejects", () => {
  const rejected = [
    { planner: "claude", implementer: "claude" },
    { planner: "codex", implementer: "claude", router: "other" },
    { planner: "nope", implementer: "codex" },
    { planner: "codex", implementer: "codex; rm -rf ~" },
  ];
  for (const choice of rejected) {
    assert.notEqual(setupError(choice), "", JSON.stringify(choice));
    assert.equal(setupCommand(choice), "", JSON.stringify(choice));
  }
  assert.match(
    setupError({ planner: "claude", implementer: "claude" }),
    /both be Claude Code/,
  );
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
