import assert from "node:assert/strict";
import test from "node:test";
import { teamSettingsCommand } from "../src/team-settings.js";

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
