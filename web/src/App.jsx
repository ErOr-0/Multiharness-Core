import GettingStarted from "./components/GettingStarted.jsx";
export { GettingStarted };
import { useEffect, useRef, useState } from "react";
import {
  Aperture,
  ArrowRight,
  ArrowUpRight,
  Check,
  CheckCheck,
  Code2,
  Copy,
  FileCode2,
  GitFork,
  Layers,
  Menu,
  Minus,
  Plus,
  ShieldCheck,
  SquareTerminal,
  Workflow,
  X,
  Zap,
} from "lucide-react";
import Brand, { BrandMark, DockerIcon, MetaIcon } from "./components/Brand.jsx";
import WorkflowDemo from "./components/WorkflowDemo.jsx";
import {
  DOCS,
  DOCKER_HUB,
  REPO,
  faqs,
  teamRecipes,
  workflowSteps,
} from "./content.js";
import { harnesses, teamLogins, teamRolesCommand } from "./team-settings.js";

const navigation = [
  ["#savings", "Why it saves"],
  ["#workflow", "The workflow"],
  ["#why", "Why Multiharness"],
  ["#requirements", "Requirements"],
  ["#faq", "FAQs"],
];

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
          <a className="button button-primary button-small" href="#download">
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
          <a className="hero-pill" href="#savings">
            <span className="pill-tag">New</span>
            Premium planning. Budget building.
            <ArrowRight size={14} />
          </a>
          <h1 id="hero-title">
            Frontier-quality code.
            <span className="hero-gradient">Without the frontier bill.</span>
          </h1>
          <p className="hero-description">
            Your best model plans and reviews. A cheaper model writes the code.
            Run it on your computer with Docker.
          </p>
          <div className="hero-actions">
            <a className="button button-primary" href="#download">
              Get Multiharness <DockerIcon />
            </a>
            <a className="button button-ghost" href="#workflow">
              See how it works <ArrowRight size={16} />
            </a>
          </div>
          <ul className="hero-proof" aria-label="Highlights">
            <li>
              <Check size={15} /> No extra subscription
            </li>
            <li>
              <Check size={15} /> Your accounts
            </li>
            <li>
              <Check size={15} /> Runs locally
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
  const tools = [
    [<Code2 key="i" />, "Claude Code"],
    [<Aperture key="i" />, "Codex"],
    [<SquareTerminal key="i" />, "OpenCode"],
    [<MetaIcon key="i" />, "Muse Code"],
  ];
  return (
    <div className="integration-strip">
      <div className="container integration-inner">
        <p>Works with</p>
        <ul>
          {tools.map(([icon, name]) => (
            <li className="integration-name" key={name}>
              {icon}
              {name}
            </li>
          ))}
        </ul>
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

const tiers = [
  {
    role: "Planner",
    tier: "Premium",
    share: "small",
    detail: "Writes a short plan.",
  },
  {
    role: "Implementer",
    tier: "Budget",
    share: "large",
    detail: "Reads, edits and repairs.",
  },
  {
    role: "Reviewer",
    tier: "Premium",
    share: "small",
    detail: "Checks the diff and tests.",
  },
];

function RecipeCard({ recipe }) {
  const [status, setStatus] = useState("");
  const command = teamRolesCommand(recipe.team);
  const logins = teamLogins(recipe.team);
  async function copy() {
    try {
      await navigator.clipboard.writeText(command);
      setStatus("Copied. Paste it into magent.");
    } catch {
      setStatus("Select the commands and copy them.");
    }
  }
  return (
    <article className="recipe-card">
      <div className="recipe-top">
        <h3>{recipe.name}</h3>
        <span>{recipe.pitch}</span>
      </div>
      <ol className="recipe-roles">
        {["planner", "implementer", "reviewer"].map((role) => {
          const pick = recipe.team[role];
          return (
            <li key={role}>
              <span>{role}</span>
              <strong>{harnesses[pick.harness]}</strong>
              <code>{pick.model}</code>
            </li>
          );
        })}
      </ol>
      <details className="recipe-command">
        <summary>View commands</summary>
        <pre tabIndex={0} aria-label={`${recipe.name} settings`}>
          <code>{command}</code>
        </pre>
      </details>
      <div className="recipe-actions">
        <button onClick={copy} aria-label={`Copy ${recipe.name} settings`}>
          <Copy size={14} /> Copy settings
        </button>
        <span>{logins.map((name) => `/login ${name}`).join(" · ")}</span>
      </div>
      <p className="recipe-status" role="status">
        {status}
      </p>
    </article>
  );
}

function Savings() {
  return (
    <section
      className="section savings-section"
      id="savings"
      aria-labelledby="savings-title"
    >
      <div className="container">
        <SectionHeading
          eyebrow="WHERE YOUR MONEY GOES"
          id="savings-title"
          title="Pay for judgment."
          accent="Not for keystrokes."
        >
          Premium models only where they change the outcome.
        </SectionHeading>
        <div className="tier-grid">
          {tiers.map((item) => (
            <article
              className={`tier-card tier-${item.tier.toLowerCase()}`}
              key={item.role}
            >
              <div className="tier-top">
                <h3>{item.role}</h3>
                <span className="tier-badge">{item.tier}</span>
              </div>
              <p>{item.detail}</p>
              <div className="tier-meter" aria-hidden="true">
                <span className={`tier-fill tier-fill-${item.share}`} />
              </div>
              <span className="tier-share">
                {item.share === "large" ? "Most tokens" : "Few tokens"}
              </span>
            </article>
          ))}
        </div>
        <p className="tier-note">Illustrative. Usage varies by task.</p>
        <div className="recipes">
          <h3 className="recipes-title">Ready-made teams</h3>
          <div className="recipe-grid">
            {teamRecipes.map((recipe) => (
              <RecipeCard recipe={recipe} key={recipe.id} />
            ))}
          </div>
        </div>
      </div>
    </section>
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
      id="workflow"
      aria-labelledby="workflow-title"
    >
      <div className="container">
        <SectionHeading
          eyebrow="THE TEAM WORKFLOW"
          id="workflow-title"
          title="Plan. Build. Check."
          accent="Repeat until right."
        />
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
            <p>{step.copy}</p>
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
            <div className="code-preview-footer">ILLUSTRATIVE OUTPUT</div>
          </div>
        </div>
        <div className="repair-loop">
          <Workflow size={16} />
          <p>
            <strong>Blocker found? It goes back with full context.</strong>
          </p>
        </div>
      </div>
    </section>
  );
}

const features = [
  [
    ShieldCheck,
    "Evidence over “looks good.”",
    "Review sees the real diff and your test results.",
  ],
  [
    Layers,
    "Context that fits small models.",
    "Compact handoffs, not your whole workspace.",
  ],
  [Zap, "Approve from one terminal.", "Agent permission requests come to you."],
  [
    CheckCheck,
    "Know where the work stands.",
    "Clear outcomes. A retry limit is never a success.",
  ],
];

function WhySection() {
  return (
    <section
      className="section why-section"
      id="why"
      aria-labelledby="why-title"
    >
      <div className="container">
        <SectionHeading
          eyebrow="WHY MULTIHARNESS"
          id="why-title"
          title="Cheaper models."
          accent="Same standard."
        />
        <div className="feature-grid">
          {features.map(([Icon, title, copy]) => (
            <article className="feature-card" key={title}>
              <span className="feature-icon">
                <Icon size={20} />
              </span>
              <h3>{title}</h3>
              <p>{copy}</p>
            </article>
          ))}
        </div>
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
          <a
            className="text-link"
            href={`${DOCS}#cli-and-configuration`}
            target="_blank"
            rel="noreferrer"
          >
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
                <p>{answer}</p>
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
        <h2 id="final-cta-title">Ship more. Spend less.</h2>
        <div className="hero-actions">
          <a className="button button-primary" href="#start">
            Install Multiharness <ArrowRight size={16} />
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
          <a
            href={`${DOCS}#cli-and-configuration`}
            target="_blank"
            rel="noreferrer"
          >
            Documentation
          </a>
          <a href={`${REPO}/releases`} target="_blank" rel="noreferrer">
            Releases
          </a>
          <a href={DOCKER_HUB} target="_blank" rel="noreferrer">
            Docker Hub
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
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <div id="top" />
      <Header />
      <main id="main-content">
        <Hero />
        <Savings />
        <WorkflowSection />
        <WhySection />
        <GettingStarted />
        <FAQ />
        <FinalCTA />
      </main>
      <Footer />
    </>
  );
}
