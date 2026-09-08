import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer } from "vite";
import { DOCKER_HUB, DOCKER_IMAGE, dockerCommands } from "../src/content.js";

test("each platform copies the documented image extraction and project launch", async () => {
  const guide = await readFile(
    new URL("../../docs/docker.md", import.meta.url),
    "utf8",
  );
  for (const [platform, commands] of Object.entries(dockerCommands)) {
    const runnable = commands
      .split("\n")
      .filter((line) => line && !line.startsWith("#"));
    assert.equal(
      runnable.length,
      6,
      `${platform}: four extraction and two launch commands`,
    );
    for (const line of runnable) {
      assert.ok(
        guide.includes(line),
        `${platform}: command must match the shipped Docker guide: ${line}`,
      );
    }
    assert.equal(runnable[0], `docker pull ${DOCKER_IMAGE}`);
    assert.match(runnable[2], /:\/opt\/magent\/launcher \.\/magent-docker$/);
    assert.match(runnable[4], /setup$/);
    assert.doesNotMatch(
      commands,
      /git clone|make install|npm install|export PATH/,
    );
    if (platform === "Windows") {
      assert.match(
        runnable[5],
        /^\.\\magent-docker\\scripts\\magent-docker\.ps1 -Project '/,
      );
    } else {
      assert.match(
        runnable[5],
        /^sh \.\/magent-docker\/scripts\/magent-docker\.sh --project '/,
      );
    }
  }
});

test("the rendered page leads with Docker and explains remaining setup", async () => {
  const server = await createServer({
    server: { middlewareMode: true },
    appType: "custom",
  });
  try {
    const { default: App } = await server.ssrLoadModule("/src/App.jsx");
    const html = renderToStaticMarkup(App());
    assert.ok(html.includes(`href="${DOCKER_HUB}"`));
    assert.match(html, /Get the Docker image/);
    assert.match(html, /Docker setup commands/);
    assert.match(html, /private Docker volume/);
    assert.match(html, /does not automatically inherit logins/);
    assert.match(html, /WSL 2 behind the scenes/);
    assert.match(html, /Native ARM sandboxing/);
    assert.doesNotMatch(html, /BUILD FROM SOURCE|Windows via WSL|make install/);
  } finally {
    await server.close();
  }
});
