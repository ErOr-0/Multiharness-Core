import { Copy } from "lucide-react";

// A titled, copyable command. onCopied receives the message for the section's
// shared status line.
export default function CommandBlock({ title, value, onCopied }) {
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      onCopied?.(`${title} copied.`);
    } catch {
      onCopied?.("Select and copy the command above.");
    }
  }
  return (
    <div className="command-block">
      <div className="install-code-header">
        <h4>{title}</h4>
        <button onClick={copy} aria-label={`Copy ${title}`}>
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
