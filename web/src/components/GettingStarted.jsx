import { useState } from "react";
import { ArrowRight, Copy } from "lucide-react";
import { DOCS, dockerCommands } from "../content.js";
import {
  folderError,
  launchCommand,
  linuxPolicyCommand,
} from "../docker-install.js";

export default function GettingStarted({ initialPlatform = "Windows" } = {}) {
  const [platform, setPlatform] = useState(initialPlatform);
  const [feedback, setFeedback] = useState("");
  const windows = platform === "Windows";
  const linux = platform === "Linux";
  const [folder, setFolder] = useState("");
  const [appArmor, setAppArmor] = useState(true);
  const [showError, setShowError] = useState(false);
  const error = folderError(folder, platform);
  const launch = launchCommand(folder, platform, appArmor);
  async function copy(value, title) {
    try {
      await navigator.clipboard.writeText(value);
      setFeedback(`${title} copied.`);
    } catch {
      setFeedback("Select and copy the command above.");
    }
  }
  function command(title, value, expandable = false) {
    const code = (
      <pre
        className="install-code"
        tabIndex={0}
        role="region"
        aria-label={title}
      >
        <code>{value}</code>
      </pre>
    );
    return (
      <div className="docker-command">
        <div className="install-code-header">
          <h4>{title}</h4>
          <button
            onClick={() => copy(value, title)}
            aria-label={`Copy ${title}`}
          >
            <Copy size={14} />
            Copy
          </button>
        </div>
        {expandable ? (
          <details className="launch-command-details">
            <summary>View full Docker command</summary>
            {code}
          </details>
        ) : (
          code
        )}
      </div>
    );
  }
  return (
    <section className="section container" aria-labelledby="start-title">
      <div className="quick-install" id="start">
        <div className="quick-install-heading">
          <div>
            <span className="eyebrow">ONE CONTAINER · SETTINGS SAVED</span>
            <h2 id="start-title">Pull. Launch. Configure.</h2>
            <p>
              One container. Docker loads the configuration and security
              policies. No setup script or ZIP download.
            </p>
          </div>
          <div
            className="platform-tabs"
            role="group"
            aria-label="Installation platform"
          >
            {["macOS", "Linux", "Windows"].map((item) => (
              <button
                key={item}
                aria-pressed={item === platform}
                onClick={() => {
                  setPlatform(item);
                  setFeedback("");
                  setFolder("");
                  setShowError(false);
                }}
              >
                {item}
              </button>
            ))}
          </div>
        </div>
        <p className="quick-prerequisite">
          <strong>Before you start:</strong>{" "}
          {linux ? (
            <>
              <a href="https://docs.docker.com/engine/install/">
                Docker Engine with Compose
              </a>{" "}
              running as your regular user
            </>
          ) : (
            <>
              <a href="https://docs.docker.com/get-started/get-docker/">
                Docker Desktop
              </a>{" "}
              running{windows && " with Linux containers"}
            </>
          )}{" "}
          and <a href="https://git-scm.com/downloads">Git</a> installed. Use{" "}
          {windows ? "PowerShell" : "Terminal"}. Docker Compose fetches the
          configuration from our public GitHub repository.
        </p>
        <ol className="install-steps">
          <li>
            <h3>
              <span>1</span> Pull the image
            </h3>
            {command("Pull image", dockerCommands.pull)}
            <p>Download the app and agent tools.</p>
            {linux && (
              <div className="linux-install-policy">
                <label>
                  <input
                    type="checkbox"
                    checked={appArmor}
                    onChange={(e) => setAppArmor(e.target.checked)}
                  />{" "}
                  AppArmor host (e.g. Ubuntu)
                </label>
                {appArmor && (
                  <details>
                    <summary>Install Linux policy once</summary>
                    <p>
                      Run this before step 2. It loads only the Multiharness
                      profile and saves it for reboot. Docker’s other profiles
                      remain unchanged.
                    </p>
                    {command("Install Linux policy", linuxPolicyCommand)}
                  </details>
                )}
              </div>
            )}
          </li>
          <li>
            <h3>
              <span>2</span> Launch with your folder
            </h3>
            <label className="launch-folder-label" htmlFor="launch-folder">
              Full projects folder path
            </label>
            <input
              id="launch-folder"
              value={folder}
              placeholder={windows ? "D:\\Projects" : "/path/to/Projects"}
              onChange={(e) => {
                setFolder(e.target.value);
                setShowError(true);
              }}
              onBlur={() => setShowError(true)}
              aria-invalid={showError && !!error}
              aria-describedby="launch-folder-help"
              spellCheck={false}
              autoComplete="off"
            />
            <p id="launch-folder-help" className="install-hint">
              {showError && error
                ? error
                : "Stays in your browser. Docker shares this folder so edits appear on your computer."}
            </p>
            <p className="install-hint">
              Copy the command, paste into {windows ? "PowerShell" : "Terminal"}{" "}
              and run.
            </p>
            {launch ? (
              command("Create and open", launch, true)
            ) : (
              <p className="launch-placeholder">
                Enter a path to get your Docker command.
              </p>
            )}
          </li>
          <li>
            <h3>
              <span>3</span> Configure in the app
            </h3>
            <p>
              Choose a project inside your shared folder, then your agents and
              models. Completed settings save automatically.
            </p>
            <p>
              Sign in with <code>/login codex</code> (also{" "}
              <code>/login opencode</code> if selected), then type your task.
            </p>
            <p className="install-hint">
              Example: “Explain this project and how to run it.”
            </p>
          </li>
        </ol>
        <div className="quick-reopen">
          <div>
            <h3>Next time</h3>
            <p>Run from any folder. Your settings load automatically.</p>
          </div>
          {command("Start from any folder", dockerCommands.start)}
          <p className="quick-config">
            <code>/config</code> → <strong>1</strong> project ·{" "}
            <strong>2</strong> team
            <br />
            <code>/quit</code> to finish
          </p>
        </div>
        <div className="quick-install-footer">
          <span>Preview · uses your own provider accounts</span>
          <a href={`${DOCS}/docker.md`}>
            Updates &amp; troubleshooting <ArrowRight size={15} />
          </a>
        </div>
        <p className="setup-feedback" role="status">
          {feedback}
        </p>
      </div>
    </section>
  );
}
