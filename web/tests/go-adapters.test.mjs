// Keeps the site's agent list in step with the Go adapters it describes, so a
// change in cmd/harness-bridge can't leave the website stale with every test
// green.
import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { agents } from "../src/bridge-setup.js";

const goDir = new URL("../../cmd/harness-bridge/", import.meta.url);

async function goSources() {
  let names;
  try {
    names = await readdir(goDir);
  } catch (error) {
    assert.fail(
      `Can't read the Go adapters in ${fileURLToPath(goDir)} (${error.code}). This test needs cmd/harness-bridge next to web/.`,
    );
  }
  const files = names.filter(
    (name) => name.endsWith(".go") && !name.endsWith("_test.go"),
  );
  assert.ok(
    files.includes("harness.go"),
    "cmd/harness-bridge has no harness.go",
  );
  return Promise.all(
    files.map(async (name) => ({
      name,
      text: await readFile(new URL(name, goDir), "utf8"),
    })),
  );
}

function field(body, key, variable) {
  const match = body.match(new RegExp(`\\b${key}:\\s*"([^"]*)"`));
  assert.ok(match, `${variable} has no ${key}: "…" field`);
  return match[1];
}

// What the Go code says about each agent, in the order of `var adapters`.
async function goAdapters() {
  const sources = await goSources();
  const all = sources.map((source) => source.text).join("\n");
  const list = all.match(/var adapters = \[\]\*adapter\{([^}]*)\}/);
  assert.ok(list, "harness.go has no `var adapters = []*adapter{…}` line");
  const variables = list[1].split(",").map((name) => name.trim());
  // Planner hooks are assigned in planners.go's init(), or set in the literal.
  const planners = new Set(
    [...all.matchAll(/^\s*(\w+)\.Planner\s*=\s*\w+/gm)].map((m) => m[1]),
  );
  return variables.map((variable) => {
    // The literal ends at the first closing brace in column 0.
    const literal = all.match(
      new RegExp(`var ${variable} = &adapter\\{([\\s\\S]*?)\\n\\}`),
    );
    assert.ok(
      literal,
      `no \`var ${variable} = &adapter{…}\` in cmd/harness-bridge`,
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
  assert.ok(go.length > 0, "no adapters found in cmd/harness-bridge");
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
    "web/src/bridge-setup.js `agents` differs from the adapters in cmd/harness-bridge; update the site to match",
  );
});
