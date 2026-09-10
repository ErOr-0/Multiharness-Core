import { useEffect, useState } from "react";
import {
  ArrowDown,
  ArrowUpRight,
  Check,
  CircleCheck,
  FolderOpen,
  LoaderCircle,
  Play,
  RotateCcw,
  Square,
  Terminal,
} from "lucide-react";
import { BrandMark } from "./Brand.jsx";

const stages = [
  {
    label: "Plan",
    agent: "Codex",
    detail: "Read the folder. Outline the change.",
    message: "Plan ready · endpoint + focused tests",
    icon: "01",
  },
  {
    label: "Implement",
    agent: "OpenCode",
    detail: "Implement the endpoint and its tests.",
    message: "Changes observed · 2 files",
    icon: "02",
  },
  {
    label: "Validate",
    agent: "Your checks",
    detail: "Run the configured project checks.",
    message: "go test ./... · 1 failing check",
    icon: "03",
  },
  {
    label: "Review",
    agent: "Codex",
    detail: "Inspect the diff and validation evidence.",
    message: "Repair requested · handle HEAD requests",
    icon: "04",
  },
  {
    label: "Repair",
    agent: "OpenCode",
    detail: "Apply the findings with the full task context.",
    message: "Repair applied · checks now pass",
    icon: "05",
  },
  {
    label: "Approve",
    agent: "Codex",
    detail: "Review the updated evidence independently.",
    message: "Approved · ready for your final inspection",
    icon: "06",
  },
];

export default function WorkflowDemo() {
  const [stage, setStage] = useState(-1);
  const [running, setRunning] = useState(false);
  const [complete, setComplete] = useState(false);
  const [cancelled, setCancelled] = useState(false);

  useEffect(() => {
    if (!running) return;
    const timer = window.setTimeout(() => {
      if (stage === stages.length - 1) {
        setRunning(false);
        setComplete(true);
      } else setStage((value) => value + 1);
    }, 1050);
    return () => window.clearTimeout(timer);
  }, [running, stage]);

  function start() {
    setStage(0);
    setComplete(false);
    setCancelled(false);
    setRunning(true);
  }

  function stop() {
    setRunning(false);
    setCancelled(true);
    setComplete(false);
  }

  const displayed =
    stage < 0
      ? stages.slice(0, 3)
      : stages.slice(Math.max(0, stage - 2), stage + 1);
  const status = complete
    ? "Approved"
    : cancelled
      ? "Cancelled"
      : running
        ? stages[stage].label
        : "Ready when you are";

  return (
    <div className="hero-demo-wrap">
      <div className="demo-annotation">
        <span className="annotation-line" />A small team. A complete workflow.
        <ArrowDown size={16} />
      </div>
      <div className="workflow-demo" aria-label="Interactive workflow preview">
        <div className="terminal-chrome">
          <div className="window-dots">
            <i />
            <i />
            <i />
          </div>
          <span>
            <Terminal size={13} /> magent
          </span>
          <span className="terminal-branch">
            <FolderOpen size={12} /> your folder
          </span>
        </div>
        <div className="terminal-content">
          <div className="terminal-greeting">
            <BrandMark />
            <span>Your agents are ready.</span>
            <span className="terminal-mini-badge">LOCAL</span>
          </div>
          <div className="terminal-prompt">
            <span>›</span>
            <p>
              Add a health-check endpoint
              <br className="desktop-break" /> and tests for it.
            </p>
            <span className="terminal-enter">↵</span>
          </div>
          <div className="terminal-flow-line">
            <span>ONE TASK</span>
            <div />
            <span>SHARED CONTEXT</span>
          </div>
          <div className="demo-log" aria-live="polite" aria-atomic="true">
            {displayed.map((item) => {
              const index = stages.indexOf(item);
              const active = running && index === stage;
              const done = stage > index || complete;
              return (
                <div
                  className={`demo-log-row ${active ? "active" : ""} ${done ? "done" : ""}`}
                  key={item.label}
                >
                  <span className="log-state">
                    {active ? (
                      <LoaderCircle className="spin" size={14} />
                    ) : done ? (
                      <Check size={14} />
                    ) : (
                      item.icon
                    )}
                  </span>
                  <div>
                    <div className="log-title">
                      {item.label}
                      <span>{item.agent}</span>
                    </div>
                    <p>
                      {done || (active && stage > 0)
                        ? item.message
                        : item.detail}
                    </p>
                  </div>
                  {(done || (stage < 0 && index === 0)) && (
                    <span className="log-dot" />
                  )}
                </div>
              );
            })}
          </div>
          <div className={`demo-status ${complete ? "complete" : ""}`}>
            <span>
              {complete ? (
                <CircleCheck size={15} />
              ) : (
                <span className={`status-dot ${running ? "pulsing" : ""}`} />
              )}
              {status}
            </span>
            {running ? (
              <button onClick={stop} aria-label="Cancel workflow preview">
                <Square size={11} /> Stop
              </button>
            ) : (
              <button
                onClick={start}
                aria-label={
                  complete || cancelled
                    ? "Replay workflow preview"
                    : "Run workflow preview"
                }
              >
                {complete || cancelled ? (
                  <RotateCcw size={13} />
                ) : (
                  <Play size={12} fill="currentColor" />
                )}
                {complete || cancelled ? "Replay" : "Run preview"}
              </button>
            )}
          </div>
        </div>
        <div className="demo-caption">
          <span className="preview-dot" />
          INTERACTIVE PREVIEW<span>Simulated · no model calls</span>
        </div>
      </div>
      <div className="context-float">
        <div className="context-float-icon">
          <FolderOpen size={18} />
        </div>
        <div>
          <strong>Your files stay in your folder.</strong>
          <span>Changes ready for you to review.</span>
        </div>
        <ArrowUpRight size={17} />
      </div>
      <span className="demo-orbit orbit-one" aria-hidden="true" />
      <span className="demo-orbit orbit-two" aria-hidden="true" />
    </div>
  );
}
