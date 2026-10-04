export const harnesses = {
  codex: "Codex",
  claude: "Claude Code",
  muse: "Muse Code",
};

export const roles = ["planner", "implementer", "reviewer"];

export function reasoningOptions(harness) {
  if (harness === "muse")
    return [
      "low",
      "medium",
      "high",
      "xhigh",
      "max",
      "ultra",
      "minimal",
      "none",
    ];
  return [
    "low",
    "medium",
    "high",
    "xhigh",
    "max",
    ...(harness === "codex" ? ["none"] : []),
  ];
}

export function teamError(harness, effort) {
  if (!Object.hasOwn(harnesses, harness)) return "Choose an agent harness.";
  if (!reasoningOptions(harness).includes(effort)) {
    return "Choose a reasoning level.";
  }
  return "";
}

// One role's /set lines: harness plus reasoning. The model is picked in the
// app via /config. Empty when that role is invalid, so a mixed team never
// copies a partial or injectable command.
export function roleSettingCommand(role, harness, effort) {
  if (!roles.includes(role) || teamError(harness, effort)) return [];
  return [
    `/set ${role}-harness ${harness}`,
    `/set ${role}-reasoning ${effort}`,
  ];
}

// Per-role validation errors, keyed by role. Empty string means that role is ready.
export function teamRolesErrors(team) {
  const errors = {};
  for (const role of roles) {
    const selection = team?.[role] ?? {};
    errors[role] = teamError(selection.harness, selection.effort);
  }
  return errors;
}

// Distinct harness logins needed for the current team, e.g. ["codex", "claude"].
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
    return roleSettingCommand(role, selection.harness, selection.effort);
  });
  if (lines.length !== roles.length * 2) return "";
  return ["/set mode team", ...lines, "/save"].join("\n");
}

// Single-harness shortcut: same harness and reasoning for every role.
export function teamSettingsCommand(harness, effort) {
  if (teamError(harness, effort)) return "";
  return teamRolesCommand(
    Object.fromEntries(roles.map((role) => [role, { harness, effort }])),
  );
}

export function directSettingsCommand(selection) {
  const lines = roleSettingCommand(
    "implementer",
    selection.harness,
    selection.effort,
  );
  return lines.length === 2
    ? ["/set mode direct", ...lines, "/save"].join("\n")
    : "";
}
