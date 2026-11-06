import { useEffect, useRef, useState } from "react";
import {
  Aperture,
  ArrowRight,
  ArrowUpRight,
  Braces,
  Check,
  Code2,
  Command,
  FileCode2,
  GitCompare,
  GitFork,
  Info,
  Lock,
  Menu,
  Minus,
  Plus,
  Route,
  ServerCog,
  SquareTerminal,
  TimerOff,
  Workflow,
  X,
} from "lucide-react";
import Brand, { BrandMark, MetaIcon } from "./components/Brand.jsx";
import Inline from "./components/Inline.jsx";
import RoutingFlow from "./components/RoutingFlow.jsx";
import SetupGuide from "./components/SetupGuide.jsx";
import WorkflowDemo from "./components/WorkflowDemo.jsx";
import { agents } from "./bridge-setup.js";
import {
  DOCS,
  LEGACY_TAG,
  REPO,
  docs,
  faqs,
  jobCommands,
  jobStatuses,
  reportFields,
  workflowSteps,
} from "./content.js";

const navigation = [
  ["#how", "How it works"],
  ["#agents", "Agents"],
  ["#start", "Set up"],
  ["#jobs", "Jobs"],
  ["#routing", "Routing"],
  ["#faq", "FAQs"],
];

const agentIcons = {
  codex: <Aperture />,
  claude: <Code2 />,
  muse: <MetaIcon />,
  opencode: <Braces />,
  commandcode: <Command />,
};

