import { useState } from "react";
import { ArrowRight, ArrowUpRight, Copy, Terminal } from "lucide-react";
import { DOCS, DOCKER_HUB, dockerCommands } from "../content.js";
import { downloadConfiguration } from "../docker-setup.js";

export default function GettingStarted({ initialPlatform = "Windows" } = {}) {
  const [platform, setPlatform] = useState(initialPlatform);
  const [folder, setFolder] = useState("");
  const [feedback, setFeedback] = useState("");
  const [busy, setBusy] = useState(false);
  const commands = dockerCommands[platform];
  async function download() {
    setBusy(true);
    setFeedback("");
    try {
      await downloadConfiguration(folder, platform);
      setFeedback(
        "Downloaded. Extract the ZIP, then open a terminal in that folder.",
      );
    } catch (error) {
      setFeedback(error.message);
    } finally {
      setBusy(false);
    }
  }
  async function copy(key) {
    try {
      await navigator.clipboard.writeText(commands[key]);
      setFeedback("Command copied.");
    } catch {
      setFeedback("Select and copy the command shown below.");
    }
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
            <span className="green-dot" /> YOUR FILES. YOUR COMPUTER.
          </span>
          <h2 id="start-title">
            Docker runs the tools.
            <br />
            Your code stays put.
          </h2>
          <p>
            Your code stays in its original folder. Docker takes care of the
            tools and remembers your login.
          </p>
          <p className="launcher-callout">
            <strong>Before you begin</strong>
            <br />
            Install and open{" "}
            <a href="https://docs.docker.com/get-started/get-docker/">
              Docker Desktop
            </a>
            . On Windows, use Linux containers.
          </p>
          <ol className="docker-steps">
            <li>
              <strong>Choose your folder.</strong> One project or a folder with
              several projects.
            </li>
            <li>
              <strong>Download your setup.</strong> Extract the ZIP and open a
              terminal there.
            </li>
            <li>
              <strong>Sign in and start.</strong> Pick a project when
              Multiharness opens.
            </li>
          </ol>
          <a className="start-docs" href={`${DOCS}/docker.md`}>
            Need help? Read the setup guide <ArrowRight size={15} />
          </a>
          <div className="early-access-note">
            Docker preview
            {platform === "macOS" ? " · macOS testing pending" : ""}
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
                onClick={() => {
                  setPlatform(item);
                  setFolder("");
                  setFeedback("");
                }}
              >
                {item}
              </button>
            ))}
          </div>
          <div className="docker-folder-form">
            <label htmlFor="host-folder">1. Your project folder</label>
            <input
              id="host-folder"
              value={folder}
              onChange={(e) => setFolder(e.target.value)}
              placeholder={
                platform === "Windows"
                  ? "D:\\Projects"
                  : platform === "macOS"
                    ? "/Users/you/Projects"
                    : "/home/you/Projects"
              }
              spellCheck={false}
              autoComplete="off"
              aria-describedby="folder-help"
            />
            <p id="folder-help">
              Paste the full folder path from your file manager. Your files stay
              where they are.
            </p>
            <button
              className="button button-lime"
              disabled={busy || !folder.trim()}
              onClick={download}
            >
              {busy ? "Preparing…" : "2. Download setup"}
              <ArrowUpRight size={16} />
            </button>
            {platform === "Linux" && (
              <p>
                Before starting,{" "}
                <a href={`${DOCS}/docker.md#linux-apparmor-setup`}>
                  load the scoped AppArmor profile and set your UID/GID
                </a>
                . Do not run as root.
              </p>
            )}
          </div>
          <p className="install-location">
            <Terminal size={16} />
            3. Extract the ZIP. Open a terminal in the extracted folder.
          </p>
          {[
            { key: "setup", title: "Sign in once", label: "setup" },
            { key: "run", title: "Start your session", label: "launch" },
          ].map(({ key, title, label }) => (
            <div className="launcher-command" key={key}>
              <div className="install-code-header">
                <h3>{title}</h3>
                <button
                  onClick={() => copy(key)}
                  aria-label={`Copy ${label} command`}
                >
                  <Copy size={14} />
                  Copy
                </button>
              </div>
              <pre
                className="install-code"
                tabIndex={0}
                role="region"
                aria-label={`${title} command`}
              >
                <code>{commands[key]}</code>
              </pre>
            </div>
          ))}
          <p className="install-next-run">
            Press Enter to use the shown folder, or use cd to browse first. Then
            type your task. Next time, use only{" "}
            <strong>Start your session</strong>.
          </p>
          <details className="setup-details">
            <summary>Files, settings and updates</summary>
            <p>
              In the folder browser, use <code>cd api</code> to open a folder,{" "}
              <code>cd ..</code> to go back, and <code>mkdir new-project</code>{" "}
              to create one. Press Enter to use the current folder.
            </p>
            <p>
              Your folder is shared directly with Docker at{" "}
              <code>/workspace</code>. No working copy or syncing is needed.
              Your folder path stays in your browser and downloaded setup.
            </p>
            <p>
              Use <code>/workspace</code> to switch projects,{" "}
              <code>/config</code> to choose your team, and <code>/save</code>{" "}
              to remember it.
            </p>
            <p>
              Logins and settings stay in the <code>magent-state</code> volume.
              Keep this volume when updating.
            </p>
            <p>
              To update: exit, run <code>docker compose pull</code>, then start
              your session again.
            </p>
            <a href={DOCKER_HUB}>
              View the Docker image <ArrowUpRight size={14} />
            </a>
          </details>
          <p className="setup-feedback" role="status">
            {feedback}
          </p>
        </div>
      </div>
      <div className="start-footnote">
        <span>
          Run the commands in your terminal while Docker Desktop is open.
        </span>
      </div>
    </section>
  );
}
