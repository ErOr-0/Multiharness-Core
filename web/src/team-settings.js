export const harnesses = {
  codex: "Codex",
  claude: "Claude Code",
  opencode: "OpenCode",
};

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
  } else if (
    ![
      "low",
      "medium",
      "high",
      "xhigh",
      "max",
      ...(harness === "codex" ? ["none"] : []),
    ].includes(effort)
  ) {
    return "Choose a reasoning level supported by your model.";
  }
  return "";
}

// These are Multiharness prompt commands, never shell commands or model calls.
export function teamSettingsCommand(harness, model, effort) {
  if (teamError(harness, model, effort)) return "";
  return [
    ...["planner", "implementer", "reviewer"].flatMap((role) => [
      `/set ${role}-harness ${harness}`,
      `/set ${role}-model ${model}`,
      `/set ${role}-${harness === "opencode" ? "variant" : "reasoning"} ${effort || '""'}`,
    ]),
    "/save",
  ].join("\n");
}
