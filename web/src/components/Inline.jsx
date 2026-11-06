// Renders `backtick` spans in copy as <code>, so content.js stays plain text.
export default function Inline({ text }) {
  return text
    .split(/`([^`]+)`/)
    .map((part, index) => (index % 2 ? <code key={index}>{part}</code> : part));
}
