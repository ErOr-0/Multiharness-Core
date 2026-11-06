import assert from "node:assert/strict";
import test from "node:test";
import {
  agents,
  installCommand,
  setupCommand,
  setupError,
  setupFiles,
  signInCommand,
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

test("setup command names the chosen planner and implementer", () => {
  assert.equal(
    setupCommand({ planner: "claude", implementer: "codex" }),
    "harness-bridge setup --planner claude --implementer codex",
  );
  assert.equal(
    setupCommand({ planner: "codex", implementer: "claude" }),
    "harness-bridge setup --planner codex --implementer claude",
  );
  for (const planner of agents)
    for (const implementer of agents) {
      const choice = { planner: planner.name, implementer: implementer.name };
      if (planner.name === "claude" && implementer.name === "claude") continue;
      assert.equal(
        setupCommand(choice),
        `harness-bridge setup --planner ${planner.name} --implementer ${implementer.name}`,
      );
    }
});

test("setup command adds --lock-planner and --router only when chosen", () => {
  assert.equal(
    setupCommand({
      planner: "claude",
      implementer: "muse",
      lockPlanner: true,
      router: "laya",
    }),
    "harness-bridge setup --planner claude --implementer muse --lock-planner --router laya",
  );
  assert.equal(
    setupCommand({ planner: "opencode", implementer: "codex", router: "jev" }),
    "harness-bridge setup --planner opencode --implementer codex --router jev",
  );
});

test("setup command is empty for choices setup itself rejects", () => {
  const rejected = [
    { planner: "claude", implementer: "claude" },
    { planner: "codex", implementer: "codex", lockPlanner: true },
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

test("setup writes CLAUDE.md and a relay subagent for a Claude Code planner", () => {
  assert.deepEqual(setupFiles({ planner: "claude", implementer: "codex" }), [
    ".claude/agents/codex-implementer.md",
    "CLAUDE.md",
  ]);
  assert.deepEqual(
    setupFiles({ planner: "claude", implementer: "muse", lockPlanner: true }),
    [
      ".claude/agents/muse-implementer.md",
      "CLAUDE.md",
      ".claude/settings.json",
    ],
  );
  assert.deepEqual(setupFiles({ planner: "codex", implementer: "claude" }), [
    "AGENTS.md",
  ]);
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

test("install builds harness-bridge from a checkout with go install", () => {
  assert.equal(
    installCommand,
    "git clone https://github.com/ErOr-0/Multiharness-Core.git\ncd Multiharness-Core\ngo install ./cmd/harness-bridge",
  );
});
