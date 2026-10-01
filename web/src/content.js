export const REPO = "https://github.com/ErOr-0/Multiharness-Core";
export const RELEASE = {
  version: "v0.1.0-alpha.18",
  url: `${REPO}/releases/tag/v0.1.0-alpha.18`,
  bundle: `${REPO}/releases/download/v0.1.0-alpha.18/multiharness-docker.zip`,
  checksums: `${REPO}/releases/download/v0.1.0-alpha.18/checksums.txt`,
};
export const DOCS = `${REPO}/blob/main/README.md`;
export const DOCKER_HUB = "https://hub.docker.com/r/er0r2/multiharness";
export const DOCKER_IMAGE = "er0r2/multiharness:latest";
export const dockerCommands = {
  pull: "docker pull er0r2/multiharness",
  start: "docker start -ai multiharness",
};

// Ready-made teams. Commands come from roleSettingCommand, so each recipe is
// validated exactly like the interactive builder below.
export const teamRecipes = [
  {
    id: "cost-saver",
    name: "Cost saver",
    pitch: "Premium judgment, budget typing.",
    team: {
      planner: { harness: "claude", model: "opus", effort: "medium" },
      implementer: { harness: "claude", model: "haiku", effort: "low" },
      reviewer: { harness: "claude", model: "opus", effort: "high" },
    },
  },
  {
    id: "best-of-each",
    name: "Best of each",
    pitch: "Mix the subscriptions you already pay for.",
    team: {
      planner: { harness: "codex", model: "gpt-5.6-sol", effort: "high" },
      implementer: {
        harness: "muse",
        model: "muse-spark-1.3",
        effort: "medium",
      },
      reviewer: { harness: "claude", model: "opus", effort: "high" },
    },
  },
  {
    id: "one-subscription",
    name: "One subscription",
    pitch: "A whole team on one plan.",
    team: {
      planner: { harness: "muse", model: "muse-spark-1.3", effort: "low" },
      implementer: {
        harness: "muse",
        model: "muse-spark-1.3",
        effort: "medium",
      },
      reviewer: { harness: "muse", model: "muse-spark-1.3", effort: "high" },
    },
  },
];

export const workflowSteps = [
  {
    number: "01",
    label: "Plan",
    title: "A clear plan first.",
    copy: "A premium model turns your task into steps and acceptance criteria.",

    file: "plan.json",
    badge: "Premium planner",
    lines: [
      "{",
      '  "action": "implement",',
      '  "summary": "Add a health-check endpoint",',
      '  "steps": [',
      '    "Inspect the existing HTTP routes",',
      '    "Add GET /health and focused tests"',
      "  ],",
      '  "acceptance_criteria": ["Tests pass"]',
      "}",
    ],
  },
  {
    number: "02",
    label: "Implement",
    title: "A cheaper model builds.",
    copy: "The implementer gets the plan and a compact handoff. Multiharness tracks the real file changes.",

    file: "workspace.diff",
    badge: "Budget implementer",
    lines: [
      "Changes in your folder: health.go",
      "+ func health(w http.ResponseWriter, r *http.Request) {",
      '+   w.Header().Set("Content-Type", "application/json")',
      "+   w.WriteHeader(http.StatusOK)",
      '+   w.Write([]byte(`{"status":"ok"}`))',
      "+ }",
      "",
      "  Observed changes: health.go, health_test.go",
    ],
  },
  {
    number: "03",
    label: "Validate",
    title: "Your tests decide.",
    copy: "Your configured checks run, and their output goes to review.",

    file: "validation.log",
    badge: "Your project’s checks",
    lines: [
      "$ go test ./...",
      "ok   example/api        0.024s",
      "ok   example/health     0.018s",
      "",
      "$ go vet ./...",
      "No issues reported.",
      "",
      "2 configured checks completed",
    ],
  },
  {
    number: "04",
    label: "Review & repair",
    title: "Review, then repair.",
    copy: "Blocking findings go back for fixes until approved or a limit is reached.",

    file: "review.json",
    badge: "Premium reviewer",
    lines: [
      "{",
      '  "approved": true,',
      '  "summary": "Endpoint meets the plan",',
      '  "findings": [],',
      '  "suggestions": []',
      "}",
      "",
      "✓ Changes are ready for you to inspect and commit.",
    ],
  },
];

