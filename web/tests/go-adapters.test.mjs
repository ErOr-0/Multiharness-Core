// Keeps the site's agent list in step with the Go adapters it describes, so a
// change in internal/agent can't leave the website stale with every test
// green.
import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { agents } from "../src/bridge-setup.js";

// The adapters are in internal/agent; their planner hooks are attached in
// internal/setup.
const goDirs = ["agent", "setup"].map(
  (name) => new URL(`../../internal/${name}/`, import.meta.url),
);

async function goSources() {
  const sources = [];
  for (const dir of goDirs) {
    let names;
    try {
      names = await readdir(dir);
    } catch (error) {
      assert.fail(
        `Can't read the Go sources in ${fileURLToPath(dir)} (${error.code}). This test needs internal/agent and internal/setup next to web/.`,
      );
    }
    for (const name of names) {
      if (name.endsWith(".go") && !name.endsWith("_test.go")) {
        sources.push(await readFile(new URL(name, dir), "utf8"));
      }
    }
  }
  return sources;
}

function field(body, key, variable) {
  const match = body.match(new RegExp(`\\b${key}:\\s*"([^"]*)"`));
  assert.ok(match, `${variable} has no ${key}: "…" field`);
  return match[1];
}

// What the Go code says about each agent, in the order of `var All`.
async function goAdapters() {
  const all = (await goSources()).join("\n");
  const list = all.match(/var All = \[\]\*Adapter\{([^}]*)\}/);
  assert.ok(list, "internal/agent has no `var All = []*Adapter{…}` line");
  const variables = list[1].split(",").map((name) => name.trim());
  // Planner hooks are assigned in internal/setup's init(), or set in the
  // literal.
  const planners = new Set(
    [...all.matchAll(/^\s*agent\.(\w+)\.Planner\s*=\s*\w+/gm)].map((m) => m[1]),
  );
  return variables.map((variable) => {
    // The literal ends at the first closing brace in column 0.
    const literal = all.match(
      new RegExp(`var ${variable} = &Adapter\\{([\\s\\S]*?)\\n\\}`),
    );
    assert.ok(
      literal,
      `no \`var ${variable} = &Adapter{…}\` in internal/agent`,
    );
    const body = literal[1];
    return {
      name: field(body, "Name", variable),
      title: field(body, "Title", variable),
      login: field(body, "Login", variable),
      resume: /\bResume:\s*true\b/.test(body),
      mcpPlanner:
        planners.has(variable) || /\bPlanner:\s*\w+Planner\b/.test(body),
    };
  });
}

test("the site's agents match the Go adapters", async () => {
  const go = await goAdapters();
  assert.ok(go.length > 0, "no adapters found in internal/agent");
  const site = agents.map(({ name, title, login, resume, mcpPlanner }) => ({
    name,
    title,
    login,
    resume,
    mcpPlanner,
  }));
  assert.deepEqual(
    site,
    go,
    "web/src/bridge-setup.js `agents` differs from the adapters in internal/agent; update the site to match",
  );
});
