export const REPO = "https://github.com/ErOr-0/Multiharness-Core";
export const DOCS = `${REPO}/blob/main/docs`;
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
    copy: "Your planner turns the task into a structured plan. The next agent gets the original intent, the steps, and the acceptance criteria.",
    file: "plan.json",
    badge: "Codex or OpenCode",
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
    copy: "Your chosen Codex or OpenCode implementer works in your folder with the task and plan in hand. Multiharness observes the actual file changes, rather than relying on an agent’s summary.",
    file: "workspace.diff",
    badge: "Codex or OpenCode",
    lines: [
      "diff --git a/health.go b/health.go",
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
    badge: "Independent Codex review",
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
    "What is Multiharness, exactly?",
    "Multiharness is a local command-line application, running inside one reusable Docker container. It coordinates a planner, an implementer, configured validation commands, and an independent reviewer in your project folder. This website introduces the product; the interactive preview is a simulation.",
  ],
  [
    "Do I need another model subscription?",
    "Choose the CLI and model for each planning and implementation role. For example, use Codex with Astra for planning and Luna for implementation; OpenCode is optional. Sign in inside the container using your own provider accounts; it does not automatically inherit logins or environment variables from your computer. Model access and usage charges depend on your provider and plan. Saved logins and settings persist in a private Docker volume.",
  ],
  [
    "Do I still need installation commands?",
    "Pull er0r2/multiharness and create one named container using the configuration bundle. Then run docker start -ai multiharness from any folder. Your files stay on your computer; logins and settings persist in one volume. Pulling alone does not create a container.",
  ],
  [
    "Can I start it with Docker Desktop’s Run button?",
    "Keep Docker Desktop running and reopen the same named container with docker start -ai multiharness. If it is already running, use docker attach multiharness. This is a terminal application with no browser dashboard or exposed web port.",
  ],
  [
    "Can Docker read my project files and use my tools?",
    "Docker shares your original folder at /workspace. Select a child folder before chatting or switch with /workspace. Use /config to change the agent team. There is no second working copy or synchronization step. It can contain one project, multiple projects with separate Git repositories, or plain files without Git. Edits appear in that folder on your computer; other folders are not shared automatically. Git, Codex, OpenCode, Go, Node, Python and common build tools are included. Extra SDKs such as .NET must be added to a derived image; tools installed on your computer are separate.",
  ],
  [
    "Will it commit or overwrite my existing work?",
    "It does not automatically commit, stage, stash, or roll back your changes. It snapshots the selected folder and protects existing uncommitted files in each Git repository. Plain files without Git can be edited against their captured starting state. Resolve existing Git changes to any file you want the workflow to edit before starting.",
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
    "The preview targets Linux amd64 and arm64, including Docker Desktop. The single-container lifecycle passes on macOS Apple silicon; native Linux checks run in CI. Linux with AppArmor needs a one-time host profile setup and matching UID/GID. Fresh Windows installation and authenticated workflow release checks remain separate; see the Docker guide.",
  ],
  [
    "Is this a hosted service?",
    "No. Multiharness is built for a local, single operator working in their own repositories. Provider CLIs still communicate with their services under your account settings. There is no Multiharness cloud account to create.",
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
        title: "Real-agent confidence.",
        description:
          "Verify full approval and repair loops, cancellation, and provider handoffs with authenticated Codex and OpenCode. The test harness is built; live verification is still pending.",
        note: "Complete the live-agent release checks",
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
    label: "Coming next",
    description: "Planned improvements to make your team truly yours.",
    items: [
      {
        id: "builder",
        tag: "Agent choice",
        title: "Codex as your builder.",
        description:
          "Choose Codex or OpenCode for implementation and repairs. Run an all-Codex team, or mix harnesses and models by role, with the same independent review workflow.",
        note: "Choose your builder directly at setup",
      },
      {
        id: "onboarding",
        tag: "Local setup",
        title: "Set up once. Get to work.",
        description:
          "Guide the first launch through role and model choices, then reuse a saved local team. Extend the existing /config and /save flow to include your chosen builder.",
        note: "First-launch guidance for a saved local team",
      },
    ],
  },
];
