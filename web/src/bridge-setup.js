// The agent CLIs harness-bridge can drive, as listed in README.md under
// "Supported agents". Each one is an adapter in cmd/harness-bridge.
export const agents = [
  {
    name: "codex",
    title: "Codex CLI",
    login: "codex login",
    resume: true,
    note: "Sandboxed: commands have no network unless you allow it.",
  },
  {
    name: "claude",
    title: "Claude Code",
    login: "claude auth login",
    resume: true,
    note: "Shell commands only through `--allow 'Bash(go test:*)'`.",
  },
  {
    name: "muse",
    title: "Muse Code",
    login: "muse login",
    resume: false,
    note: "No shell headless, so the planner runs the checks.",
  },
  {
    name: "opencode",
    title: "OpenCode",
    login: "opencode auth login",
    resume: true,
    note: "Rejects permission prompts headless; see `opencode.json` or `-- --auto`.",
  },
  {
    name: "commandcode",
    title: "Command Code",
    login: "command-code login",
    resume: true,
    note: "Accepts edits; no per-command allow list.",
  },
];

export const routers = [
  ["", "No routing"],
  ["laya", "Laya (self-hosted)"],
  ["jev", "Jev (hosted)"],
];

export const installCommand = [
  "git clone https://github.com/ErOr-0/Multiharness-Core.git",
  "cd Multiharness-Core",
  "go install ./cmd/harness-bridge",
].join("\n");

export function agentNamed(name) {
  return agents.find((agent) => agent.name === name);
}

// The same rules `harness-bridge setup` applies before it writes anything.
export function setupError({
  planner,
  implementer,
  router = "",
  lockPlanner = false,
}) {
  if (!agentNamed(planner)) return "Choose a planner.";
  if (!agentNamed(implementer)) return "Choose an implementer.";
  if (planner === "claude" && implementer === "claude")
    return "Planner and implementer can’t both be Claude Code: it refuses to start inside another Claude Code session. Use its own subagents for that.";
  if (!routers.some(([value]) => value === router))
    return "Route with jev or laya.";
  if (lockPlanner && planner !== "claude")
    return "--lock-planner works only with a Claude Code planner.";
  return "";
}

// Every value comes from the fixed lists above, so nothing needs quoting and
// the command reads the same in any shell. Empty when the choice is invalid.
export function setupCommand(choice) {
  if (setupError(choice)) return "";
  const { planner, implementer, router = "", lockPlanner = false } = choice;
  const parts = [
    "harness-bridge setup",
    `--planner ${planner}`,
    `--implementer ${implementer}`,
  ];
  if (lockPlanner) parts.push("--lock-planner");
  if (router) parts.push(`--router ${router}`);
  return parts.join(" ");
}

// The files setup writes in the project for this pairing.
export function setupFiles({ planner, implementer, lockPlanner = false }) {
  if (planner !== "claude") return ["AGENTS.md"];
  const files = [`.claude/agents/${implementer}-implementer.md`, "CLAUDE.md"];
  if (lockPlanner) files.push(".claude/settings.json");
  return files;
}

// One sign-in per distinct CLI.
export function signInCommand({ planner, implementer }) {
  const names = [...new Set([planner, implementer])];
  return names
    .map((name) => agentNamed(name)?.login)
    .filter(Boolean)
    .join("\n");
}
