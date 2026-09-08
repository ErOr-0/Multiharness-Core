import { useEffect, useRef, useState } from "react";
import {
  Aperture,
  ArrowRight,
  ArrowUpRight,
  Check,
  CheckCheck,
  ChevronDown,
  Code2,
  Copy,
  FileCode2,
  FileText,
  GitBranch,
  GitFork,
  Laptop,
  LockKeyhole,
  Menu,
  Minus,
  Monitor,
  Play,
  Plus,
  ShieldCheck,
  SquareTerminal,
  Terminal,
  Workflow,
  X,
} from "lucide-react";
import Brand, { BrandMark } from "./components/Brand.jsx";
import WorkflowDemo from "./components/WorkflowDemo.jsx";
import Roadmap from "./components/Roadmap.jsx";
import {
  DOCS,
  DOCKER_HUB,
  REPO,
  dockerCommands,
  faqs,
  workflowSteps,
} from "./content.js";

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
          <a href="#workflow" onClick={() => setOpen(false)}>
            The workflow
          </a>
          <a href="#why" onClick={() => setOpen(false)}>
            Why Multiharness
          </a>
          <a href="#roadmap" onClick={() => setOpen(false)}>
            Roadmap
          </a>
          <a href="#faq" onClick={() => setOpen(false)}>
            FAQs
          </a>
          <a
            href={`${REPO}`}
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
            <GitFork size={19} />
          </a>
          <a className="button button-dark button-small" href="#start">
            Get started <ArrowUpRight size={15} />
          </a>
          <button
            ref={trigger}
            className="menu-toggle"
            aria-label={open ? "Close navigation" : "Open navigation"}
            aria-expanded={open}
            aria-controls="main-navigation"
            onClick={() => setOpen(!open)}
          >
            {open ? <X size={23} /> : <Menu size={23} />}
          </button>
        </div>
      </div>
    </header>
  );
}

function Hero() {
  return (
    <section className="hero container" aria-labelledby="hero-title">
      <div className="hero-copy">
        <div className="eyebrow hero-eyebrow">
          <span className="green-dot" /> LOCAL BY DESIGN. TOGETHER BY DEFAULT.
        </div>
        <h1 id="hero-title">
          Different agents.
          <br />
          <span className="hero-highlight">One shared</span>
          <br />
          mission<span className="hero-period">.</span>
        </h1>
        <p className="hero-description">
          Your favorite coding agents, working as a team.
          <br className="desktop-break" /> Plan with one. Build with another.
          Get an independent review. All from your terminal.
        </p>
        <div className="hero-actions">
          <a className="button button-lime" href="#start">
            Get started with Docker <ArrowUpRight size={18} />
          </a>
          <a className="text-button" href="#workflow">
            <span className="play-circle">
              <Play size={11} fill="currentColor" />
            </span>
            See how it works
          </a>
        </div>
        <div className="hero-platforms">
          <span>
            <Laptop size={14} /> macOS · Docker
          </span>
          <span>
            <SquareTerminal size={14} /> Linux · Docker
          </span>
          <span>
            <Monitor size={14} /> Windows · Docker
          </span>
        </div>
      </div>
      <WorkflowDemo />
    </section>
  );
}

function IntegrationStrip() {
  return (
    <div className="integration-strip">
      <div className="container integration-inner">
        <p>
          THE TOOLS YOU KNOW.
          <br />
          <strong>A BETTER WAY TOGETHER.</strong>
        </p>
        <div className="integration-name">
          <Aperture />
          Codex
        </div>
        <span className="integration-plus">+</span>
        <div className="integration-name">
          <SquareTerminal />
          OpenCode
        </div>
        <span className="integration-plus">+</span>
        <div className="integration-name">
          <GitBranch />
          Your Git repo
        </div>
        <span className="integration-plus">=</span>
        <div className="integration-result">
          <BrandMark />
          One workflow.
        </div>
      </div>
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
      className="section workflow-section container"
      id="workflow"
      aria-labelledby="workflow-title"
    >
      <div className="section-heading">
        <div>
          <span className="eyebrow section-eyebrow">
            <span /> THE WORKFLOW
          </span>
          <h2 id="workflow-title">
            A handoff.
            <br />
            <span className="muted-heading">Not another copy-paste.</span>
          </h2>
        </div>
        <p>
          Stop carrying context between agent windows.
          <br className="desktop-break" /> Give your team one task. Multiharness
          connects the steps.
        </p>
      </div>
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
            <ArrowUpRight size={17} />
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
          <span className="step-kicker">STEP {step.number} / 04</span>
          <h3>{step.title}</h3>
          <p>{step.copy}</p>
          <div className="context-note">
            <GitBranch size={16} />
            <span>Original task + shared context, at every handoff.</span>
          </div>
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
                  line.startsWith("+") ||
                  line.startsWith("✓") ||
                  line.startsWith("ok ")
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
          <div className="code-preview-footer">
            <span className="green-dot" />
            ILLUSTRATIVE OUTPUT
          </div>
        </div>
      </div>
      <div className="repair-loop">
        <span className="repair-loop-line" />
        <Workflow size={16} />
        <p>
          Review finds a blocker?{" "}
          <strong>The feedback goes back. The context stays.</strong>
        </p>
        <span className="repair-loop-line" />
      </div>
    </section>
  );
}

