import { useState } from "react";
import { ArrowRight, ArrowUpRight, Copy } from "lucide-react";
import { DOCS, dockerCommands } from "../content.js";

export default function GettingStarted({ initialPlatform = "Windows" } = {}) {
  const [platform, setPlatform] = useState(initialPlatform);
  const [feedback, setFeedback] = useState("");
  const setup =
    platform === "Windows"
      ? 'docker compose --env-file "$env:USERPROFILE\\multiharness\\.env" -f "$env:USERPROFILE\\multiharness\\compose.yaml" up --no-start'
      : 'docker compose --env-file "$HOME/multiharness/.env" -f "$HOME/multiharness/compose.yaml" up --no-start';
  const commands = [
    { title: "Download the image", value: dockerCommands.pull },
    { title: "Create once", value: setup },
    { title: "Start from any folder", value: dockerCommands.start },
  ];
  async function copy(value) {
    try {
      await navigator.clipboard.writeText(value);
      setFeedback("Copied.");
    } catch {
      setFeedback("Select and copy the command above.");
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
            <span className="green-dot" /> ONE CONTAINER. YOUR OWN FOLDER.
          </span>
          <h2 id="start-title">
            Set up once.
            <br />
            Start from anywhere.
          </h2>
          <p>
            Run your agent team in one reusable Docker container. Your projects
            stay in their original folder. Accounts and settings are remembered.
          </p>
          <p className="docker-callout">
            <strong>Before you begin</strong>
            <br />
            Install and open{" "}
            <a href="https://docs.docker.com/get-started/get-docker/">
              Docker Desktop
            </a>
            . Windows uses Linux containers.
          </p>
          <ol className="docker-steps">
            <li>
              <strong>Download the configuration.</strong> Extract it to the
              multiharness folder in your user home. Copy .env.example to .env
              and enter your existing project folder's absolute path.
            </li>
            <li>
              <strong>Create one container.</strong> Run the commands shown.
              Pull downloads the image; creation records your folder and
              settings volume once.
            </li>
            <li>
              <strong>Reopen it from anywhere.</strong> Start the same container
              whenever you need it. No host launcher or project-directory
              change.
            </li>
          </ol>
          <p>
            Inside the prompt, use <code>/login codex</code> or{" "}
            <code>/login opencode</code>, then <code>/config</code> and{" "}
            <code>/save</code>. Use <code>/workspace</code> to switch projects
            inside your shared folder.
          </p>
          <a className="start-docs" href={`${DOCS}/docker.md`}>
            Step-by-step help <ArrowRight size={15} />
          </a>
          <div className="early-access-note">
            Preview · uses your own provider accounts
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
                aria-pressed={item === platform}
                onClick={() => {
                  setPlatform(item);
                  setFeedback("");
                }}
              >
                {item}
              </button>
            ))}
          </div>
          <div className="docker-folder-form">
            <a
              className="button button-lime"
              href="/downloads/multiharness-docker.zip"
              download
            >
              Download configuration <ArrowUpRight size={16} />
            </a>
            <p>
              The same configuration supports Intel/AMD and ARM. Docker chooses
              the image for your processor.
            </p>
            {platform === "Linux" && (
              <p>
                Set your UID/GID in .env. If your host uses AppArmor, follow the{" "}
                <a href={`${DOCS}/docker.md#linux-apparmor-setup`}>
                  Linux policy setup
                </a>{" "}
                and include its override when creating the container.
              </p>
            )}
          </div>
          {commands.map(({ title, value }) => (
            <div className="docker-command" key={title}>
              <div className="install-code-header">
                <h3>{title}</h3>
                <button
                  onClick={() => copy(value)}
                  aria-label={`Copy ${title}`}
                >
                  <Copy size={14} />
                  Copy
                </button>
              </div>
              <pre
                className="install-code"
                tabIndex={0}
                role="region"
                aria-label={title}
              >
                <code>{value}</code>
              </pre>
            </div>
          ))}
          <details className="setup-details">
            <summary>Reuse, updates and saved files</summary>
            <p>
              <code>/quit</code> stops the application and keeps the container.
              If it is already running, use{" "}
              <code>docker attach multiharness</code>. Use one interactive
              connection at a time.
            </p>
            <p>
              Your logins and settings live in <code>magent-state</code>. To
              update, exit, pull the new image and repeat the creation command,
              then start again. The named container is replaced while your
              volume and original files are retained.
            </p>
            <a href={`${DOCS}/docker.md#updates`}>Update and migration guide</a>
          </details>
          <p className="setup-feedback" role="status">
            {feedback}
          </p>
        </div>
      </div>
    </section>
  );
}
