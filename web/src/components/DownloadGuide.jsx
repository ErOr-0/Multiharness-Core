import {
  ArrowDown,
  ArrowUpRight,
  Check,
  Download,
  FolderOpen,
  KeyRound,
  PackageCheck,
  Terminal,
} from "lucide-react";
import { DOCS, DOCKER_HUB, RELEASE } from "../content.js";
import { DockerIcon } from "./Brand.jsx";
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
      <div className="download-options">
        <article className="download-primary">
          <div className="download-card-top">
            <DockerIcon />
            <span>RECOMMENDED</span>
          </div>
          <h3>Run with Docker.</h3>
          <p>
            The app, all three agent CLIs, and common build tools. Ready in one
            reusable container.
          </p>
          <div className="download-platforms">
            <span>macOS</span>
            <span>Windows</span>
            <span>Linux</span>
          </div>
          <a className="button button-lime" href="#start">
            Start installation <ArrowDown size={17} />
          </a>
          <a className="download-image-link" href={DOCKER_HUB}>
            AMD64 + ARM64 images <ArrowUpRight size={14} />
          </a>
        </article>
        <article className="download-secondary">
          <div className="download-card-top">
            <Download size={25} />
            <span>OPTIONAL DOWNLOAD</span>
          </div>
          <h3>Keep the setup files.</h3>
          <p>
            A version-pinned Docker configuration bundle with the guide and
            Linux policies. The guided setup below fetches these files for you.
          </p>
          <a className="button button-dark" href={RELEASE.bundle}>
            Download configuration ZIP <Download size={16} />
          </a>
          <div className="download-file-links">
            <a href={RELEASE.checksums}>
              Checksums <ArrowUpRight size={13} />
            </a>
            <a href={RELEASE.url}>
              Release notes <ArrowUpRight size={13} />
            </a>
          </div>
          <p className="download-small">
            Configuration files, not a standalone app installer.
          </p>
        </article>
      </div>
      <section
        className="requirements-guide"
        id="requirements"
        aria-labelledby="requirements-title"
      >
        <div className="requirements-heading">
          <h3 id="requirements-title">What you’ll need</h3>
          <span>For the recommended Docker setup</span>
        </div>
        <div className="requirements-grid">
          <article>
            <Terminal size={22} />
            <h4>Docker + Git</h4>
            <p>
              <a href="https://docs.docker.com/get-started/get-docker/">
                Docker Desktop
              </a>{" "}
              on macOS or Windows, or{" "}
              <a href="https://docs.docker.com/engine/install/">
                Docker Engine with Compose
              </a>{" "}
              on Linux. Install <a href="https://git-scm.com/downloads">Git</a>{" "}
              for the generated setup command. Windows uses Linux containers.
            </p>
          </article>
          <article>
            <KeyRound size={22} />
            <h4>Your provider account</h4>
            <p>
              Sign in to the providers you choose. Model access and usage
              charges follow your provider’s plan. Internet access is needed for
              downloads, sign-in, and model requests.
            </p>
          </article>
          <article>
            <FolderOpen size={22} />
            <h4>A project folder</h4>
            <p>
              Choose an existing folder on your computer. A Git repository is
              optional. The app edits the folder you share with Docker.
            </p>
          </article>
        </div>
        <div className="included-agents">
          <PackageCheck size={23} />
          <div>
            <strong>No separate agent installation.</strong>
            <p>
              Codex, OpenCode, and Claude Code are already included in the
              Docker image. Choose which to use inside the app.
            </p>
          </div>
          <span>
            <Check size={15} /> Included
          </span>
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
      </section>
    </div>
  );
}

export function JevGuide({ command }) {
  return (
    <section className="jev-guide" aria-labelledby="jev-guide-title">
      <div className="jev-copy">
        <span className="eyebrow">JEV ROUTING · OPTIONAL</span>
        <h3 id="jev-guide-title">A lighter route through Team mode.</h3>
        <p>
          Enable Jev to decide when planning or a full review can be skipped.
          It’s off by default; your normal Direct and Team workflows work
          without it.
        </p>
        <a href={RELEASE.evidence}>
          View the live API test results <ArrowUpRight size={15} />
        </a>
      </div>
      <div className="jev-setup">
        <h4>Enable it inside the terminal app</h4>
        {command(
          "Enable Jev routing",
          "/set mode team\n/set decision-enabled true\n/save",
        )}
        <ul>
          <li>
            <strong>Your OpenRouter key.</strong> On your next task, the app
            asks for it with hidden input if it isn’t already configured. Enter
            it in the app, never on this website.
          </li>
          <li>
            <strong>Session-only storage.</strong> A key entered at the prompt
            stays in memory for that app session. Jev requests use your
            OpenRouter credits.
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
    </section>
  );
}