function TeamPreview() {
  const [planner, setPlanner] = useState("gpt-5.6-sol");
  const [builder, setBuilder] = useState("big-pickle");
  const [reviewer, setReviewer] = useState("gpt-6-astra");
  return (
    <div className="team-preview">
      <div className="team-mini-row">
        <span className="team-role">PLANNER</span>
        <span>
          <Aperture size={14} />
          Codex
        </span>
        <div className="select-wrap">
          <select
            value={planner}
            onChange={(event) => setPlanner(event.target.value)}
            aria-label="Example planner model"
          >
            <option value="gpt-5.6-sol">gpt-5.6-sol</option>
            <option value="gpt-6-astra">gpt-6-astra</option>
          </select>
          <ChevronDown size={12} />
        </div>
      </div>
      <div className="team-mini-row">
        <span className="team-role">BUILDER</span>
        <span>
          <SquareTerminal size={14} />
          OpenCode
        </span>
        <div className="select-wrap">
          <select
            value={builder}
            onChange={(event) => setBuilder(event.target.value)}
            aria-label="Example builder model"
          >
            <option value="big-pickle">big-pickle</option>
            <option value="">OpenCode default</option>
          </select>
          <ChevronDown size={12} />
        </div>
      </div>
      <div className="team-mini-row">
        <span className="team-role">REVIEWER</span>
        <span>
          <Aperture size={14} />
          Codex
        </span>
        <div className="select-wrap">
          <select
            value={reviewer}
            onChange={(event) => setReviewer(event.target.value)}
            aria-label="Example reviewer model"
          >
            <option value="gpt-6-astra">gpt-6-astra</option>
            <option value="gpt-5.6-sol">gpt-5.6-sol</option>
          </select>
          <ChevronDown size={12} />
        </div>
      </div>
      <p className="example-label">
        Example team. Choose models your accounts support.
      </p>
    </div>
  );
}

