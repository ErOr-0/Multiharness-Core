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
    directSettingsCommand({
      harness: "opencode",
      model: "provider/model",
      effort: "",
    }),
    '/set mode direct\n/set implementer-harness opencode\n/set implementer-model provider/model\n/set implementer-variant ""\n/save',
  );
  assert.equal(
    directSettingsCommand({ harness: "codex", model: "", effort: "high" }),
    "",
  );
});

test("team settings use each harness's real reasoning option and save all roles", () => {
  for (const harness of ["codex", "claude", "opencode"]) {
    const model = harness === "opencode" ? "provider/model" : "model-id";
    const command = teamSettingsCommand(harness, model, "high");
    for (const role of ["planner", "implementer", "reviewer"]) {
      assert.ok(
        command.includes(
          `/set ${role}-harness ${harness}\n/set ${role}-model ${model}\n/set ${role}-${harness === "opencode" ? "variant" : "reasoning"} high`,
        ),
      );
    }
    assert.ok(command.endsWith("\n/save"));
  }
  assert.ok(
    teamSettingsCommand("opencode", "provider/model", "").includes(
      '/set planner-variant ""',
    ),
  );
});

test("mixed teams select opencode or codex independently per role", () => {
  const command = teamRolesCommand({
    planner: { harness: "opencode", model: "provider/plan", effort: "" },
    implementer: { harness: "opencode", model: "provider/build", effort: "" },
    reviewer: { harness: "codex", model: "model-id", effort: "high" },
  });
  assert.ok(command.includes("/set planner-harness opencode"));
  assert.ok(command.includes("/set implementer-harness opencode"));
  assert.ok(command.includes("/set reviewer-harness codex"));
  assert.ok(command.includes("/set planner-model provider/plan"));
  assert.ok(command.includes("/set reviewer-reasoning high"));
  assert.ok(command.endsWith("\n/save"));

  const opposite = teamRolesCommand({
    planner: { harness: "codex", model: "model-id", effort: "medium" },
    implementer: { harness: "codex", model: "model-id", effort: "medium" },
    reviewer: { harness: "opencode", model: "provider/review", effort: "" },
  });
  assert.ok(opposite.includes("/set planner-harness codex"));
  assert.ok(opposite.includes("/set reviewer-harness opencode"));
  assert.ok(opposite.includes('/set reviewer-variant ""'));

  const errors = teamRolesErrors({
    planner: { harness: "codex", model: "model-id", effort: "high" },
    implementer: { harness: "codex", model: "", effort: "high" },
    reviewer: { harness: "codex", model: "model-id", effort: "high" },
  });
  assert.equal(errors.planner, "");
  assert.ok(errors.implementer);
  assert.equal(
    teamRolesCommand({
      planner: { harness: "codex", model: "model-id", effort: "high" },
      implementer: { harness: "codex", model: "", effort: "high" },
      reviewer: { harness: "codex", model: "model-id", effort: "high" },
    }),
    "",
  );

  assert.deepEqual(
    teamLogins({
      planner: { harness: "opencode" },
      implementer: { harness: "opencode" },
      reviewer: { harness: "codex" },
    }),
    ["opencode", "codex"],
  );
});

test("invalid input cannot inject commands into copied settings", () => {
  for (const model of [
    "",
    "model\n/quit",
    "model\r/save",
    "model with spaces",
    'model"',
    "-model",
    "model\u0000bad",
  ]) {
    assert.equal(teamSettingsCommand("codex", model, "high"), "");
  }
  assert.equal(teamSettingsCommand("opencode", "model", "high"), "");
  assert.equal(teamSettingsCommand("claude", "model", "none"), "");
  assert.equal(
    teamSettingsCommand("opencode", "provider/model", "high\n/quit"),
    "",
  );
});
