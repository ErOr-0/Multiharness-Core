import { useState } from "react";
import { ArrowRight, ArrowUpRight, Copy } from "lucide-react";
import { DOCS } from "../content.js";

export default function GettingStarted({ initialPlatform = "Windows" } = {}) {
  const [platform, setPlatform] = useState(initialPlatform);
  const [arch, setArch] = useState("amd64");
  const [feedback, setFeedback] = useState("");
  const system = { Windows: "windows", macOS: "darwin", Linux: "linux" }[
    platform
  ];
  const commands =
    platform === "Windows"
      ? [
          { title: "Install once", value: ".\\magent.exe --install" },
          {
            title: "Choose folder, models and accounts",
            value: "magent --config",
          },
          { title: "Start working", value: "magent" },
        ]
      : [
          { title: "Make the launcher executable", value: "chmod +x magent" },
          {
            title: "Choose folder, models and accounts",
            value: "./magent --config",
          },
          { title: "Start working", value: "./magent" },
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
            <span className="green-dot" /> ONE COMMAND. YOUR OWN FOLDER.
          </span>
          <h2 id="start-title">
            Choose a folder.
            <br />
            Let your team work.
          </h2>
          <p>
            Magent runs on your computer and connects Docker to the folder you
            choose. Agents edit your original files. Logins and tools stay in
            Docker.
          </p>
          <p className="launcher-callout">
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
              <strong>Download the Magent launcher.</strong> Extract the ZIP and
              open a terminal in that folder.
            </li>
            <li>
              <strong>Run the configuration menu.</strong> Choose Folder, Models
              or Accounts. Enter any existing full folder path on your computer.
            </li>
            <li>
              <strong>Start Magent.</strong> Docker downloads the image if
              needed and connects the folder automatically.
            </li>
          </ol>
          <p>
            No Compose files to edit. To change folders later, exit and run{" "}
            <code>magent --config</code> again.
          </p>
          <a className="start-docs" href={`${DOCS}/host-launcher.md`}>
            Step-by-step help <ArrowRight size={15} />
          </a>
          <div className="early-access-note">
            Preview
            {platform === "macOS" ? " · macOS Docker testing pending" : ""}
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
            <label htmlFor="processor">Your processor</label>
            <select
              id="processor"
              value={arch}
              onChange={(e) => setArch(e.target.value)}
            >
              <option value="amd64">Intel / AMD (x64)</option>
              <option value="arm64">
                {platform === "macOS" ? "Apple Silicon (M-series)" : "ARM64"}
              </option>
            </select>
            <a
              className="button button-lime"
              href={`/downloads/magent-host_${system}_${arch}.zip`}
              download
            >
              Download for {platform}
              <ArrowUpRight size={16} />
            </a>
            <p>Extract the ZIP. Open a terminal in the extracted folder.</p>
            {platform === "Linux" && (
              <p>
                Using AppArmor? Complete the{" "}
                <a href={`${DOCS}/host-launcher.md#macos-and-linux`}>
                  one-time Linux host setup
                </a>{" "}
                first.
              </p>
            )}
          </div>
          {commands.map(({ title, value }) => (
            <div className="launcher-command" key={title}>
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
              {title === "Install once" && (
                <p className="command-hint">
                  After installation, open a new terminal for the commands
                  below.
                </p>
              )}
            </div>
          ))}
          <details className="setup-details">
            <summary>Where files live and how updates work</summary>
            <p>
              The launcher stores your folder choice on your PC. Docker stores
              your logins and model settings in <code>magent-state</code>. Your
              source files stay in their original folder.
            </p>
            <p>
              Run <code>magent --update</code> to update the image. A Docker
              image download alone does not install a command on your PC; the
              launcher provides that command.
            </p>
            <a href={`${DOCS}/docker.md`}>
              Already using Compose? Advanced Docker guide
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
