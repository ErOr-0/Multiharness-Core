export const REPO = "https://github.com/ErOr-0/Multiharness-Core";
export const DOCS = `${REPO}/blob/main/README.md`;
export const LEGACY_TAG = `${REPO}/tree/magent-final`;

// Anchors of README.md headings, so links land on the matching section.
export const docs = {
  install: `${DOCS}#install`,
  setup: `${DOCS}#set-up-a-project`,
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
    title: "One command hands it over.",
    copy: "`harness-bridge start` runs the implementer’s CLI headless with the plan on stdin and prints a job ID at once, so a long job never hits the planner’s shell timeout.",
    file: "terminal",
    badge: "harness-bridge",
    lines: [
      "$ harness-bridge start --to codex < plan.md",
      "{",
      '  "job": "20261106-101500-a1b2c3",',
      '  "harness": "codex",',
      '  "status": "running",',
      '  "next": "harness-bridge wait 20261106-101500-a1b2c3"',
      "}",
    ],
  },
  {
    number: "03",
    label: "Report",
    title: "One JSON report back.",
    copy: "`harness-bridge wait` follows the CLI’s event stream and prints the final report: status, session ID, the agent’s message, files, commands with exit codes, and what Git sees afterwards.",
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
    copy: "The planner runs `git diff` and the checks itself and compares them with the plan. Fixes go back with `--session`, so the implementer continues the same session. After 3 rounds it reports what is still open.",
    file: "terminal",
    badge: "Planner",
    lines: [
      "$ git diff --stat",
      " server.go      | 12 ++++++++++++",
      " health_test.go | 24 ++++++++++++++++++++++++",
      "$ go test ./...",
      "ok   example/api   0.024s",
      "",
      "$ harness-bridge start --to codex --session 019a3f2c < fixes.md",
    ],
  },
];

export const jobCommands = [
  ["harness-bridge start --to codex < plan.md", "Prints a job ID at once."],
  ["harness-bridge wait JOB --max 9m", "Repeat while the status is running."],
  [
    "harness-bridge start --to codex --session ID < fixes.md",
    "Continue the same session.",
  ],
  ["harness-bridge cancel JOB", "Stops the whole process tree."],
  ["harness-bridge run --to codex < plan.md", "Or block until it ends."],
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
  [
    "recent · next",
    "Latest activity while running, and the command to run next.",
  ],
];

export const faqs = [
  [
    "What is harness-bridge, exactly?",
    "A small Go command that lets one agent CLI hand work to a different agent CLI and read back what happened. There is no orchestrator, terminal UI or container. The planner’s own agent loop is the workflow, and its instructions file (`CLAUDE.md` or `AGENTS.md`) says how to delegate.",
  ],
  [
    "Which agent should plan, and which should implement?",
    "Any of the five can take either role, and any pairing works both ways round. For example, Claude Code plans and reviews while Codex CLI writes the code. The one exception: planner and implementer can’t both be Claude Code, because Claude Code refuses to start inside another Claude Code session. Use its own subagents for that.",
  ],
  [
    "Why does work run as a background job?",
    "Implementations often outlast an agent’s shell timeout: Claude Code’s Bash tool stops after 10 minutes. `start` returns a job ID at once, and the planner runs `wait` again while the status is `running`. Jobs stay in your user cache directory (or `HARNESS_BRIDGE_HOME`) for 7 days, with the raw event log.",
  ],
  [
    "Will setup install agent CLIs without asking?",
    "No. `setup` checks that both CLIs are installed. For a missing one it shows the official installer, for example `npm install --global @openai/codex`, and runs it only after you answer `y`. Without a terminal it never installs, so an agent running `setup` has to ask you first and then pass `--yes`.",
  ],
  [
    "Can the planner skip a step or edit files itself?",
    "The workflow is instructions to the planner, not code, so a planner can skip a step. The bridge guarantees timeouts and cancellation that stop the whole process tree, no prompt passing through a shell, and independent Git evidence in every report. With a Claude Code planner, `--lock-planner` denies its `Edit`, `Write` and `NotebookEdit` tools, so every change goes through the implementer.",
  ],
  [
    "Do I need a new account or subscription?",
    "No. harness-bridge has no account or service of its own. It runs the agent CLIs you already use, each signed in with its own login, so usage follows each provider’s plan. Sign in to each CLI once, for example `codex login` or `claude auth login`.",
  ],
  [
    "Can a decision model route my requests?",
    "Yes, optionally. `harness-bridge route` asks Jev or Laya whether a request is an `answer`, `needs_planning` or can go straight to `direct_implement`. Laya is self-hosted: set `LAYA_ENDPOINT`, by default `http://127.0.0.1:8765/v1/systemone`. Jev is TypeSafe's hosted model, reached through OpenRouter with your `OPENROUTER_API_KEY`. When the model is unreachable or unsure, `route` returns the safe choice, `needs_planning`.",
  ],
  [
    "Does it work on Windows?",
    "Yes. harness-bridge builds and is tested on Linux, macOS and Windows, and on each one a timeout or `cancel` stops the whole process tree. `setup` offers the installer that fits your system.",
  ],
];
