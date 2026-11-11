// The agent CLIs harness-bridge can drive, as listed in README.md under
// "Supported agents". Each one is an adapter in internal/agent.
// mcpPlanner: the agent can load harness-bridge as an MCP server, so setup
// can connect it as the planner for every project at once. Muse Code only
// implements.
export const agents = [
  {
    name: "codex",
    title: "Codex CLI",
    login: "codex login",
    resume: true,
    mcpPlanner: true,
    note: "Commands have no network unless you allow it.",
  },
  {
    name: "claude",
    title: "Claude Code",
    login: "claude auth login",
    resume: true,
    mcpPlanner: true,
    note: "Gets its instructions through `~/.claude/CLAUDE.md`. Runs shell commands only through `allow` rules such as `Bash(go test:*)`.",
  },
  {
    name: "muse",
    title: "Muse Code",
    login: "muse login",
    resume: false,
    mcpPlanner: false,
    note: "No shell headless, so the planner runs the checks.",
  },
  {
    name: "opencode",
    title: "OpenCode",
    login: "opencode auth login",
    resume: true,
    mcpPlanner: true,
    note: "v1 and v2. Headless it rejects permission prompts; see `opencode.json` or `--auto`.",
  },
  {
    name: "commandcode",
    title: "Command Code",
    login: "command-code login",
    resume: true,
    mcpPlanner: true,
    note: "Gets its instructions through `~/.commandcode/AGENTS.md`.",
  },
];

export const routers = [
  ["", "No routing"],
  ["laya", "Laya (self-hosted)"],
  ["jev", "Jev (hosted)"],
];

const RAW = "https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/main";

// One line per platform: download the prebuilt binary, check its checksum,
// then start the setup wizard. No Go needed.
export const platforms = [
  {
    id: "unix",
    label: "macOS & Linux",
    shell: "Terminal",
    install: `curl -fsSL ${RAW}/install.sh | sh`,
    note: "Installs to `~/.local/bin`.",
  },
  {
    id: "windows",
    label: "Windows",
    shell: "PowerShell",
    install: `irm ${RAW}/install.ps1 | iex`,
    note: "Installs to your user programs folder and adds it to your `PATH`.",
  },
];

export const sourceInstallCommand = [
  "git clone https://github.com/ErOr-0/Multiharness-Core.git",
  "cd Multiharness-Core",
  "go install ./cmd/harness-bridge",
  "harness-bridge",
].join("\n");

// Best guess from the browser: "windows" when the user agent says Windows,
// "unix" otherwise (and when the user agent can't be read at all).
export function detectPlatform() {
  try {
    return /Windows/i.test(navigator.userAgent) ? "windows" : "unix";
  } catch {
    return "unix";
  }
}

export function agentNamed(name) {
  return agents.find((agent) => agent.name === name);
}

// The planners the setup wizard offers, and the implementers it offers for a
// planner: any agent, except that Claude Code can't implement for itself.
export const wizardPlanners = agents.filter((agent) => agent.mcpPlanner);

export function implementersFor(planner) {
  return agents.filter(
    (agent) => !(planner === "claude" && agent.name === "claude"),
  );
}

// The same rules `harness-bridge setup` applies before it changes anything.
export function setupError({ planner, implementer, router = "" }) {
  if (!agentNamed(planner)) return "Choose a planner.";
  if (!agentNamed(implementer)) return "Choose an implementer.";
  if (planner === "claude" && implementer === "claude")
    return "Planner and implementer can’t both be Claude Code: it refuses to start inside another Claude Code session. Use its own subagents for that.";
  if (!routers.some(([value]) => value === router))
    return "Route with jev or laya.";
  return "";
}

// The non-interactive form of the wizard. Every value comes from the fixed
// lists above, so nothing needs quoting and the command reads the same in any
// shell. A planner that can't load MCP servers would get instruction files in
// one project instead (the advanced --dir form), so it needs --dir. Empty when
// the choice is invalid.
export function setupCommand(choice) {
  if (setupError(choice)) return "";
  const { planner, implementer, router = "" } = choice;
  const parts = [
    "harness-bridge setup",
    `--planner ${planner}`,
    `--implementer ${implementer}`,
  ];
  if (router) parts.push(`--router ${router}`);
  if (!agentNamed(planner).mcpPlanner) parts.push("--dir path/to/project");
  return parts.join(" ");
}

// One sign-in per distinct CLI, for the non-interactive form; the wizard opens
// these itself.
export function signInCommand({ planner, implementer }) {
  const names = [...new Set([planner, implementer])];
  return names
    .map((name) => agentNamed(name)?.login)
    .filter(Boolean)
    .join("\n");
}
