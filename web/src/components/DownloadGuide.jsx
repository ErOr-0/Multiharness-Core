import {
  ArrowUpRight,
  Download,
  FolderOpen,
  KeyRound,
  Terminal,
} from "lucide-react";
import { DOCS, RELEASE } from "../content.js";
import "./DownloadGuide.css";

export default function DownloadGuide() {
  return (
    <div className="download-guide" id="download">
      <div className="download-heading">
        <div>
          <span className="eyebrow">GET MULTIHARNESS</span>
          <h2>
            Everything you need.
            <br />
            <span>One place to start.</span>
          </h2>
          <p>
            Bring a project and a provider account. We’ve packaged the rest.
          </p>
        </div>
        <a className="release-stamp" href={RELEASE.url}>
          <span className="green-dot" /> {RELEASE.version}
          <span className="release-preview">Preview</span>
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
            <h4>Docker + Git</h4>
            <p>
              <a href="https://docs.docker.com/get-started/get-docker/">
                Docker Desktop
              </a>{" "}
              on macOS/Windows or{" "}
              <a href="https://docs.docker.com/engine/install/">
                Docker Engine with Compose
              </a>{" "}
              on Linux, plus <a href="https://git-scm.com/downloads">Git</a>.
            </p>
          </article>
          <article>
            <KeyRound size={22} />
            <h4>Your provider account</h4>
            <p>
              Codex, Claude Code and Muse Code are already included in the
              image. Sign in to the ones you choose; usage follows your plan.
            </p>
          </article>
          <article>
            <FolderOpen size={22} />
            <h4>A project folder</h4>
            <p>Any folder on your computer. Git is optional.</p>
          </article>
        </div>
        <details className="native-setup-note">
          <summary>Running natively instead of Docker?</summary>
          <p>
            On macOS and Linux, Multiharness can offer to install a missing
            selected agent through npm, with your confirmation. Node.js and npm
            must be available. After installation, sign in and rerun your task.
            Explicit executable paths are not replaced, and native Windows
            automatic installation is not supported.
          </p>
          <a href={`${DOCS}#development-and-verification`}>
            Build and native setup guide <ArrowUpRight size={14} />
          </a>
        </details>
        <p className="download-file-links">
          <span>Optional, the setup below fetches these for you:</span>
          <a href={RELEASE.bundle}>
            Download configuration ZIP <Download size={13} />
          </a>
          <a href={RELEASE.checksums}>
            Checksums <ArrowUpRight size={13} />
          </a>
        </p>
      </section>
    </div>
  );
}

export function JevGuide({ command }) {
  return (
    <details className="jev-guide">
      <summary>Optional: route requests with Jev or Laya</summary>
      <div className="jev-copy">
        <h3>One request. Three clear routes.</h3>
        <p>
          A decision model decides how to handle your request before a Team
          agent starts: TypeSafe’s hosted Jev, or the open Laya model on a
          server you host. It’s off by default; Direct and Team workflows also
          work without it.
        </p>
        <dl className="jev-routes">
          <div>
            <dt>
              <span>01</span> Answer a question
            </dt>
            <dd>
              Your planner agent inspects the project read-only and answers. No
              implementation or review stages run.
            </dd>
          </div>
          <div>
            <dt>
              <span>02</span> Plan a change
            </dt>
            <dd>
              Your planner works through the requested change before the
              implementer starts.
            </dd>
          </div>
          <div>
            <dt>
              <span>03</span> Implement directly
            </dt>
            <dd>
              A clear, simple change goes to the implementer. Configured
              validation and review still follow.
            </dd>
          </div>
        </dl>
        <p className="jev-route-note">
          See the route and confidence in terminal progress. If routing fails or
          is uncertain, the workflow falls back to read-only assessment.
        </p>
      </div>
      <div className="jev-setup">
        <h4>Enable it inside the terminal app</h4>
        {command(
          "Enable Jev routing",
          "/set mode team\n/set decision-enabled true\n/save",
        )}
        {command(
          "Enable self-hosted Laya routing",
          "/set mode team\n/set decision-enabled true\n/set decision-provider laya\n/save",
        )}
        <ul>
          <li>
            <strong>Jev: your OpenRouter key.</strong> During setup or before
            your next task, the app asks for it with hidden input if it isn’t
            already configured. Enter it in the app, never on this website.
          </li>
          <li>
            <strong>Laya: your own server.</strong> Run a Jev-compatible Laya
            container (Docker works) and the app talks to it at{" "}
            <code>http://127.0.0.1:8765/v1/systemone</code> by default. Change{" "}
            <code>/set decision-endpoint</code> and{" "}
            <code>/set decision-model</code> to match your server; a key is only
            needed if your server asks for one.
          </li>
          <li>
            <strong>Session-only storage.</strong> A key entered at the prompt
            stays in memory for that app session. Jev requests use your
            OpenRouter credits; Laya requests never leave your network.
          </li>
          <li>
            <strong>Review stays the fallback.</strong> Failed requests, invalid
            decisions, failed checks, or no configured checks keep full review
            in place.
          </li>
        </ul>
        <p className="jev-disable">
          To turn it off: <code>/set decision-enabled false</code>, then{" "}
          <code>/save</code>.
        </p>
      </div>
    </details>
  );
}