function WhySection() {
  return (
    <section className="why-section" id="why" aria-labelledby="why-title">
      <div className="container section">
        <div className="section-heading">
          <div>
            <span className="eyebrow section-eyebrow">
              <span /> BUILT FOR THE WAY YOU WORK
            </span>
            <h2 id="why-title">
              More coordination.
              <br />
              <span className="muted-heading">Less compromise.</span>
            </h2>
          </div>
          <p>
            A focused tool for your local workflow.
            <br className="desktop-break" /> Your repository, your provider
            accounts, your decisions.
          </p>
        </div>
        <div className="feature-grid">
          <article className="feature-card feature-local">
            <span className="feature-icon">
              <GitBranch size={21} />
            </span>
            <h3>Your work. Still yours.</h3>
            <p>
              Changes happen in your Git repository. Existing work is tracked
              and protected. You decide what to inspect, keep, and commit.
            </p>
            <div className="file-preview">
              <div>
                <span>
                  <GitBranch size={13} /> YOUR REPOSITORY
                </span>
                <span className="outline-badge">LOCAL</span>
              </div>
              <div>
                <span>
                  <FileCode2 size={16} />
                  health.go
                </span>
                <span className="file-addition">
                  +12 <span>−0</span>
                </span>
              </div>
              <div>
                <span>
                  <FileCode2 size={16} />
                  health_test.go
                </span>
                <span className="file-addition">
                  +28 <span>−0</span>
                </span>
              </div>
              <div className="protected-file">
                <span>
                  <FileText size={16} />
                  your-notes.md
                </span>
                <span>
                  <LockKeyhole size={12} />
                  Protected
                </span>
              </div>
            </div>
          </article>
          <article className="feature-card feature-team">
            <span className="feature-icon">
              <Workflow size={21} />
            </span>
            <h3>Pick the right agent for the job.</h3>
            <p>
              Configure models by role. Let your planner think, your implementer
              build, and your reviewer bring a fresh perspective.
            </p>
            <TeamPreview />
          </article>
          <article className="feature-card feature-review">
            <span className="feature-icon">
              <ShieldCheck size={21} />
            </span>
            <div>
              <h3>Evidence over “looks good.”</h3>
              <p>
                Independent Git diffs and your configured test results give the
                reviewer something concrete to work with.
              </p>
            </div>
            <div className="evidence-tags">
              <span>
                <Check size={13} />
                Repository diff
              </span>
              <span>
                <Check size={13} />
                Configured checks
              </span>
              <span>
                <Check size={13} />
                Independent review
              </span>
            </div>
          </article>
          <article className="feature-card feature-finish">
            <span className="feature-icon">
              <CheckCheck size={21} />
            </span>
            <div>
              <h3>Know where the work stands.</h3>
              <p>
                Explicit outcomes, bounded repairs, and cancellation. A retry
                limit never quietly turns into a success.
              </p>
            </div>
            <div className="outcome-tags">
              <span className="outcome-approved">
                <span />
                approved
              </span>
              <span>answered</span>
              <span>cancelled</span>
              <span>repair_limit_reached</span>
            </div>
          </article>
        </div>
      </div>
    </section>
  );
}

function GettingStarted() {
  const [platform, setPlatform] = useState("macOS");
  const [copyState, setCopyState] = useState("idle");
  const timeout = useRef(null);
  const code = dockerCommands[platform];
  useEffect(() => () => window.clearTimeout(timeout.current), []);
  function choosePlatform(value) {
    setPlatform(value);
    setCopyState("idle");
    window.clearTimeout(timeout.current);
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
    window.clearTimeout(timeout.current);
    timeout.current = window.setTimeout(() => setCopyState("idle"), 3500);
  }

  return (
    <section
      className="section container"
      id="start"
      aria-labelledby="start-title"
    >
      <div className="start-panel">
        <div className="start-copy">
          <span className="eyebrow">
            <span className="green-dot" /> AVAILABLE ON DOCKER HUB
          </span>
          <h2 id="start-title">
            Pull the image.
            <br />
            Bring your team.
          </h2>
          <p>
            Multiharness, Codex, OpenCode and common build tools, bundled in one
            image. No source build or separate agent installs.
          </p>
          <ol className="docker-steps">
            <li>
              <strong>Start Docker.</strong> Use{" "}
              <a
                href="https://docs.docker.com/get-started/get-docker/"
                target="_blank"
                rel="noreferrer"
              >
                Docker Desktop or Engine
              </a>{" "}
              on your computer.
            </li>
            <li>
              <strong>Pull and launch.</strong> Use the commands for your
              platform, with your Git repository path.
            </li>
            <li>
              <strong>Sign in and configure.</strong> Setup offers provider
              login. Then use <code>/config</code> to choose models, configure
              your project checks, and use <code>/save</code> to keep your
              settings.
            </li>
          </ol>
          <a
            className="button button-lime"
            href={DOCKER_HUB}
            target="_blank"
            rel="noreferrer"
          >
            Get the Docker image <ArrowUpRight size={17} />
          </a>
          <a
            className="start-docs"
            href={`${DOCS}/docker.md`}
            target="_blank"
            rel="noreferrer"
          >
            Read the Docker setup guide <ArrowRight size={15} />
          </a>
          <div className="early-access-note">
            <span />
            Preview · amd64 + arm64 · platform verification ongoing
          </div>
        </div>
        <div className="install-card">
          <div
            className="platform-tabs"
            role="group"
            aria-label="Installation platform"
          >
            {["macOS", "Linux", "Windows"].map((item) => (
              <button
                key={item}
                aria-pressed={platform === item}
                onClick={() => choosePlatform(item)}
              >
                {item}
              </button>
            ))}
          </div>
          <div className="install-code-header">
            <span>
              <Terminal size={14} />{" "}
              {platform === "Windows" ? "POWERSHELL" : "TERMINAL"} · DOCKER
              PREVIEW
            </span>
            <button onClick={copy} aria-label="Copy Docker setup commands">
              {copyState === "copied" ? (
                <Check size={14} />
              ) : (
                <Copy size={14} />
              )}
              {copyState === "copied" ? "Copied!" : "Copy"}
            </button>
          </div>
          <pre
            className="install-code"
            tabIndex={0}
            role="region"
            aria-label="Docker setup commands"
          >
            <code>
              {code.split("\n").map((line, index) => (
                <span
                  key={index}
                  className={
                    line.startsWith("#")
                      ? "code-comment"
                      : line.startsWith("docker pull")
                        ? "code-command-accent"
                        : ""
                  }
                >
                  {line || " "}
                </span>
              ))}
            </code>
          </pre>
          <div className="install-requirements">
            {platform === "Windows" ? (
              <>
                <Monitor size={16} />
                <p>
                  Docker Desktop must be running in Linux-container mode.
                  <br />
                  Run from PowerShell. Docker may still use WSL 2 internally.
                </p>
              </>
            ) : (
              <>
                <Code2 size={16} />
                <p>
                  {platform === "Linux"
                    ? "Start your local Docker Engine."
                    : "Start Docker Desktop."}
                  <br />
                  The launcher mounts your project and keeps logins in a private
                  volume.
                </p>
              </>
            )}
          </div>
          <p className="install-next-run">
            Run these once from a folder where you keep tools, outside your
            target repository. Keep the extracted launcher folder; for later
            sessions, reuse the last command. If the temporary container name is
            taken, choose another name in the create, copy and remove commands.
          </p>
          <p
            className={`copy-feedback ${copyState === "failed" ? "visible" : ""}`}
            role="status"
          >
            {copyState === "failed"
              ? "Copy unavailable. Select and copy the commands above."
              : copyState === "copied"
                ? "Docker setup commands copied to clipboard."
                : ""}
          </p>
        </div>
      </div>
      <div className="start-footnote">
        <a href={`${DOCS}/releases.md`} target="_blank" rel="noreferrer">
          Prefer a native macOS/Linux binary or source build?
        </a>
        <span>
          Use your own provider accounts. Host logins are not imported.
        </span>
      </div>
    </section>
  );
}

