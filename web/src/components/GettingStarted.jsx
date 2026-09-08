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
    <section className="section container" aria-labelledby="start-title">
      <div className="quick-install" id="start">
        <div className="quick-install-heading">
          <div>
            <span className="eyebrow">ONE CONTAINER · SETTINGS SAVED</span>
            <h2 id="start-title">Install once. Reopen with Docker.</h2>
            <p>
              Setup downloads the image and applies the required Docker
              policies. No separate pull command needed.
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
              Install{" "}
              <a href="https://docs.docker.com/engine/install/">
                Docker Engine and Compose
              </a>
              ; make sure <code>docker info</code> works as your regular user.
              Use Terminal below.
            </>
          ) : (
            <>
              Open{" "}
              <a href="https://docs.docker.com/get-started/get-docker/">
                Docker Desktop
              </a>{" "}
              and wait until it is running.{" "}
              {windows
                ? "Use Linux containers and PowerShell below."
                : "Use Terminal below."}
            </>
          )}
        </p>
        <ol className="install-steps">
          <li>
            <h3>
              <span>1</span> Download
            </h3>
            <p>
              Extract the ZIP into <code>{folder}</code>
              {windows
                ? "; replace YOUR_NAME with your username"
                : "; ~ is your home folder"}
              .
            </p>
            <a
              className="button button-lime"
              href="/downloads/multiharness-docker.zip"
              download
            >
              Download setup <ArrowUpRight size={16} />
            </a>
            <p className="install-hint">
              The <code>scripts</code> folder should be directly inside{" "}
              <code>multiharness</code>. Keep this folder for updates.
            </p>
          </li>
          <li>
            <h3>
              <span>2</span> Install &amp; open
            </h3>
            {command("Run setup", setup)}
            <p>
              Paste into {windows ? "PowerShell" : "Terminal"}. Enter the full
              path to your projects folder when asked. Setup saves it and opens
              one container.
            </p>
            {linux && (
              <p className="install-hint">
                On AppArmor hosts, approve the sudo prompt to install the
                required policy.
              </p>
            )}
          </li>
          <li>
            <h3>
              <span>3</span> Configure &amp; work
            </h3>
            <p>
              In the app, choose a project inside your shared folder, then your
              agents and models. Settings save automatically.
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
