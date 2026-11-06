import { useState } from "react";
import {
  ArrowRight,
  ArrowUpRight,
  FolderOpen,
  KeyRound,
  Terminal,
} from "lucide-react";
import CommandBlock from "./CommandBlock.jsx";
import {
  agentNamed,
  agents,
  installCommand,
  routers,
  setupCommand,
  setupError,
  setupFiles,
  signInCommand,
} from "../bridge-setup.js";
import { docs } from "../content.js";
import "./SetupGuide.css";

export default function SetupGuide() {
  const [choice, setChoice] = useState({
    planner: "claude",
    implementer: "codex",
    router: "",
    lockPlanner: false,
  });
  const [feedback, setFeedback] = useState("");
  const claudePlanner = choice.planner === "claude";
  // The lock only exists for a Claude Code planner; keep the box's state but
  // drop the flag for any other planner.
  const effective = {
    ...choice,
    lockPlanner: choice.lockPlanner && claudePlanner,
  };
  const error = setupError(effective);
  const command = setupCommand(effective);
  const implementer = agentNamed(choice.implementer);
  function update(field, value) {
    setChoice((previous) => ({ ...previous, [field]: value }));
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
            Two agents. One bridge.
            <br />
            <span>Three steps to start.</span>
          </h2>
          <p>Install the command, pick who plans and who builds, sign in.</p>
        </div>
        <a
          className="release-stamp"
          href={docs.setup}
          target="_blank"
          rel="noreferrer"
        >
          <span className="green-dot" /> Full setup guide
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
            <h4>Go and Git</h4>
            <p>
              <a href="https://go.dev/doc/install">Go</a> to build the bridge,{" "}
              <a href="https://git-scm.com/downloads">Git</a> to clone it. In a
              Git repository, every report also carries what Git sees.
            </p>
          </article>
          <article>
            <KeyRound size={22} />
            <h4>Two agent CLIs</h4>
            <p>
              Any two of {agents.map((agent) => agent.title).join(", ")}. For a
              missing one, <code>setup</code> offers its official installer.
            </p>
          </article>
          <article>
            <FolderOpen size={22} />
            <h4>A project folder</h4>
            <p>
              <code>setup</code> writes the planner’s instructions there, next
              to your code.
            </p>
          </article>
        </div>
      </section>
      <div className="quick-install">
        <ol className="install-steps">
          <li>
            <h3>
              <span>1</span> Install the bridge
            </h3>
            <CommandBlock
              title="Install harness-bridge"
              value={installCommand}
              onCopied={setFeedback}
            />
            <p className="install-hint">
              <code>$(go env GOPATH)/bin</code> must be on your{" "}
              <code>PATH</code>, because the planner calls{" "}
              <code>harness-bridge</code> from its shell.
            </p>
          </li>
          <li>
            <h3>
              <span>2</span> Pick the pair
            </h3>
            <div className="team-setup-fields">
              <label htmlFor="setup-planner">Planner</label>
              <select
                id="setup-planner"
                value={choice.planner}
                onChange={(event) => update("planner", event.target.value)}
              >
                {agents.map((agent) => (
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
                {agents.map((agent) => (
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
              <label className="setup-check">
                <input
                  type="checkbox"
                  checked={effective.lockPlanner}
                  disabled={!claudePlanner}
                  onChange={(event) =>
                    update("lockPlanner", event.target.checked)
                  }
                />
                Lock the planner (Claude Code only)
              </label>
            </div>
            {command ? (
              <CommandBlock
                title="Setup command"
                value={command}
                onCopied={setFeedback}
              />
            ) : (
              <p className="launch-placeholder" role="alert">
                {error}
              </p>
            )}
            <p className="install-hint">
              Run it in your project folder, or add{" "}
              <code>--dir path/to/project</code>.
            </p>
          </li>
          <li>
            <h3>
              <span>3</span> Sign in and start
            </h3>
            {!error && (
              <>
                <CommandBlock
                  title="Sign in once"
                  value={signInCommand(choice)}
                  onCopied={setFeedback}
                />
                <p className="setup-files">
                  <code>setup</code> writes{" "}
                  {setupFiles(effective).map((file, index, files) => (
                    <span key={file}>
                      {index > 0 &&
                        (index === files.length - 1 ? " and " : ", ")}
                      <code>{file}</code>
                    </span>
                  ))}
                  .
                </p>
                <p>
                  Then ask the planner for a change. It plans, runs{" "}
                  <code>harness-bridge start --to {choice.implementer}</code>,
                  waits for the report and reviews the diff.
                  {implementer && !implementer.resume && (
                    <>
                      {" "}
                      {implementer.title} can’t continue a session, so fixes go
                      as a new job.
                    </>
                  )}
                </p>
              </>
            )}
            {error && (
              <p className="launch-placeholder">
                Choose two agents that can work together to see the sign-in
                commands.
              </p>
            )}
          </li>
        </ol>
        <div className="quick-install-footer">
          <span>setup asks before it installs anything</span>
          <a href={docs.setup} target="_blank" rel="noreferrer">
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