function Header() {
  const [open, setOpen] = useState(false);
  const trigger = useRef(null);
  useEffect(() => {
    if (!open) return;
    function close(event) {
      if (event.key === "Escape") {
        setOpen(false);
        trigger.current?.focus();
      }
    }
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [open]);

  return (
    <header className="site-header">
      <div className="container header-inner">
        <Brand />
        <nav
          aria-label="Main navigation"
          className={`main-nav ${open ? "is-open" : ""}`}
          id="main-navigation"
        >
          {navigation.map(([href, label]) => (
            <a href={href} key={href} onClick={() => setOpen(false)}>
              {label}
            </a>
          ))}
          <a
            href={REPO}
            target="_blank"
            rel="noreferrer"
            className="mobile-github"
            onClick={() => setOpen(false)}
          >
            GitHub <ArrowUpRight size={14} />
          </a>
        </nav>
        <div className="header-actions">
          <a
            className="github-link"
            href={REPO}
            target="_blank"
            rel="noreferrer"
            aria-label="View Multiharness on GitHub"
          >
            <GitFork size={18} />
          </a>
          <a className="button button-primary button-small" href="#start">
            Get started <ArrowRight size={15} />
          </a>
          <button
            ref={trigger}
            className="menu-toggle"
            aria-label={open ? "Close navigation" : "Open navigation"}
            aria-expanded={open}
            aria-controls="main-navigation"
            onClick={() => setOpen(!open)}
          >
            {open ? <X size={22} /> : <Menu size={22} />}
          </button>
        </div>
      </div>
    </header>
  );
}

function Hero() {
  return (
    <section className="hero" aria-labelledby="hero-title">
      <div className="hero-glow" aria-hidden="true" />
      <div className="container hero-inner">
        <div className="hero-copy">
          <a className="hero-pill" href="#agents">
            <span className="pill-tag">New</span>
            Five agent CLIs. Any pairing.
            <ArrowRight size={14} />
          </a>
          <h1 id="hero-title">
            One agent plans.
            <span className="hero-gradient">Another builds.</span>
          </h1>
          <p className="hero-description">
            harness-bridge is a small Go command that lets one coding agent hand
            work to another agent CLI and read back what happened. Claude Code
            plans, Codex CLI writes the code, Claude Code reviews. Or the other
            way round.
          </p>
          <div className="hero-actions">
            <a className="button button-primary" href="#start">
              Set up harness-bridge <SquareTerminal size={18} />
            </a>
            <a className="button button-ghost" href="#how">
              See how it works <ArrowRight size={16} />
            </a>
          </div>
          <ul className="hero-proof" aria-label="Highlights">
            <li>
              <Check size={15} /> No orchestrator
            </li>
            <li>
              <Check size={15} /> Your own CLIs and sign-ins
            </li>
            <li>
              <Check size={15} /> Git evidence in every report
            </li>
          </ul>
        </div>
        <WorkflowDemo />
      </div>
      <IntegrationStrip />
    </section>
  );
}

function IntegrationStrip() {
  const groups = [
    [
      "Works with",
      agents.map((agent) => [agentIcons[agent.name], agent.title]),
    ],
    [
      "Routes with",
      [
        [<Route key="i" />, "Jev"],
        [<ServerCog key="i" />, "Laya", "self-hosted"],
      ],
      "#routing",
    ],
  ];
  return (
    <div className="integration-strip">
      <div className="container integration-inner">
        {groups.map(([label, tools, href]) => (
          <div
            className={`integration-group ${href ? "integration-group-routing" : ""}`}
            key={label}
          >
            <p>{label}</p>
            <ul>
              {tools.map(([icon, name, note]) => (
                <li className="integration-name" key={name}>
                  {href ? (
                    <a href={href}>
                      {icon}
                      {name}
                      {note && <span className="integration-note">{note}</span>}
                    </a>
                  ) : (
                    <>
                      {icon}
                      {name}
                    </>
                  )}
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </div>
  );
}

function SectionHeading({ eyebrow, id, title, accent, children }) {
  return (
    <div className="section-heading">
      <span className="eyebrow">{eyebrow}</span>
      <h2 id={id}>
        {title}
        {accent && <span className="muted-heading"> {accent}</span>}
      </h2>
      {children && <p>{children}</p>}
    </div>
  );
}

function WorkflowSection() {
  const [selected, setSelected] = useState(0);
  const step = workflowSteps[selected];
  const tabRefs = useRef([]);
  function navigateTabs(event, index) {
    let next;
    if (event.key === "ArrowRight") next = (index + 1) % workflowSteps.length;
    else if (event.key === "ArrowLeft")
      next = (index + workflowSteps.length - 1) % workflowSteps.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = workflowSteps.length - 1;
    else return;
    event.preventDefault();
    setSelected(next);
    tabRefs.current[next]?.focus();
  }

  return (
    <section
      className="section workflow-section"
      id="how"
      aria-labelledby="workflow-title"
    >
      <div className="container">
        <SectionHeading
          eyebrow="HOW IT WORKS"
          id="workflow-title"
          title="Plan. Delegate. Review."
          accent="Repeat until right."
        >
          There is no orchestrator, terminal UI or container: the planner’s own
          loop is the workflow. Its instructions file says how to delegate, and
          harness-bridge does the hand-off.
        </SectionHeading>
        <div
          className="workflow-tabs"
          role="tablist"
          aria-label="Workflow stages"
        >
          {workflowSteps.map((item, index) => (
            <button
              key={item.label}
              ref={(node) => {
                tabRefs.current[index] = node;
              }}
              role="tab"
              id={`step-tab-${index}`}
              aria-selected={selected === index}
              aria-controls="step-panel"
              tabIndex={selected === index ? 0 : -1}
              onKeyDown={(event) => navigateTabs(event, index)}
              onClick={() => setSelected(index)}
            >
              <span>{item.number}</span>
              {item.label}
            </button>
          ))}
        </div>
        <div
          className="workflow-detail"
          role="tabpanel"
          id="step-panel"
          aria-labelledby={`step-tab-${selected}`}
          tabIndex={0}
        >
          <div className="workflow-detail-copy">
            <span className="step-kicker">
              STEP {step.number} / 04 · {step.badge}
            </span>
            <h3>{step.title}</h3>
            <p>
              <Inline text={step.copy} />
            </p>
          </div>
          <div className="code-preview">
            <div className="code-preview-header">
              <span>
                <FileCode2 size={15} />
                {step.file}
              </span>
              <span className="code-agent">{step.badge}</span>
            </div>
            <div
              className="code-lines"
              key={selected}
              tabIndex={0}
              role="region"
              aria-label={`${step.label} example output`}
            >
              {step.lines.map((line, index) => (
                <div
                  className={
                    line.startsWith("ok ") || line.includes('"completed"')
                      ? "positive-code"
                      : ""
                  }
                  key={index}
                >
                  <span className="line-number" aria-hidden="true">
                    {index + 1}
                  </span>
                  <code>{line || " "}</code>
                </div>
              ))}
            </div>
            <div className="code-preview-footer">ILLUSTRATIVE OUTPUT</div>
          </div>
        </div>
        <div className="repair-loop">
          <Workflow size={16} />
          <p>
            <strong>
              Something wrong? Fixes go back to the same session, up to 3
              rounds.
            </strong>
          </p>
        </div>
      </div>
    </section>
  );
}

const guarantees = [
  [
    GitCompare,
    "Independent Git evidence.",
    "Every report carries `git status` and `git diff --stat`, so the review doesn’t depend on the agent’s own account.",
  ],
  [
    TimerOff,
    "Timeouts that really stop.",
    "A timeout or `cancel` stops the whole process tree, not just the first process.",
  ],
  [
    SquareTerminal,
    "No prompt through a shell.",
    "The plan reaches the CLI on stdin (Muse Code: a prompt file), never as shell text.",
  ],
  [
    Lock,
    "A planner that can’t edit.",
    "With a Claude Code planner, `--lock-planner` denies `Edit`, `Write` and `NotebookEdit`, so every change goes through the implementer.",
  ],
];

function Guarantees() {
  return (
    <section
      className="section why-section"
      id="guarantees"
      aria-labelledby="guarantees-title"
    >
      <div className="container">
        <SectionHeading
          eyebrow="WHAT THE BRIDGE GUARANTEES"
          id="guarantees-title"
          title="Instructions, not a cage."
          accent="Four things it does enforce."
        >
          The workflow is instructions to the planner, not code, so a planner
          can skip a step. The bridge itself guarantees these.
        </SectionHeading>
        <div className="feature-grid">
          {guarantees.map(([Icon, title, copy]) => (
            <article className="feature-card" key={title}>
              <span className="feature-icon">
                <Icon size={20} />
              </span>
              <h3>{title}</h3>
              <p>
                <Inline text={copy} />
              </p>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}

function Agents() {
  return (
    <section
      className="section agents-section"
      id="agents"
      aria-labelledby="agents-title"
    >
      <div className="container">
        <SectionHeading
          eyebrow="SUPPORTED AGENTS"
          id="agents-title"
          title="Five CLIs."
          accent="Any one plans, any one builds."
        >
          Each is a single adapter file in <code>cmd/harness-bridge</code>: its
          command line, its event parser and its official installer.
        </SectionHeading>
        <ul className="agent-grid">
          {agents.map((agent) => (
            <li className="agent-card" key={agent.name}>
              <div className="agent-card-top">
                <span className="agent-icon">{agentIcons[agent.name]}</span>
                <h3>{agent.title}</h3>
              </div>
              <code>--to {agent.name}</code>
              <span
                className={`agent-session ${agent.resume ? "" : "agent-session-no"}`}
              >
                {agent.resume ? "Continues a session" : "New job per fix"}
              </span>
              <p>
                <Inline text={agent.note} />
              </p>
            </li>
          ))}
        </ul>
        <p className="agents-note">
          <Info size={16} aria-hidden="true" />
          <span>
            Planner and implementer can’t both be Claude Code: Claude Code
            refuses to start inside another Claude Code session. Use its own
            subagents for that.
          </span>
        </p>
      </div>
    </section>
  );
}

function Jobs() {
  return (
    <section
      className="section jobs-section"
      id="jobs"
      aria-labelledby="jobs-title"
    >
      <div className="container">
        <SectionHeading
          eyebrow="HOW A JOB RUNS"
          id="jobs-title"
          title="Long jobs."
          accent="Short shell calls."
        >
          Implementations often outlast an agent’s shell timeout (Claude Code’s
          Bash tool stops after 10 minutes), so work runs as a background job.
        </SectionHeading>
        <div className="jobs-grid">
          <div className="jobs-commands">
            <h3>Commands the planner runs</h3>
            <ol>
              {jobCommands.map(([command, note]) => (
                <li key={command}>
                  <code>{command}</code>
                  <span>{note}</span>
                </li>
              ))}
            </ol>
            <p>
              Also <code>--sandbox read-only</code>, <code>--model</code>,{" "}
              <code>--reasoning</code>, <code>--timeout</code>,{" "}
              <code>--prompt-file</code>, <code>--bin</code> and <code>--</code>{" "}
              to pass anything else to the CLI.
            </p>
          </div>
          <div className="jobs-report">
            <h3>One JSON report</h3>
            <ul className="status-tags" aria-label="Job statuses">
              {jobStatuses.map((status) => (
                <li
                  key={status}
                  className={status === "completed" ? "status-completed" : ""}
                >
                  {status}
                </li>
              ))}
            </ul>
            <dl className="report-fields">
              {reportFields.map(([field, meaning]) => (
                <div key={field}>
                  <dt>{field}</dt>
                  <dd>
                    <Inline text={meaning} />
                  </dd>
                </div>
              ))}
            </dl>
          </div>
        </div>
        <p className="jobs-footnote">
          Jobs stay in your user cache directory (or{" "}
          <code>HARNESS_BRIDGE_HOME</code>) for 7 days, with the raw event log.{" "}
          <a
            className="text-link"
            href={docs.jobs}
            target="_blank"
            rel="noreferrer"
          >
            All job options <ArrowUpRight size={15} />
          </a>
        </p>
      </div>
    </section>
  );
}

function FAQ() {
  const [open, setOpen] = useState(0);
  return (
    <section
      className="section faq-section"
      id="faq"
      aria-labelledby="faq-title"
    >
      <div className="container faq-inner">
        <div className="faq-intro">
          <span className="eyebrow">QUESTIONS</span>
          <h2 id="faq-title">Good to know.</h2>
          <a className="text-link" href={DOCS} target="_blank" rel="noreferrer">
            Full documentation <ArrowUpRight size={16} />
          </a>
        </div>
        <div className="faq-list">
          {faqs.map(([question, answer], index) => (
            <article
              className={`faq-item ${open === index ? "open" : ""}`}
              key={question}
            >
              <h3>
                <button
                  aria-expanded={open === index}
                  aria-controls={`faq-answer-${index}`}
                  id={`faq-question-${index}`}
                  onClick={() => setOpen(open === index ? null : index)}
                >
                  {question}
                  <span>
                    {open === index ? <Minus size={17} /> : <Plus size={17} />}
                  </span>
                </button>
              </h3>
              <div
                id={`faq-answer-${index}`}
                role="region"
                aria-labelledby={`faq-question-${index}`}
                hidden={open !== index}
              >
                <p>
                  <Inline text={answer} />
                </p>
              </div>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}

function FinalCTA() {
  return (
    <section className="final-cta" aria-labelledby="final-cta-title">
      <div className="container final-cta-inner">
        <BrandMark />
        <h2 id="final-cta-title">Plan with one. Build with another.</h2>
        <div className="hero-actions">
          <a className="button button-primary" href="#start">
            Set up harness-bridge <ArrowRight size={16} />
          </a>
          <a
            className="button button-ghost"
            href={REPO}
            target="_blank"
            rel="noreferrer"
          >
            <GitFork size={16} /> GitHub
          </a>
        </div>
      </div>
    </section>
  );
}

function Footer() {
  return (
    <footer className="site-footer">
      <div className="container footer-inner">
        <Brand />
        <div className="footer-links">
          <a href={DOCS} target="_blank" rel="noreferrer">
            Documentation
          </a>
          <a href={docs.agents} target="_blank" rel="noreferrer">
            Supported agents
          </a>
          <a href={REPO} target="_blank" rel="noreferrer">
            GitHub
          </a>
          <a href="#top" className="back-to-top">
            Back to top ↑
          </a>
        </div>
      </div>
      <p className="container footer-legacy">
        The previous app, magent, lives at tag{" "}
        <a href={LEGACY_TAG} target="_blank" rel="noreferrer">
          magent-final
        </a>
        .
      </p>
    </footer>
  );
}

export default function App() {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <div id="top" />
      <Header />
      <main id="main-content">
        <Hero />
        <WorkflowSection />
        <Guarantees />
        <Agents />
        <SetupGuide />
        <Jobs />
        <RoutingFlow />
        <FAQ />
        <FinalCTA />
      </main>
      <Footer />
    </>
  );
}
