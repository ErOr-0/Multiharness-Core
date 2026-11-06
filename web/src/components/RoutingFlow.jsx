import {
  ArrowRight,
  Aperture,
  CheckCheck,
  Code2,
  MessageSquareText,
  Route,
  ServerCog,
  ShieldCheck,
} from "lucide-react";

// Example pairing used only to illustrate who handles each route.
const AGENTS = {
  claude: [<Code2 key="i" size={14} />, "Claude Code"],
  codex: [<Aperture key="i" size={14} />, "Codex CLI"],
};

const ROUTES = [
  {
    number: "01",
    name: "answer",
    when: "A question or an explanation. Nothing to change.",
    chain: [
      ["Planner", "claude"],
      ["Read-only answer", null, "end"],
    ],
  },
  {
    number: "02",
    name: "needs_planning",
    when: "A real change: new feature, refactor, multi-file fix.",
    chain: [
      ["Plan", "claude"],
      ["Implement", "codex"],
      ["Review", "claude"],
    ],
  },
  {
    number: "03",
    name: "direct_implement",
    when: "A small, clear edit. A short plan is enough.",
    chain: [
      ["Short plan", "claude"],
      ["Implement", "codex"],
      ["Review", "claude"],
    ],
  },
];

function Arrow({ className = "" }) {
  return (
    <span className={`routing-arrow ${className}`} aria-hidden="true">
      <ArrowRight size={18} />
    </span>
  );
}

function Chip({ label, agent, kind }) {
  const harness = agent ? AGENTS[agent] : null;
  return (
    <span className={`routing-chip ${kind ? `routing-chip-${kind}` : ""}`}>
      <strong>{label}</strong>
      {harness && (
        <span className="routing-chip-agent">
          {harness[0]}
          {harness[1]}
        </span>
      )}
    </span>
  );
}

export default function RoutingFlow() {
  return (
    <section
      className="section routing-section"
      id="routing"
      aria-labelledby="routing-title"
    >
      <div className="container">
        <div className="section-heading">
          <span className="eyebrow">OPTIONAL · BEFORE THE PLAN</span>
          <h2 id="routing-title">
            Ask first.
            <span className="muted-heading"> Plan only if needed.</span>
          </h2>
          <p>
            <code>harness-bridge route</code> asks a System One decision model
            whether a request needs a plan. Set up with{" "}
            <code>--router laya</code> or <code>--router jev</code> and the
            planner routes each request first.
          </p>
        </div>

        <div className="routing-diagram">
          <div className="routing-stage routing-request">
            <span className="routing-kicker">01 · YOUR REQUEST</span>
            <div className="routing-bubble">
              <MessageSquareText size={18} aria-hidden="true" />
              <p>“fix the typo in README.md”</p>
            </div>
            <p className="routing-stage-note">
              Piped in:{" "}
              <code>echo "…" | harness-bridge route --provider laya</code>
            </p>
          </div>

          <Arrow className="routing-arrow-stage" />

          <div className="routing-stage routing-decision">
            <span className="routing-kicker">02 · DECISION MODEL</span>
            <div className="routing-models">
              <article className="routing-model">
                <span className="routing-model-icon">
                  <Route size={30} aria-hidden="true" />
                </span>
                <h3>Jev</h3>
                <p>
                  TypeSafe’s hosted model, reached through OpenRouter. Needs{" "}
                  <code>OPENROUTER_API_KEY</code>.
                </p>
              </article>
              <span className="routing-or">or</span>
              <article className="routing-model routing-model-laya">
                <span className="routing-model-icon">
                  <ServerCog size={30} aria-hidden="true" />
                </span>
                <h3>
                  Laya <span className="integration-note">self-hosted</span>
                </h3>
                <p>
                  On a server you run. Point <code>LAYA_ENDPOINT</code> at it
                  (default <code>127.0.0.1:8765</code>).
                </p>
              </article>
            </div>
            <p className="routing-stage-note">
              One route out, with confidence and advice.
            </p>
          </div>

          <Arrow className="routing-arrow-stage" />

          <div className="routing-stage routing-routes">
            <span className="routing-kicker">03 · ROUTE → PLANNER</span>
            <ol className="routing-list">
              {ROUTES.map((route) => (
                <li className="routing-route" key={route.name}>
                  <div className="routing-route-head">
                    <span className="routing-route-number">{route.number}</span>
                    <h3>{route.name}</h3>
                    <p>{route.when}</p>
                  </div>
                  <div className="routing-chain">
                    {route.chain.map(([label, agent, kind], index) => (
                      <span className="routing-chain-item" key={label}>
                        {index > 0 && <Arrow />}
                        <Chip label={label} agent={agent} kind={kind} />
                      </span>
                    ))}
                  </div>
                </li>
              ))}
            </ol>
            <p className="routing-fallback">
              <ShieldCheck size={16} aria-hidden="true" />
              <span>
                <strong>Unreachable, unsure or a bad answer?</strong> Below{" "}
                <code>--threshold</code> (default 0.75), <code>route</code>{" "}
                returns the safe choice, <code>needs_planning</code>, and says
                why in <code>fallback</code> and <code>error</code>.
              </span>
            </p>
          </div>
        </div>

        <p className="routing-footnote">
          <CheckCheck size={15} aria-hidden="true" />
          <span>
            <code>--review</code> asks instead whether a described change needs
            a full review: <code>needs_full_review</code> or{" "}
            <code>auto_approve</code>. Agents shown are an example pairing.
          </span>
        </p>
      </div>
    </section>
  );
}