export const faqs = [
  [
    "How does Multiharness reduce what I spend on models?",
    "In team mode each role has its own CLI, model and reasoning level. Implementation usually reads and edits the most files, so it can run on a cheaper or subscription model while a premium model plans and reviews. Handoffs are bounded: the implementer gets the task, plan and the evidence it needs, not your whole workspace, so smaller context windows are enough. Actual cost depends on your task, models and provider plans.",
  ],
  [
    "What is Multiharness, exactly?",
    "Multiharness is a local command-line application, running inside one reusable Docker container. By default it sends your task to one configured CLI and shows its response. Optional team mode adds a planner, validation commands and an independent reviewer. This website introduces the product; the interactive preview is a simulation.",
  ],
  [
    "Do I need another model subscription?",
    "Choose Codex, OpenCode, Claude Code or Muse Code as your agent. Team mode lets you configure each role separately, so a premium model can plan and review while a cheaper one implements. Use your existing provider accounts; there is no extra Multiharness model subscription. Sign in inside the container using your own provider accounts; it does not automatically inherit logins or environment variables from your computer. Model access and usage charges depend on your provider and plan. Saved logins and settings persist in a private Docker volume.",
  ],
  [
    "Will it commit or overwrite my existing work?",
    "The task can change files in your selected folder. Direct mode uses your CLI permissions and project instructions. Team mode also saves a recovery copy of included files before implementation. No Git repository or commit is needed. Backups stay in your Docker state volume, including after container updates. Ignored files are excluded. There is no automatic rollback; inspect partial changes if a task stops.",
  ],
  [
    "Can I run it on Windows?",
    "Yes, use Docker Desktop in Linux-container mode and the same single-container setup. You do not need to install the agent tools in a WSL distribution. Docker Desktop still needs virtualization and may use WSL 2 behind the scenes. The image is a preview; native Windows executable workflows remain unsupported.",
  ],
  [
    "Is this a hosted service?",
    "No. Multiharness is built for a local, single operator working in their own folders. Provider CLIs still communicate with their services under your account settings. There is no Multiharness cloud account to create.",
  ],
];

export const roadmap = [
  {
    id: "development",
    label: "Under development",
    description: "The foundations are built. Release verification continues.",
    items: [
      {
        id: "reliability",
        tag: "Reliability",
        title: "Broader live-agent coverage.",
        description:
          "Expand authenticated approval, repair, cancellation and provider-handoff checks across more agent and model combinations. Jev planning and review requests are verified locally and inside Docker.",
        note: "Extend coverage across provider combinations",
      },
      {
        id: "installation",
        tag: "Installation",
        title: "One image. More platform checks.",
        description:
          "The Docker preview bundles Multiharness and its agent tools in one reusable container. Continue fresh-machine checks alongside the native architecture sandbox checks and authenticated workflow verification.",
        note: "Docker preview published for amd64 and arm64",
      },
    ],
  },
  {
    id: "next",
    label: "Available now",
    description: "Start with one agent. Add a team when you need it.",
    items: [
      {
        id: "builder",
        tag: "Agent choice",
        title: "One agent by default. An optional team.",
        description:
          "Choose a CLI, model and reasoning setting. It handles the task directly. Enable team mode for independent planning and review.",
        note: "Choose your builder directly at setup",
      },
      {
        id: "onboarding",
        tag: "Local setup",
        title: "Set up once. Get to work.",
        description:
          "The first launch asks for your folder and agent settings. They save automatically. Next time, start the same container and give it another task.",
        note: "First-launch guidance for a saved local team",
      },
    ],
  },
];
