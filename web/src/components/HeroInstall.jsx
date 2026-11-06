import { useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import { detectPlatform, platforms } from "../bridge-setup.js";

// The install line for the detected system, right in the hero.
export default function HeroInstall() {
  const platform =
    platforms.find((item) => item.id === detectPlatform()) ?? platforms[0];
  const [feedback, setFeedback] = useState("");
  const [copied, setCopied] = useState(false);
  const timer = useRef(null);

  useEffect(() => () => clearTimeout(timer.current), []);

  async function copy() {
    try {
      await navigator.clipboard.writeText(platform.install);
      setFeedback("Install command copied.");
      setCopied(true);
      clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 2000);
    } catch {
      setFeedback("Select and copy the command.");
    }
  }

  return (
    <div className="hero-install">
      <div className="hero-install-chip">
        <span className="hero-install-glyph" aria-hidden="true">
          {platform.id === "windows" ? ">" : "$"}
        </span>
        <div
          className="hero-install-code"
          tabIndex={0}
          role="region"
          aria-label="Install command for your system"
        >
          <code>{platform.install}</code>
        </div>
        <button
          className="hero-install-copy"
          onClick={copy}
          aria-label="Copy the install command"
        >
          {copied ? <Check size={16} /> : <Copy size={16} />}
        </button>
      </div>
      <p className="hero-install-note">
        For {platform.label} · {platform.shell}.{" "}
        <a href="#start">Other systems</a>
      </p>
      <span className="hero-install-feedback" aria-live="polite">
        {feedback}
      </span>
    </div>
  );
}
