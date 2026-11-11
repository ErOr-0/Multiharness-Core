export const REPO = "https://github.com/ErOr-0/Multiharness-Core";
export const DOCS = `${REPO}/blob/main/README.md`;

// Anchors of README.md headings, so links land on the matching section.
export const docs = {
  install: `${DOCS}#install`,
  howItWorks: `${DOCS}#how-it-works`,
  commandLine: `${DOCS}#command-line`,
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

// The same jobs from a shell, for scripts and planners without MCP. start uses
// the implementer chosen in setup unless --to names another.
export const shellCommands = [
  "harness-bridge start < plan.md",
  "harness-bridge wait JOB --max 9m",
  "harness-bridge start --session ID < fixes.md",
  "harness-bridge cancel JOB",
  "harness-bridge run --to opencode < plan.md",
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
    "Claude Code, Codex CLI, OpenCode and Command Code can plan and review; setup connects the one you pick for all your projects. All five can write the code, and Muse Code only implements. Planner and implementer can be the same CLI on different models, for example Claude Code planning on Fable and writing the code on Sonnet with one subscription.",
  ],
  [
    "Does it write anything into my projects?",
    "No. Your choices are saved in `harness-bridge/config.json` in your user config folder, and harness-bridge is registered with the planner once, for all projects. Run `harness-bridge` again to change them.",
  ],
  [
    "Will setup install agent CLIs without asking?",
    "No. Setup shows its plan, including the official installer it would run (for example `npm install --global @openai/codex`), and asks one `Continue? [Y/n]` before changing anything. Missing agents then install in the background. Without a terminal it never installs unless you pass `--yes`, and the planner’s MCP tools can’t install anything.",
  ],
  [
    "Why does work run as a background job?",
    "Implementations often outlast an agent’s tool or shell timeout: Claude Code’s Bash tool stops after 10 minutes. `delegate` returns a job id at once, and the planner calls `wait` again while the status is `running`. Jobs and their logs stay in your user cache folder (or `HARNESS_BRIDGE_HOME`) for 7 days.",
  ],
  [
    "Can the planner skip a step or edit files itself?",
    "The workflow is instructions to the planner, not code, so a planner can skip a step. The bridge itself guarantees timeouts and cancellation that stop the whole process tree, a report with independent Git evidence for every job, no prompt passing through a shell, and installs only after you confirm in setup.",
  ],
  [
    "Do I need a new account or subscription?",
    "No. harness-bridge has no account or service of its own. It runs the agent CLIs you already use, each signed in with its own login, so usage follows each provider’s plan. The wizard opens a sign-in for any agent that isn’t signed in yet.",
  ],
  [
    "Can a decision model route my requests?",
    "Yes, optionally: pick Laya or Jev in setup. The planner then calls `route`, which asks whether a request is an `answer`, `needs_planning` or can go straight to `direct_implement`. Laya is self-hosted: set `LAYA_ENDPOINT`, by default `http://127.0.0.1:8765/v1/systemone`. Jev is TypeSafe's hosted model, reached through OpenRouter with your `OPENROUTER_API_KEY`. When the model is unreachable or unsure, `route` returns the safe choice, `needs_planning`.",
  ],
  [
    "Does it work on Windows?",
    "Yes. The PowerShell install line puts harness-bridge in your user programs folder and adds it to your `PATH`. It’s tested on Linux, macOS and Windows, and on each one a timeout or `cancel` stops the whole process tree.",
  ],
];
