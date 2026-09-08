export function BrandMark({ className = "" }) {
  return (
    <svg
      className={`brand-mark ${className}`}
      viewBox="0 0 28 28"
      fill="currentColor"
      aria-hidden="true"
    >
      <rect x="1" y="1" width="10" height="10" rx="2" />
      <rect x="17" y="1" width="10" height="26" rx="2" />
      <rect x="1" y="17" width="10" height="10" rx="2" />
    </svg>
  );
}

export default function Brand() {
  return (
    <a className="brand" href="#top" aria-label="Multiharness home">
      <BrandMark />
      <span>
        multiharness<span className="brand-period">.</span>
      </span>
    </a>
  );
}
