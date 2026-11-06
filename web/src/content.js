export const REPO = "https://github.com/ErOr-0/Multiharness-Core";
export const DOCS = `${REPO}/blob/main/README.md`;
export const LEGACY_TAG = `${REPO}/tree/magent-final`;

// Anchors of README.md headings, so links land on the matching section.
export const docs = {
  install: `${DOCS}#install`,
  jobs: `${DOCS}#how-a-job-runs`,
  routing: `${DOCS}#routing-with-jev-or-laya`,
  agents: `${DOCS}#supported-agents`,
};

export const workflowSteps = [
  {
    number: "01",
    label: "Plan",
    title: "A plan the implementer can follow.",
    copy: "The planner reads what it needs, then writes the goal, the files to change, the steps, the checks to run and what done looks like.",
    file: "plan.md",
    badge: "Planner",
    lines: [
      "# Add a health-check endpoint",
      "Goal: GET /health returns 200 and {status: ok}",
      "Files: server.go, health_test.go",
      "Checks: go test ./...",
      "",
      "When you finish, reply with what you changed,",
      "the checks you ran and their results, and",
      "anything you could not do.",
    ],
  },
  {
    number: "02",
    label: "Delegate",
    title: "One tool call hands it over.",
    copy: "The planner calls harness-bridge’s `delegate` tool with the plan and the project folder. harness-bridge runs the implementer’s CLI headless and returns a job id at once, so a long job never hits a tool timeout.",
    file: "delegate",
    badge: "harness-bridge",
    lines: [
      "→ delegate",
      '    task: "# Add a health-check endpoint …"',
      '    directory: "/home/you/api"',
      "← {",
      '    "job": "20261106-101500-a1b2c3",',
      '    "harness": "codex",',
      '    "status": "running"',
      "  }",
    ],
  },
  {
    number: "03",
    label: "Report",
    title: "One JSON report back.",
    copy: "`wait` follows the CLI’s event stream and returns the final report: status, session id, the agent’s message, files, commands with exit codes, and what Git sees afterwards.",
    file: "report.json",
    badge: "harness-bridge",
    lines: [
      "{",
      '  "status": "completed",',
      '  "session_id": "019a3f2c",',
      '  "message": "Added GET /health and a test.",',
      '  "files_changed": ["server.go", "health_test.go"],',
      '  "commands": [',
      '    {"command": "go test ./...", "exit_code": 0}',
      "  ],",
      '  "git": {',
      '    "dirty_before": 0,',
      '    "status": [" M server.go", "?? health_test.go"]',
      "  }",
      "}",
    ],
  },
  {
    number: "04",
    label: "Review & repair",
    title: "Review the repository, not the report.",
    copy: "The planner runs `git diff` and the checks itself and compares them with the plan. Fixes go back through `delegate` with the `session_id`, so the implementer continues the same session. After 3 rounds it reports what is still open.",
    file: "terminal",
    badge: "Planner",
    lines: [
      "$ git diff --stat",
      " server.go      | 12 ++++++++++++",
      " health_test.go | 24 ++++++++++++++++++++++++",
      "$ go test ./...",
      "ok   example/api   0.024s",
      "",
      '→ delegate  task: "Also handle HEAD requests"',
      '            session_id: "019a3f2c"',
    ],
  },
];

// The MCP tools setup gives the planner.
export const mcpTools = [
  ["delegate", "Hand a plan to the implementer. Returns a job id at once."],
  ["wait", "Progress while the job runs, the report when it ends."],
  ["cancel", "Stop a job. Edits it already made stay in the workspace."],
  ["route", "Ask Jev or Laya whether a request needs a plan."],
  ["agents", "Which agent CLIs are installed, and the roles you chose."],
];

// The same jobs from a shell, for scripts and planners without MCP.
export const shellCommands = [
  "harness-bridge start --to codex < plan.md",
  "harness-bridge wait JOB --max 9m",
  "harness-bridge start --to codex --session ID < fixes.md",
  "harness-bridge cancel JOB",
  "harness-bridge run --to codex < plan.md",
];