function FAQ() {
  const [open, setOpen] = useState(0);
  return (
    <section
      className="faq-section container section"
      id="faq"
      aria-labelledby="faq-title"
    >
      <div className="faq-intro">
        <span className="eyebrow section-eyebrow">
          <span /> A FEW GOOD QUESTIONS
        </span>
        <h2 id="faq-title">
          Before you <br />
          bring the team in.
        </h2>
        <p>Still curious? Everything is in the docs.</p>
        <a
          className="text-link"
          href={`${DOCS}/cli.md`}
          target="_blank"
          rel="noreferrer"
        >
          Explore the documentation <ArrowUpRight size={16} />
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
              <p>{answer}</p>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}

function Footer() {
  return (
    <footer className="site-footer">
      <div className="container">
        <div className="footer-top">
          <div>
            <Brand />
            <p>Good agents. Better together.</p>
          </div>
          <div className="footer-links">
            <a href={`${DOCS}/cli.md`} target="_blank" rel="noreferrer">
              Documentation <ArrowUpRight size={13} />
            </a>
            <a href={`${REPO}/releases`} target="_blank" rel="noreferrer">
              Releases <ArrowUpRight size={13} />
            </a>
            <a href={DOCKER_HUB} target="_blank" rel="noreferrer">
              Docker Hub <ArrowUpRight size={13} />
            </a>
            <a href={REPO} target="_blank" rel="noreferrer">
              <GitFork size={16} /> GitHub
            </a>
          </div>
          <a href="#top" className="back-to-top" aria-label="Back to top">
            <ArrowUpRight size={19} />
          </a>
        </div>
        <div className="footer-bottom">
          <span>Built for the builders. Multiharness Core.</span>
          <span>
            <span className="green-dot" /> Runs locally. Works together.
          </span>
        </div>
      </div>
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
        <IntegrationStrip />
        <WorkflowSection />
        <WhySection />
        <Roadmap />
        <GettingStarted />
        <FAQ />
      </main>
      <Footer />
    </>
  );
}
