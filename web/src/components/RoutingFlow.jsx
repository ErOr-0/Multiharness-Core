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
import { MetaIcon } from "./Brand.jsx";

// Example team used only to illustrate which agent each route reaches. The
// real roles come from the user's own configuration.
const AGENTS = {
  claude: [<Code2 key="i" size={14} />, "Claude Code"],
  codex: [<Aperture key="i" size={14} />, "Codex"],
  muse: [<MetaIcon key="i" />, "Muse Code"],
};

const ROUTES = [
  {
    number: "01",
    name: "Answer",
    when: "A question, an explanation or a review with no request to edit.",
    chain: [
      ["Planner", "claude"],
      ["Read-only answer", null, "end"],
    ],
  },
  {
    number: "02",
    name: "Plan",
    when: "A real change: new feature, refactor, multi-file fix.",
    chain: [
      ["Planner", "codex"],
      ["Implementer", "muse"],
      ["Validate", null],
      ["Reviewer", "claude"],
    ],
  },
  {
    number: "03",
    name: "Implement directly",
    when: "A small, explicit, low-risk edit. Still validated and reviewed.",
    chain: [
      ["Implementer", "muse"],
      ["Validate", null],
      ["Reviewer", "claude"],
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
          <span className="eyebrow">BEFORE THE TEAM STARTS</span>
          <h2 id="routing-title">
            Ask anything.
            <span className="muted-heading"> The right route, every time.</span>
          </h2>
          <p>
            In team mode an optional decision model reads your request first and
            picks one of three routes. Your coding agents only start once the
            route is known.
          </p>
        </div>

        <div className="routing-diagram">
          <div className="routing-stage routing-request">
            <span className="routing-kicker">01 · YOUR REQUEST</span>
            <div className="routing-bubble">
              <MessageSquareText size={18} aria-hidden="true" />
              <p>“Is the agent loop implemented correctly?”</p>
            </div>
            <p className="routing-stage-note">
              Any question or task, in your own words.
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
                <p>TypeSafe’s hosted model, through your own OpenRouter key.</p>
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
                  The open Laya model on a server you run, like a Docker
                  container. Nothing leaves your network.
                </p>
              </article>
            </div>
            <p className="routing-stage-note">
              One request in, one typed route out, with a confidence score.
            </p>
          </div>

          <Arrow className="routing-arrow-stage" />

          <div className="routing-stage routing-routes">
            <span className="routing-kicker">03 · ROUTE → AGENTS</span>
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
                <strong>Unsure, offline or low confidence?</strong> The request
                falls back to a read-only assessment by your planner. Routing
                never skips validation or review.
              </span>
            </p>
          </div>
        </div>

        <p className="routing-footnote">
          <CheckCheck size={15} aria-hidden="true" />
          Agents shown are an example team. Each role uses the CLI and model you
          configure, and routing is off by default.
        </p>
      </div>
    </section>
  );
}
