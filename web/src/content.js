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

export const workflowSteps = [
  {
    number: "01",
    label: "Plan",
    title: "Start with a shared understanding.",
    copy: "For changes that need a plan, your planner turns the task into structured steps. The next agent gets the original intent, the steps, and the acceptance criteria.",
    file: "plan.json",
    badge: "Codex, OpenCode or Claude",
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
    title: "Give the builder the whole picture.",
    copy: "Your chosen Codex, OpenCode or Claude implementer works in your folder with the task and plan in hand. Multiharness observes the actual file changes, rather than relying on an agent’s summary.",
    file: "workspace.diff",
    badge: "Codex, OpenCode or Claude",
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
    title: "Give the review real evidence.",
    copy: "Run the test and validation commands you configure. Their exit codes and output travel with the code changes into the independent review.",
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
    title: "A second perspective. A clear finish.",
    copy: "The reviewer inspects the plan, diff, and check results. Blocking findings go back for repair until approval or an explicit stopping condition.",
    file: "review.json",
    badge: "Your chosen reviewer",
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
    "Do I need Codex, OpenCode or Claude Code installed first?",
    "No separate agent installation is needed for Docker: all three CLIs are included. Choose your agent inside the app and sign in to its provider. For native macOS/Linux use, Multiharness can offer to install a missing selected agent through npm with your confirmation. Node.js and npm must already be available; sign in and rerun the task afterward. Native Windows automatic installation is not supported.",
  ],
  [
    "Do I need an OpenRouter key for Jev?",
    "Only if you enable optional Jev routing in Team mode. It is disabled by default. With Jev enabled, the terminal app asks for a missing key using hidden input and keeps it only for the session. Scripted runs must provide OPENROUTER_API_KEY. Jev uses your OpenRouter credits. Before a Team agent starts, Jev chooses between answering read-only, planning a change, and direct implementation. Its route and confidence appear in progress. Failed or uncertain routing falls back to read-only assessment; failed checks or missing validation keep full review in place.",
  ],
  [
    "What is Multiharness, exactly?",
    "Multiharness is a local command-line application, running inside one reusable Docker container. By default it sends your task to one configured CLI and shows its response. Optional team mode adds a planner, validation commands and an independent reviewer. This website introduces the product; the interactive preview is a simulation.",
  ],
  [
    "What can I change in /config?",
    "Open /config inside the app: 1 selects your project folder, 2 sets your agent or team, 3 controls the selected agent’s supported permissions, and 4 switches Direct/Team mode. Direct uses one agent. Team lets you choose a separate planner, implementer and reviewer, each with its own CLI, model and reasoning or variant. Menu changes save automatically; /cancel keeps your settings. Switching modes starts a new conversation. Use /configuration to check readiness, /setup to resolve missing prerequisites, /settings to see current values and /options for advanced controls such as timeouts and progress; change those with /set, then /save.",
  ],
  [
    "Which accounts need to be ready?",
    "The app checks the agents selected for your workflow: one in Direct mode, or the planner, implementer and reviewer in Team mode. Use /configuration to see each role’s status and /setup for missing setup. Fallbacks are off by default; optional fallback accounts are checked only after you accept a switch. Native login checks confirm local setup, but cannot guarantee remote model access or remaining credits.",
  ],
  [
    "Do I need another model subscription?",
    "Choose Codex, OpenCode or Claude Code as your agent. Team mode lets you configure each role separately. Use your existing provider accounts; there is no extra Multiharness model subscription. Sign in inside the container using your own provider accounts; it does not automatically inherit logins or environment variables from your computer. Model access and usage charges depend on your provider and plan. Saved logins and settings persist in a private Docker volume.",
  ],
  [
    "Do I need to download or run a setup script?",
    "No. Pull the image, enter your projects folder path on this page and copy the generated Docker command. Docker Compose fetches the configuration and security files from GitHub; Git must be installed. Native Linux hosts with AppArmor also need the named host policy installed once. Configure agents and models inside the app. Later, use docker start -ai multiharness from any folder.",
  ],
  [
    "Can I start it with Docker Desktop’s Run button?",
    "Keep Docker Desktop running and reopen the same named container with docker start -ai multiharness. If it is already running, use docker attach multiharness. This is a terminal application with no browser dashboard or exposed web port.",
  ],
  [
    "Can Docker read my project files and use my tools?",
    "Docker shares your original folder at /workspace. Select a child folder before chatting or switch with /workspace. You can also use /config and choose 1 to change projects; the selection saves automatically. There is no second working copy or synchronization step. It can contain one project, multiple projects with separate Git repositories, or plain files without Git. Edits appear in that folder on your computer; other folders are not shared automatically. Git, Codex, OpenCode, Claude Code, Go, Node, Python and common build tools are included. Extra SDKs such as .NET must be added to a derived image; tools installed on your computer are separate.",
  ],
  [
    "Will it commit or overwrite my existing work?",
    "The task can change files in your selected folder. Direct mode uses your CLI permissions and project instructions. Team mode also saves a recovery copy of included files before implementation. No Git repository or commit is needed. Backups stay in your Docker state volume, including after container updates. Ignored files are excluded. There is no automatic rollback; inspect partial changes if a task stops.",
  ],
  [
    "Will the terminal fill up with command output?",
    "Progress is compact by default: an animated indicator shows the current stage and elapsed time. Command output stays collapsed. Use /set progress expanded before a task if you want the detailed transcript. Failures and the final result remain visible either way.",
  ],
  [
    "What happens when a review finds a problem?",
    "Blocking findings, the original task, the plan, and current evidence return to the implementation agent for repair. The workflow stops on approval, a failure, cancellation, or the configured repair limit. Reaching a limit is never treated as approval.",
  ],
  [
    "Can I run it on Windows?",
    "Yes, use Docker Desktop in Linux-container mode and the same single-container setup. You do not need to install the agent tools in a WSL distribution. Docker Desktop still needs virtualization and may use WSL 2 behind the scenes. The image is a preview; native Windows executable workflows remain unsupported.",
  ],
  [
    "Is the Docker image ready for every machine?",
    "The preview targets Linux amd64 and arm64, including Docker Desktop. The single-container lifecycle passes on macOS Apple silicon; native Linux checks run in CI. Linux with AppArmor needs a one-time host profile setup and matching UID/GID. Fresh Windows installation and authenticated workflow release checks remain separate; see the README.",
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
