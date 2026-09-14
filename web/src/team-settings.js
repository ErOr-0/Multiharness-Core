export const harnesses = {
  codex: "Codex",
  claude: "Claude Code",
  opencode: "OpenCode",
};

export const roles = ["planner", "implementer", "reviewer"];

export function reasoningOptions(harness) {
  return [
    "low",
    "medium",
    "high",
    "xhigh",
    "max",
    ...(harness === "codex" ? ["none"] : []),
  ];
}

export function teamError(harness, model, effort) {
  if (!Object.hasOwn(harnesses, harness)) return "Choose an agent harness.";
  if (!model) return "Enter a model ID from your provider account.";
  if (!/^[a-zA-Z0-9][a-zA-Z0-9._:/@+-]*$/.test(model))
    return "Use one model ID without spaces or quotes.";
  if (harness === "opencode") {
    if (!model.includes("/"))
      return "Use the provider/model format for OpenCode.";
    if (effort && !/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/.test(effort))
      return "Use a variant name without spaces or quotes.";
  } else if (!reasoningOptions(harness).includes(effort)) {
    return "Choose a reasoning level supported by your model.";
  }
  return "";
}

// One role's three /set lines. Empty when that role is invalid, so a mixed
// team never copies a partial or injectable command.
export function roleSettingCommand(role, harness, model, effort) {
  if (!roles.includes(role) || teamError(harness, model, effort)) return [];
  return [
    `/set ${role}-harness ${harness}`,
    `/set ${role}-model ${model}`,
    `/set ${role}-${harness === "opencode" ? "variant" : "reasoning"} ${effort || '""'}`,
  ];
}

// Per-role validation errors, keyed by role. Empty string means that role is ready.
export function teamRolesErrors(team) {
  const errors = {};
  for (const role of roles) {
    const selection = team?.[role] ?? {};
    errors[role] = teamError(
      selection.harness,
      selection.model,
      selection.effort,
    );
  }
  return errors;
}

// Distinct harness logins needed for the current team, e.g. ["codex", "opencode"].
export function teamLogins(team) {
  const logins = [];
  for (const role of roles) {
    const harness = team?.[role]?.harness;
    if (Object.hasOwn(harnesses, harness) && !logins.includes(harness)) {
      logins.push(harness);
    }
  }
  return logins;
}

// These are Multiharness prompt commands, never shell commands or model calls.
export function teamRolesCommand(team) {
  const lines = roles.flatMap((role) => {
    const selection = team?.[role] ?? {};
    return roleSettingCommand(
      role,
      selection.harness,
      selection.model,
      selection.effort,
    );
  });
  if (lines.length !== roles.length * 3) return "";
  return [...lines, "/save"].join("\n");
}

// Single-harness shortcut: same agent, model and effort for every role.
export function teamSettingsCommand(harness, model, effort) {
  if (teamError(harness, model, effort)) return "";
  return teamRolesCommand(
    Object.fromEntries(roles.map((role) => [role, { harness, model, effort }])),
  );
}
