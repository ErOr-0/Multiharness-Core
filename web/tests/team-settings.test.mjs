import assert from "node:assert/strict";
import test from "node:test";
import {
  directSettingsCommand,
  teamLogins,
  teamRolesCommand,
  teamRolesErrors,
  teamSettingsCommand,
} from "../src/team-settings.js";

test("direct settings configure only one agent and reject incomplete input", () => {
  assert.equal(
    directSettingsCommand({ harness: "claude", effort: "high" }),
    "/set mode direct\n/set implementer-harness claude\n/set implementer-reasoning high\n/save",
  );
  assert.equal(
    directSettingsCommand({ harness: "codex", effort: "high" }),
    "/set mode direct\n/set implementer-harness codex\n/set implementer-reasoning high\n/save",
  );
  assert.equal(directSettingsCommand({ harness: "codex", effort: "" }), "");
  assert.equal(directSettingsCommand({ harness: "nope", effort: "high" }), "");
});

test("team settings use each harness's real reasoning option and save all roles", () => {
  for (const harness of ["codex", "claude", "muse"]) {
    const command = teamSettingsCommand(harness, "high");
    for (const role of ["planner", "implementer", "reviewer"]) {
      assert.ok(
        command.includes(
          `/set ${role}-harness ${harness}\n/set ${role}-reasoning high`,
        ),
      );
    }
    assert.ok(!command.includes("-model"));
    assert.ok(!command.includes("-variant"));
    assert.ok(command.endsWith("\n/save"));
  }
  assert.equal(teamSettingsCommand("opencode", ""), "");
});

test("mixed teams select each harness independently per role", () => {
  const command = teamRolesCommand({
    planner: { harness: "muse", effort: "ultra" },
    implementer: { harness: "claude", effort: "medium" },
    reviewer: { harness: "codex", effort: "high" },
  });
  assert.ok(command.includes("/set planner-harness muse"));
  assert.ok(command.includes("/set planner-reasoning ultra"));
  assert.ok(command.includes("/set implementer-harness claude"));
  assert.ok(command.includes("/set reviewer-harness codex"));
  assert.ok(!command.includes("-model"));
  assert.ok(!command.includes("-variant"));
  assert.ok(command.includes("/set reviewer-reasoning high"));
  assert.ok(command.endsWith("\n/save"));

  const opposite = teamRolesCommand({
    planner: { harness: "codex", effort: "medium" },
    implementer: { harness: "codex", effort: "medium" },
    reviewer: { harness: "claude", effort: "low" },
  });
  assert.ok(opposite.includes("/set planner-harness codex"));
  assert.ok(opposite.includes("/set reviewer-harness claude"));
  assert.ok(opposite.includes("/set planner-reasoning medium"));

  const errors = teamRolesErrors({
    planner: { harness: "codex", effort: "high" },
    implementer: { harness: "codex", effort: "" },
    reviewer: { harness: "codex", effort: "high" },
  });
  assert.equal(errors.planner, "");
  assert.ok(errors.implementer);
  assert.equal(
    teamRolesCommand({
      planner: { harness: "codex", effort: "high" },
      implementer: { harness: "codex", effort: "" },
      reviewer: { harness: "codex", effort: "high" },
    }),
    "",
  );

  assert.deepEqual(
    teamLogins({
      planner: { harness: "claude" },
      implementer: { harness: "claude" },
      reviewer: { harness: "codex" },
    }),
    ["claude", "codex"],
  );
});

test("invalid input cannot inject commands into copied settings", () => {
  for (const effort of [
    "high\n/quit",
    "high\r/save",
    "high with spaces",
    'high"',
    "high\u0000bad",
  ]) {
    assert.equal(teamSettingsCommand("codex", effort), "");
  }
  assert.equal(teamSettingsCommand("codex", "bogus-level"), "");
  assert.equal(teamSettingsCommand("nope", "high"), "");
  assert.equal(teamSettingsCommand("claude", "none"), "");
  assert.equal(teamSettingsCommand("codex\nharness", "high"), "");
  assert.equal(teamSettingsCommand("muse", "high\n/quit"), "");
});
