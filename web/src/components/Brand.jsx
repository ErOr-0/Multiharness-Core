export function DockerIcon() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M2 10h3v3H2zm4 0h3v3H6zm4 0h3v3h-3zm4 0h3v3h-3zM6 6h3v3H6zm4 0h3v3h-3zm0-4h3v3h-3z" />
      <path d="M23.5 10.5c-1.2-.7-2.4-.6-3.2-.3-.2-1.4-1-2.4-2-3.1-.7 1.5-.9 3 .1 4.3-.6.9-1.5 1.6-3.4 1.6H1c-.2 5 2.8 8 7.5 8 5.3 0 9-2.6 10.7-7.1 2.1.1 3.5-1.1 4.3-3.4Z" />
    </svg>
  );
}

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
