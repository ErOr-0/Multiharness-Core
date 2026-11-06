import { useState } from "react";
import {
  ArrowRight,
  ArrowUpRight,
  FolderOpen,
  KeyRound,
  Package,
  Terminal,
} from "lucide-react";
import CommandBlock from "./CommandBlock.jsx";
import Inline from "./Inline.jsx";
import {
  agentNamed,
  agents,
  implementersFor,
  platforms,
  routers,
  setupCommand,
  setupError,
  signInCommand,
  sourceInstallCommand,
  wizardPlanners,
} from "../bridge-setup.js";
import { DOCS, docs } from "../content.js";
import "./SetupGuide.css";

function detectPlatform() {
  try {
    return /Windows/i.test(navigator.userAgent) ? "windows" : "unix";
  } catch {
    return "unix";
  }
}

const plannerNames = wizardPlanners.map((agent) => agent.title).join(" · ");
const implementerNames = agents.map((agent) => agent.title).join(" · ");

export default function SetupGuide() {
  const [platformId, setPlatformId] = useState(detectPlatform);
  const [feedback, setFeedback] = useState("");
  const [choice, setChoice] = useState({
    planner: "claude",
    implementer: "codex",
    router: "",
  });
  const platform =
    platforms.find((item) => item.id === platformId) ?? platforms[0];
  const error = setupError(choice);
  const command = setupCommand(choice);
  const planner = agentNamed(choice.planner);
  function update(field, value) {
    setChoice((previous) => {
      const next = { ...previous, [field]: value };
      // Claude Code can't implement for itself, so it isn't offered then.
      if (next.planner === "claude" && next.implementer === "claude")
        next.implementer = "codex";
      return next;
    });
    setFeedback("");
  }

  return (
    <section
      className="section container setup-section"
      id="start"
      aria-labelledby="start-title"
    >
      <div className="download-heading">
        <div>
          <span className="eyebrow">GET STARTED</span>
          <h2 id="start-title">
            One line to install.
            <br />
            <span>Three questions to answer.</span>
          </h2>
          <p>No Go, no per-project files. The setup wizard does the rest.</p>
        </div>
        <a
          className="release-stamp"
          href={docs.install}
          target="_blank"
          rel="noreferrer"
        >
          <span className="green-dot" /> Full install guide
          <ArrowUpRight size={16} />
        </a>
      </div>
      <section
        className="requirements-guide"
        id="requirements"
        aria-labelledby="requirements-title"
      >
        <div className="requirements-heading">
          <h3 id="requirements-title">What you’ll need</h3>
          <span>macOS · Windows · Linux</span>
        </div>
        <div className="requirements-grid">
          <article>
            <Terminal size={22} />
            <h4>A terminal</h4>
            <p>PowerShell on Windows, Terminal on macOS or Linux.</p>
          </article>
          <article>
            <KeyRound size={22} />
            <h4>Accounts for two agents</h4>
            <p>
              The wizard installs a missing agent CLI and opens its sign-in in
              your browser.
            </p>
          </article>
          <article>
            <Package size={22} />
            <h4>npm for some agents</h4>
            <p>
              Codex CLI, OpenCode and Command Code use npm installers on most
              systems, so have <a href="https://nodejs.org/">Node.js</a> ready
              if you pick one.
            </p>
          </article>
        </div>
      </section>
      <div className="quick-install">
        <div className="quick-install-heading">
          <span className="eyebrow">INSTALL · ANSWER · ASK</span>
          <div className="platform-tabs" role="group" aria-label="Your system">
            {platforms.map((item) => (
              <button
                key={item.id}
                aria-pressed={item.id === platform.id}
                onClick={() => {
                  setPlatformId(item.id);
                  setFeedback("");
                }}
              >
                {item.label}
              </button>
            ))}
          </div>
        </div>
        <ol className="install-steps">
          <li>
            <h3>
              <span>1</span> Run the install line
            </h3>
            <CommandBlock
              title="Install line"
              value={platform.install}
              onCopied={setFeedback}
            />
            <p className="install-hint">
              Paste it into {platform.shell}. It downloads the prebuilt
              harness-bridge, checks its SHA-256 checksum and starts setup.{" "}
              <Inline text={platform.note} />
            </p>
          </li>
          <li>
            <h3>
              <span>2</span> Answer three questions
            </h3>
            <ol className="wizard-questions" aria-label="Setup questions">
              <li>
                <strong>Which agent should plan and review your work?</strong>
                <span>{plannerNames}</span>
              </li>
              <li>
                <strong>Which agent should write the code?</strong>
                <span>{implementerNames}</span>
              </li>
              <li>
                <strong>Route requests through Laya or Jev first?</strong>
                <span>Optional · default no</span>
              </li>
            </ol>
            <p>
              Then it shows its plan and asks one <code>Continue? [Y/n]</code>{" "}
              before changing anything.
            </p>
          </li>
          <li>
            <h3>
              <span>3</span> Open your planner and ask
            </h3>
            <p>
              Missing agents install in the background, sign-in opens in your
              browser where needed, and harness-bridge connects to your planner
              as an MCP server for all your projects.
            </p>
            <p>
              Then open your planner in any project and ask for a change. It
              plans, hands the coding to your implementer and reviews the
              result.
            </p>
            <p className="install-hint">
              Nothing is written into your projects. Run{" "}
              <code>harness-bridge</code> again to change your choices.
            </p>
          </li>
        </ol>
        <div className="setup-more">
          <details>
            <summary>Set up without questions</summary>
            <p>
              For scripts and agents: the same choices as flags. Add{" "}
              <code>--yes</code> only after you’ve agreed to run any installer
              it shows.
            </p>
            <div className="team-setup-fields">
              <label htmlFor="setup-planner">Planner</label>
              <select
                id="setup-planner"
                value={choice.planner}
                onChange={(event) => update("planner", event.target.value)}
              >
                {wizardPlanners.map((agent) => (
                  <option key={agent.name} value={agent.name}>
                    {agent.title}
                  </option>
                ))}
              </select>
              <label htmlFor="setup-implementer">Implementer</label>
              <select
                id="setup-implementer"
                value={choice.implementer}
                onChange={(event) => update("implementer", event.target.value)}
              >
                {implementersFor(choice.planner).map((agent) => (
                  <option key={agent.name} value={agent.name}>
                    {agent.title}
                  </option>
                ))}
              </select>
            </div>
            <div className="setup-options">
              <label htmlFor="setup-router">Route requests first</label>
              <select
                id="setup-router"
                value={choice.router}
                onChange={(event) => update("router", event.target.value)}
              >
                {routers.map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </div>
            {command ? (
              <>
                <CommandBlock
                  title="Setup command"
                  value={command}
                  onCopied={setFeedback}
                />
                <p className="install-hint setup-mode">
                  Connects {planner.title} for all projects and saves the
                  choices, like the wizard does.
                </p>
                <CommandBlock
                  title="Sign in once"
                  value={signInCommand(choice)}
                  onCopied={setFeedback}
                />
              </>
            ) : (
              <p className="launch-placeholder" role="alert">
                {error}
              </p>
            )}
          </details>
          <details>
            <summary>Build from source</summary>
            <p>
              Needs <a href="https://go.dev/doc/install">Go</a> and Git. The
              last line starts the same setup wizard.
            </p>
            <CommandBlock
              title="Build from source"
              value={sourceInstallCommand}
              onCopied={setFeedback}
            />
            <p className="install-hint">
              <code>$(go env GOPATH)/bin</code> must be on your{" "}
              <code>PATH</code>.
            </p>
          </details>
        </div>
        <div className="quick-install-footer">
          <span>
            <FolderOpen size={13} aria-hidden="true" /> Choices are saved once,
            for every project
          </span>
          <a href={DOCS} target="_blank" rel="noreferrer">
            Options &amp; troubleshooting <ArrowRight size={15} />
          </a>
        </div>
        <p className="setup-feedback" role="status">
          {feedback}
        </p>
      </div>
    </section>
  );
}
