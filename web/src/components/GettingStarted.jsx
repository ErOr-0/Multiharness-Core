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
            Connect your original project folder once. Multiharness edits those
            files directly, while Docker keeps the tools, logins and settings.
            No project copies. No launcher scripts.
          </p>
          <ol className="docker-steps">
            <li>
              <strong>Start Docker Desktop.</strong> Windows uses Linux
              containers. Linux Engine users: follow the{" "}
              <a href={`${DOCS}/docker.md#linux-apparmor-setup`}>
                host setup guide
              </a>{" "}
              first.
            </li>
            <li>
              <strong>Choose your parent folder below.</strong> Use an existing
              folder containing one or several projects. Git is optional.
            </li>
            <li>
              <strong>Download and extract the configuration.</strong> Keep its
              Compose file and sandbox policy together. Open a terminal in the
              extracted folder.
            </li>
            <li>
              <strong>Sign in once, then start.</strong> Choose your workspace
              before sending a task. Use <code>/config</code> for your team and
              folder, <code>/workspace</code> to switch folders, and{" "}
              <code>/save</code> to remember your team.
            </li>
          </ol>
          <div className="launcher-callout">
            <strong>One folder mapping, direct edits.</strong>
            <br />
            <code>
              {folder ||
                (platform === "Windows" ? "D:/Projects" : "/home/you/Projects")}
            </code>{" "}
            on your computer → <code>/workspace</code> in Docker. Selecting{" "}
            <code>api</code> works in that original subfolder.
          </div>
          <a className="start-docs" href={`${DOCS}/docker.md`}>
            Detailed setup and updates <ArrowRight size={15} />
          </a>
          <a className="start-docs" href={DOCKER_HUB}>
            View image on Docker Hub <ArrowUpRight size={15} />
          </a>
          <div className="early-access-note">
            Preview · amd64 + arm64 · macOS Docker testing pending
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
            <label htmlFor="host-folder">
              Existing folder on your computer
            </label>
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
              Paste the full path from your file manager. It stays in your
              browser and downloaded configuration; it is not sent to our
              server.
            </p>
            <button
              className="button button-lime"
              disabled={busy || !folder.trim()}
              onClick={download}
            >
              {busy ? "Preparing…" : "Download Docker configuration"}
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
            <Terminal size={16} /> Terminal · inside the extracted configuration
            folder
          </p>
          {[
            { key: "setup", title: "1. First-time sign-in", label: "setup" },
            { key: "run", title: "2. Start Multiharness", label: "launch" },
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
            Reuse the second command for later sessions. Configuration and
            logins persist in <code>magent-state</code>. To update: exit, run{" "}
            <code>docker compose pull</code>, then start again.
          </p>
          <p className="setup-feedback" role="status">
            {feedback}
          </p>
        </div>
      </div>
      <div className="start-footnote">
        <span>
          This is a terminal application. Compose supplies the settings missing
          from Docker Desktop’s basic Run dialog.
        </span>
      </div>
    </section>
  );
}
