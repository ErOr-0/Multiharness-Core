import {
  ArrowRight,
  ArrowUpRight,
  Check,
  Download,
  FlaskConical,
  Settings2,
  Workflow,
} from "lucide-react";
import { DOCS, roadmap } from "../content.js";
import "./Roadmap.css";

const icons = {
  reliability: FlaskConical,
  installation: Download,
  builder: Workflow,
  onboarding: Settings2,
};

export default function Roadmap() {
  return (
    <section
      className="section container roadmap-section"
      id="roadmap"
      aria-labelledby="roadmap-title"
    >
      <div className="section-heading">
        <div>
          <span className="eyebrow section-eyebrow">
            <span /> THE ROAD AHEAD
          </span>
          <h2 id="roadmap-title">
            What we're building next.
            <br />
            <span className="muted-heading">Still local. More flexible.</span>
          </h2>
        </div>
        <p>
          A look at current work and planned improvements. More choice in your
          team, with less setup between you and your next task.
        </p>
      </div>

      <div className="roadmap-lanes">
        {roadmap.map((lane) => (
          <div className={`roadmap-lane roadmap-lane-${lane.id}`} key={lane.id}>
            <div className="roadmap-lane-heading">
              <h3>
                <span className="roadmap-status-mark" aria-hidden="true" />
                {lane.label}
              </h3>
              <span className="roadmap-count">
                {String(lane.items.length).padStart(2, "0")} INITIATIVES
              </span>
            </div>
            <p className="roadmap-lane-description">{lane.description}</p>
            <div className="roadmap-cards">
              {lane.items.map((item) => {
                const Icon = icons[item.id];
                return (
                  <article className="roadmap-card" key={item.id}>
                    <div className="roadmap-card-top">
                      <span className="roadmap-icon">
                        <Icon size={20} />
                      </span>
                      <span className="roadmap-badge">{item.tag}</span>
                    </div>
                    <h4>{item.title}</h4>
                    <p>{item.description}</p>
                    <div className="roadmap-card-note">
                      <ArrowRight size={14} />
                      <span>{item.note}</span>
                    </div>
                  </article>
                );
              })}
            </div>
          </div>
        ))}
      </div>

      <div className="roadmap-footer">
        <p>
          <Check size={16} />
          <span>
            <strong>Already here:</strong> save local settings with{" "}
            <code>/config</code> and <code>/save</code>.
          </span>
        </p>
        <a
          className="text-link"
          href={`${DOCS}#development-and-verification`}
          target="_blank"
          rel="noreferrer"
        >
          See release progress <ArrowUpRight size={16} />
        </a>
      </div>
      <p className="roadmap-expectations">
        Coming next marks planned work. Timing will be shared as features are
        ready.
      </p>
    </section>
  );
}
