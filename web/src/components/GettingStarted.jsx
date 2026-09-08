import { useState } from "react";
import { ArrowRight, ArrowUpRight, Copy } from "lucide-react";
import { DOCS, dockerCommands } from "../content.js";

export default function GettingStarted({ initialPlatform = "Windows" } = {}) {
  const [platform, setPlatform] = useState(initialPlatform);
  const [feedback, setFeedback] = useState("");
  const windows = platform === "Windows";
  const linux = platform === "Linux";
  const folder = windows
    ? "C:\\Users\\YOUR_NAME\\multiharness"
    : "~/multiharness";
  const setup = windows
    ? 'powershell -NoProfile -ExecutionPolicy Bypass -File "$env:USERPROFILE\\multiharness\\scripts\\setup.ps1"'
    : 'bash "$HOME/multiharness/scripts/setup.sh"';
  async function copy(value, title) {
    try {
      await navigator.clipboard.writeText(value);
      setFeedback(`${title} copied.`);
    } catch {
      setFeedback("Select and copy the command above.");
    }
  }
  function command(title, value) {
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
        <pre
          className="install-code"
          tabIndex={0}
          role="region"
          aria-label={title}
        >
          <code>{value}</code>
        </pre>
      </div>
    );
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
            <span className="green-dot" /> SET UP ONCE. SETTINGS REMEMBERED.
          </span>
          <h2 id="start-title">
            Run setup.
            <br />
            Answer the prompts.
          </h2>
          <p>
            First time? Setup asks for your projects folder, then the app guides
            you through your team. Your answers save automatically.
          </p>
          <div className="everyday-start">
            <h3>Already set up?</h3>
            <p>Open a terminal in any folder and run:</p>
            {command("Start from any folder", dockerCommands.start)}
            <p>
              Your saved project and team load automatically. Type{" "}
              <code>/quit</code> when finished.
            </p>
            <p>
              To change settings, type <code>/config</code>, then choose{" "}
              <strong>1 for the project folder</strong> or{" "}
              <strong>2 for the agent team</strong>.
            </p>
          </div>
          <a className="start-docs" href={`${DOCS}/docker.md`}>
            Full installation guide <ArrowRight size={15} />
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
          <div className="install-prerequisite">
            <strong>Before you start</strong>
            <p>
              {linux ? (
                <>
                  Install{" "}
                  <a href="https://docs.docker.com/engine/install/">
                    Docker Engine and the Compose plugin
                  </a>
                  . Make sure <code>docker info</code> works as your regular
                  user.
                </>
              ) : (
                <>
                  Install and open{" "}
                  <a href="https://docs.docker.com/get-started/get-docker/">
                    Docker Desktop
                  </a>
                  . Wait for the engine to start.
                  {windows && " Use Linux containers."}
                </>
              )}
            </p>
            <p>
              Use <strong>{windows ? "PowerShell" : "Terminal"}</strong> for the
              commands below.
            </p>
          </div>
          <ol className="install-steps">
            <li>
              <h3>
                <span>1</span> Download and extract
              </h3>
              <p>
                Extract the ZIP into <code>{folder}</code>
                {windows
                  ? "; replace YOUR_NAME with your computer username"
                  : "; ~ means your home folder"}
                .
              </p>
              <a
                className="button button-lime"
                href="/downloads/multiharness-docker.zip"
                download
              >
                Download setup <ArrowUpRight size={16} />
              </a>
              <p className="step-check">
                The <code>scripts</code> folder and <code>compose.yaml</code>{" "}
                should be directly inside <code>multiharness</code>.
              </p>
            </li>
            <li>
              <h3>
                <span>2</span> Run setup once
              </h3>
              {command("Run setup", setup)}
              <p>
                When asked, enter the full path to the folder containing your
                projects. Setup saves it, downloads the image and opens one
                container.
              </p>
              <p>
                {linux
                  ? "On AppArmor hosts, setup also asks for sudo to install the required container policy."
                  : "The first download can take a few minutes."}{" "}
                Keep this setup folder for updates.
              </p>
            </li>
            <li>
              <h3>
                <span>3</span> Follow the app's prompts
              </h3>
              <p>
                Choose your project folder, then your agents and models. Press
                Enter to keep a suggested value. The completed configuration
                saves automatically.
              </p>
              <p>
                Sign in at the app's prompt using <code>/login codex</code>. If
                you selected OpenCode, also use <code>/login opencode</code>.
                Follow the sign-in instructions shown.
              </p>
              <p>
                Then type your first task, for example:{" "}
                <strong>“Explain this project and how to run it.”</strong>
              </p>
              <p className="step-check">
                Next time, your saved settings load straight away. Use the
                everyday start command on this page.
              </p>
            </li>
          </ol>
          <details className="setup-details">
            <summary>Changing folders, reconnecting and updates</summary>
            <p>
              <code>/config</code> → <strong>1</strong> selects another project
              inside your shared folder. <code>/config</code> →{" "}
              <strong>2</strong> changes your team. Both save automatically.
            </p>
            <p>
              If already running, reconnect with{" "}
              <code>docker attach multiharness</code>.
            </p>
            <p>
              To update, type <code>/quit</code>, then run the setup command
              again. It reuses your saved folder and logins.
            </p>
            <p>
              Docker can access only the host folder you shared during setup. To
              share a different host folder, follow the folder-change
              instructions in the guide.
            </p>
            <a href={`${DOCS}/docker.md#updates`}>
              Update and folder-change guide
            </a>
          </details>
          <p className="setup-feedback" role="status">
            {feedback}
          </p>
        </div>
      </div>
    </section>
  );
}
