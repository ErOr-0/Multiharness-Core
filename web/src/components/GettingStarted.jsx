import { useState } from "react";
import { ArrowRight, ArrowUpRight, Copy } from "lucide-react";
import { DOCS, dockerCommands } from "../content.js";

export default function GettingStarted({ initialPlatform = "Windows" } = {}) {
  const [platform, setPlatform] = useState(initialPlatform);
  const [feedback, setFeedback] = useState("");
  const [appArmor, setAppArmor] = useState(true);
  const windows = platform === "Windows";
  const linux = platform === "Linux";
  const folder = windows
    ? "C:\\Users\\YOUR_NAME\\multiharness"
    : "~/multiharness";
  const base = windows
    ? "$env:USERPROFILE\\multiharness"
    : "$HOME/multiharness";
  const compose = `docker compose --env-file "${base}${windows ? "\\" : "/"}.env" -f "${base}${windows ? "\\" : "/"}compose.yaml"`;
  const create = `${compose}${linux && appArmor ? ' -f "$HOME/multiharness/docker/compose.linux.yaml"' : ""} up --no-start`;
  const edit = windows
    ? 'if (!(Test-Path "$env:USERPROFILE\\multiharness\\.env")) { Copy-Item "$env:USERPROFILE\\multiharness\\.env.example" "$env:USERPROFILE\\multiharness\\.env" }\nnotepad "$env:USERPROFILE\\multiharness\\.env"'
    : 'test -f "$HOME/multiharness/.env" || cp "$HOME/multiharness/.env.example" "$HOME/multiharness/.env"\n' +
      (linux
        ? 'nano "$HOME/multiharness/.env"'
        : 'open -e "$HOME/multiharness/.env"');
  const example = windows
    ? "D:/Projects"
    : linux
      ? "/home/YOUR_NAME/Projects"
      : "/Users/YOUR_NAME/Projects";
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
            <span className="green-dot" /> ONE CONTAINER. YOUR OWN FOLDER.
          </span>
          <h2 id="start-title">
            Your first task,
            <br />
            step by step.
          </h2>
          <p>
            Choose your operating system and follow steps 1–4 once. You run the
            commands on your own computer. Your projects stay in their original
            folder.
          </p>
          <div className="everyday-start">
            <h3>Already set up?</h3>
            <p>Open a terminal in any folder and run:</p>
            {command("Start from any folder", dockerCommands.start)}
            <p>
              Finished working? Type <code>/quit</code>. Next time, use this
              same start command.
            </p>
            <p>
              If the container is already running, reconnect with{" "}
              <code>docker attach multiharness</code>.
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
            <strong>Before step 1</strong>
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
                  . Wait until it says the engine is running.
                  {windows && " Use Linux containers."}
                </>
              )}
            </p>
            <p>
              Use <strong>{windows ? "PowerShell" : "Terminal"}</strong> for the
              commands below. You can stay in any folder.
            </p>
          </div>
          <ol className="install-steps">
            <li>
              <h3>
                <span>1</span> Download and extract
              </h3>
              <p>
                Download the ZIP, extract it, and put its contents in{" "}
                <code>{folder}</code>. Replace <code>YOUR_NAME</code> with your
                computer username
                {!windows && (
                  <>
                    ; <code>~</code> means your home folder
                  </>
                )}
                .
              </p>
              <a
                className="button button-lime"
                href="/downloads/multiharness-docker.zip"
                download
              >
                Download configuration <ArrowUpRight size={16} />
              </a>
              <p className="step-check">
                Check: <code>compose.yaml</code> must be directly inside your{" "}
                <code>multiharness</code> folder, not inside another nested
                folder.
              </p>
            </li>
            <li>
              <h3>
                <span>2</span> Choose the folder to share
              </h3>
              <p>
                Run this to create and open your settings file. If you already
                have one, it opens your existing settings.
              </p>
              {command("Open settings", edit)}
              <p>
                In the editor, replace the <code>MULTIHARNESS_WORKSPACE</code>{" "}
                line with the full path to an{" "}
                <strong>existing project folder</strong>. For example:
              </p>
              <pre className="settings-example">
                <code>MULTIHARNESS_WORKSPACE='{example}'</code>
              </pre>
              <p>
                Use your real folder path, keep the single quotes, and save the
                file.{" "}
                {windows
                  ? "Use forward slashes as shown. Save as .env, not .env.txt."
                  : "The file is named .env."}{" "}
                A parent folder can hold several projects. Changes made by
                agents appear in that folder.
              </p>
              {linux ? (
                <p>
                  Set <code>MULTIHARNESS_UID</code> and{" "}
                  <code>MULTIHARNESS_GID</code> to the numbers shown by{" "}
                  <code>id -u</code> and <code>id -g</code>. In nano, press
                  Ctrl+O, Enter, then Ctrl+X to save and exit.
                </p>
              ) : (
                <p>
                  Leave the two UID/GID values at <code>1000</code>, then close
                  the editor.
                </p>
              )}
            </li>
            <li>
              <h3>
                <span>3</span> Create and start your container
              </h3>
              {linux && (
                <div className="linux-policy">
                  <label>
                    <input
                      type="checkbox"
                      checked={appArmor}
                      onChange={(event) => setAppArmor(event.target.checked)}
                    />{" "}
                    This Linux host uses AppArmor
                  </label>
                  <p>
                    Check <code>docker info</code> → Security Options. Keep this
                    checked if it lists <code>apparmor</code> (common on
                    Ubuntu). Otherwise, uncheck it. The commands below adjust to
                    your choice.
                  </p>
                  {appArmor &&
                    command(
                      "Install Linux policy once",
                      'sudo sh "$HOME/multiharness/scripts/magent-apparmor.sh"',
                    )}
                </div>
              )}
              <p>
                Copy and run each command in order. The first download can take
                a few minutes.
              </p>
              {command("Download the image", dockerCommands.pull)}
              {command("Create once", create)}
              {command("Start your team", dockerCommands.start)}
              <p className="step-check">
                You should see the Multiharness welcome screen and a folder
                picker. You now have one container named{" "}
                <code>multiharness</code>.
              </p>
            </li>
            <li>
              <h3>
                <span>4</span> Sign in and send your first task
              </h3>
              <p>
                Press Enter to use the shared folder, or choose a project inside
                it. At the app's prompt, enter these commands one at a time:
              </p>
              <dl className="onboarding-commands">
                <div>
                  <dt>
                    <code>/login codex</code>
                  </dt>
                  <dd>Follow the sign-in instructions shown.</dd>
                </div>
                <div>
                  <dt>
                    <code>/config</code>
                  </dt>
                  <dd>
                    Choose your agents and models. Sign in with{" "}
                    <code>/login opencode</code> too if you select OpenCode.
                  </dd>
                </div>
                <div>
                  <dt>
                    <code>/save</code>
                  </dt>
                  <dd>Remember your team for next time.</dd>
                </div>
              </dl>
              <p>
                Then type a task, for example:{" "}
                <strong>“Explain this project and how to run it.”</strong>
              </p>
              <p className="step-check">
                Your selected project, logins and settings are remembered. Use{" "}
                <code>/workspace</code> to switch projects later.
              </p>
            </li>
          </ol>
          <details className="setup-details">
            <summary>Need help or want to update?</summary>
            <p>
              <strong>Cannot connect to Docker?</strong> Open Docker Desktop or
              start Docker Engine and try again.
            </p>
            <p>
              <strong>Settings or folder not found?</strong> Check the
              extraction location in step 1 and the real project path in step 2.
            </p>
            <p>
              <strong>Already have a container?</strong> Use the everyday start
              command. If running, use <code>docker attach multiharness</code>.
            </p>
            <p>
              <strong>Updating?</strong> Type <code>/quit</code>, then repeat
              step 3 with your saved settings. Your logins remain in the{" "}
              <code>magent-state</code> volume. Keep the configuration folder
              for updates.
            </p>
            <a href={`${DOCS}/docker.md#updates`}>
              Update and troubleshooting guide
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
