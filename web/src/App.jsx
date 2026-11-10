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
  Menu,
  Minus,
  Plus,
  Route,
  ServerCog,
  ShieldCheck,
  SquareTerminal,
  TimerOff,
  Workflow,
  X,
} from "lucide-react";
import Brand, { BrandMark, MetaIcon } from "./components/Brand.jsx";
import HeroInstall from "./components/HeroInstall.jsx";
import Inline from "./components/Inline.jsx";
import RoutingFlow from "./components/RoutingFlow.jsx";
import SetupGuide from "./components/SetupGuide.jsx";
import WorkflowDemo from "./components/WorkflowDemo.jsx";
import { agents } from "./bridge-setup.js";
import useReveal from "./useReveal.js";
import {
  DOCS,
  REPO,
  docs,
  faqs,
  mcpTools,
  shellCommands,
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
  const [scrolled, setScrolled] = useState(false);
  const [active, setActive] = useState("");
  const trigger = useRef(null);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 8);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => {
    const sections = ["how", "agents", "start", "jobs", "routing", "faq"]
      .map((id) => document.getElementById(id))
      .filter(Boolean);
    if (!sections.length || typeof IntersectionObserver === "undefined") return;
    const visible = new Set();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) visible.add(entry.target.id);
          else visible.delete(entry.target.id);
        }
        // Above the first section (e.g. back in the hero) nothing matches.
        const current = [...visible];
        setActive(current.length ? current[current.length - 1] : "");
      },
      { rootMargin: "-40% 0px -55% 0px" },
    );
    sections.forEach((section) => observer.observe(section));
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!open) return;
    function close(event) {
      if (event.key === "Escape") {
        setOpen(false);
        trigger.current?.focus();
      }
    }
    function onPointerDown(event) {
      if (!event.target.closest(".site-header")) setOpen(false);
    }
    function onResize() {
      if (window.innerWidth > 960) setOpen(false);
    }
    window.addEventListener("keydown", close);
    window.addEventListener("pointerdown", onPointerDown);
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("keydown", close);
      window.removeEventListener("pointerdown", onPointerDown);
      window.removeEventListener("resize", onResize);
    };
  }, [open]);

  return (
    <header className={`site-header ${scrolled ? "is-scrolled" : ""}`}>
      <div className="container header-inner">
        <Brand />
        <nav
          aria-label="Main navigation"
          className={`main-nav ${open ? "is-open" : ""}`}
          id="main-navigation"
        >
          {navigation.map(([href, label]) => (
            <a
              href={href}
              key={href}
              className={active === href.slice(1) ? "is-active" : ""}
              aria-current={active === href.slice(1) ? "true" : undefined}
              onClick={() => setOpen(false)}
            >
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
          <a
            className="button button-primary mobile-cta"
            href="#start"
            onClick={() => setOpen(false)}
          >
            Get started <ArrowRight size={15} />
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
          <a className="hero-pill" href="#start">
            <span className="pill-tag">New</span>
            One install line. Three questions.
            <ArrowRight size={14} />
          </a>
          <h1 id="hero-title">
            One agent plans.
            <span className="hero-gradient">Another builds.</span>
          </h1>
          <p className="hero-description">
            harness-bridge lets one coding agent hand work to another agent CLI
            and read back what happened. Claude Code plans, Codex CLI writes the
            code, Claude Code reviews. You keep working in your planner.
          </p>
          <div className="hero-actions">
            <a className="button button-primary" href="#start">
              Install harness-bridge <SquareTerminal size={18} />
            </a>
            <a className="button button-ghost" href="#how">
              See how it works <ArrowRight size={16} />
            </a>
          </div>
          <HeroInstall />
          <ul className="hero-proof" aria-label="Highlights">
            <li>
              <Check size={15} /> No orchestrator
            </li>
            <li>
              <Check size={15} /> One install line, no Go
            </li>
            <li>
              <Check size={15} /> Git evidence in every report
            </li>
          </ul>
        </div>
        <WorkflowDemo />
      </div>
      <IntegrationStrip />
      <StatsBand />
    </section>
  );
}

const stats = [
  [String(agents.length), "agent CLIs, any pairing"],
  ["1", "install line, no Go"],
  ["3", "setup questions"],
  ["0", "orchestrators or containers"],
];

function StatsBand() {
  return (
    <div className="stats-band">
      <ul className="container stats-inner" aria-label="At a glance">
        {stats.map(([value, label], index) => (
          <li
            key={label}
            data-reveal
            style={{ "--reveal-delay": `${index * 70}ms` }}
          >
            <span className="stat-value">{value}</span>
            <span className="stat-label">{label}</span>
          </li>
        ))}
      </ul>
    </div>
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
    <div className="section-heading" data-reveal>
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
          loop is the workflow. harness-bridge runs inside it as an MCP server
          and does the hand-off.
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
    ShieldCheck,
    "Installs only after you confirm.",
    "Setup shows its plan and installs nothing until you answer `Continue? [Y/n]`. The planner’s MCP tools can’t install anything.",
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
          {guarantees.map(([Icon, title, copy], index) => (
            <article
              className="feature-card"
              key={title}
              data-reveal
              style={{ "--reveal-delay": `${index * 70}ms` }}
            >
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
          accent="Pick one to plan, another to build."
        >
          Each is a single adapter file in <code>cmd/harness-bridge</code>: its
          command line, its event parser and its official installer.
        </SectionHeading>
        <ul className="agent-grid">
          {agents.map((agent, index) => (
            <li
              className="agent-card"
              key={agent.name}
              data-reveal
              style={{ "--reveal-delay": `${index * 70}ms` }}
            >
              <div className="agent-card-top">
                <span className="agent-icon">{agentIcons[agent.name]}</span>
                <h3>{agent.title}</h3>
              </div>
              <code>--to {agent.name}</code>
              <div className="agent-badges">
                <span
                  className={`agent-session ${agent.mcpPlanner ? "" : "agent-session-no"}`}
                >
                  {agent.mcpPlanner
                    ? "Plans and implements"
                    : "Implements only"}
                </span>
                <span
                  className={`agent-session ${agent.resume ? "" : "agent-session-no"}`}
                >
                  {agent.resume ? "Continues a session" : "New job per fix"}
                </span>
              </div>
              <p>
                <Inline text={agent.note} />
              </p>
            </li>
          ))}
        </ul>
        <p className="agents-note">
          <Info size={16} aria-hidden="true" />
          <span>
            Setup connects the planner you pick for all your projects. Claude
            Code can’t implement for itself: it refuses to start inside another
            Claude Code session, so use its own subagents for that.
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
          accent="Short tool calls."
        >
          Implementations often outlast an agent’s tool or shell timeout (Claude
          Code’s Bash tool stops after 10 minutes), so work runs as a background
          job.
        </SectionHeading>
        <div className="jobs-grid">
          <div className="jobs-commands" data-reveal>
            <h3>Tools the planner gets</h3>
            <ol aria-label="MCP tools">
              {mcpTools.map(([tool, note]) => (
                <li key={tool}>
                  <code>{tool}</code>
                  <span>{note}</span>
                </li>
              ))}
            </ol>
            <h4>The same from a shell</h4>
            <p>For scripts, and planners that can’t load MCP servers:</p>
            <ul className="shell-commands">
              {shellCommands.map((command) => (
                <li key={command}>
                  <code>{command}</code>
                </li>
              ))}
            </ul>
            <p>
              <code>start</code> and <code>run</code> also take{" "}
              <code>--sandbox read-only</code>, <code>--model</code>,{" "}
              <code>--reasoning</code>, <code>--timeout</code>,{" "}
              <code>--prompt-file</code>, <code>--bin</code> and <code>--</code>{" "}
              to pass anything else to the CLI.
            </p>
          </div>
          <div className="jobs-report" data-reveal>
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
          Jobs and their logs stay in your user cache folder (or{" "}
          <code>HARNESS_BRIDGE_HOME</code>) for 7 days.{" "}
          <a
            className="text-link"
            href={docs.commandLine}
            target="_blank"
            rel="noreferrer"
          >
            Command-line options <ArrowUpRight size={15} />
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
        <div className="faq-intro" data-reveal>
          <span className="eyebrow">QUESTIONS</span>
          <h2 id="faq-title">Good to know.</h2>
          <a className="text-link" href={DOCS} target="_blank" rel="noreferrer">
            Full documentation <ArrowUpRight size={16} />
          </a>
        </div>
        <div className="faq-list" data-reveal>
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
      <div className="container">
        <div className="final-cta-panel" data-reveal>
          <BrandMark />
          <h2 id="final-cta-title">Plan with one. Build with another.</h2>
          <p>Install once. Keep planning where you already work.</p>
          <div className="hero-actions">
            <a className="button button-primary" href="#start">
              Set up harness-bridge <ArrowRight size={16} />
            </a>
            <a
              className="button button-ghost-dark"
              href={REPO}
              target="_blank"
              rel="noreferrer"
            >
              <GitFork size={16} /> GitHub
            </a>
          </div>
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
    </footer>
  );
}

export default function App() {
  useReveal();
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