export const jobStatuses = [
  "running",
  "completed",
  "failed",
  "needs_permission",
  "timed_out",
  "cancelled",
  "lost",
];

export const reportFields = [
  ["status", "Where the job stands."],
  ["session_id", "For sending fixes to the same session."],
  ["message", "The agent’s final answer."],
  ["files_changed · commands", "As the agent reported them, with exit codes."],
  ["permission_denied", "Actions the CLI refused to take."],
  [
    "git",
    "`git status` and `git diff --stat` afterwards; `dirty_before` counts changes that were already there.",
  ],
  ["recent · next", "Latest activity while running, and what to call next."],
];

export const faqs = [
  [
    "What is harness-bridge, exactly?",
    "A small Go program that lets one coding agent hand work to a different agent CLI and read back what happened. There is no orchestrator, terminal UI or container. Setup connects it to the planner as an MCP server, so the planner gets `delegate`, `wait`, `cancel`, `route` and `agents` tools, and its own agent loop is the workflow.",
  ],
  [
    "Do I need Go?",
    "No. The install line downloads a prebuilt harness-bridge for your system, checks its SHA-256 checksum and starts setup. Go is only needed to build from source with `go install ./cmd/harness-bridge`.",
  ],
  [
    "Which agent should plan, and which should implement?",
    "The setup wizard connects Claude Code as the planner for all your projects, and any of the other four writes the code. Other agents can plan one project at a time: `harness-bridge setup --planner codex --implementer claude --dir path/to/project` writes the instructions into that project’s `AGENTS.md`. Planner and implementer can’t both be Claude Code, because Claude Code refuses to start inside another Claude Code session. Use its own subagents for that.",
  ],
  [
    "Does it write anything into my projects?",
    "No. Your choices are saved once in your user config folder (`harness-bridge/config.json`), and harness-bridge is registered with the planner for every project. Only `setup --dir` writes instruction files into a project, for planners that can’t load MCP servers.",
  ],
  [
    "Will setup install agent CLIs without asking?",
    "No. The wizard lists what it will do, including the official installer it would run (for example `npm install --global @openai/codex`), and asks once before changing anything. Missing CLIs then install in the background. Without a terminal it never installs, so an agent running `setup` has to ask you first and then pass `--yes`.",
  ],
  [
    "Why does work run as a background job?",
    "Implementations often outlast an agent’s tool or shell timeout: Claude Code’s Bash tool stops after 10 minutes. `delegate` returns a job id at once, and the planner calls `wait` again while the status is `running`. Jobs stay in your user cache directory (or `HARNESS_BRIDGE_HOME`) for 7 days, with the raw event log.",
  ],
  [
    "Can the planner skip a step or edit files itself?",
    "The workflow is instructions to the planner, not code, so a planner can skip a step. The bridge guarantees timeouts and cancellation that stop the whole process tree, no prompt passing through a shell, and independent Git evidence in every report. With `setup --dir` and a Claude Code planner, `--lock-planner` denies its `Edit`, `Write` and `NotebookEdit` tools, so every change goes through the implementer.",
  ],
  [
    "Do I need a new account or subscription?",
    "No. harness-bridge has no account or service of its own. It runs the agent CLIs you already use, each signed in with its own login, so usage follows each provider’s plan. The wizard opens a sign-in for any agent that isn’t signed in yet.",
  ],
  [
    "Can a decision model route my requests?",
    "Yes, optionally: pick Laya or Jev in the wizard. The planner then calls `route`, which asks whether a request is an `answer`, `needs_planning` or can go straight to `direct_implement`. Laya is self-hosted: set `LAYA_ENDPOINT`, by default `http://127.0.0.1:8765/v1/systemone`. Jev is TypeSafe's hosted model, reached through OpenRouter with your `OPENROUTER_API_KEY`. When the model is unreachable or unsure, `route` returns the safe choice, `needs_planning`.",
  ],
  [
    "Does it work on Windows?",
    "Yes. The PowerShell install line puts harness-bridge in your user programs folder and adds it to your `PATH`. It’s tested on Linux, macOS and Windows, and on each one a timeout or `cancel` stops the whole process tree.",
  ],
];
