import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { createServer } from "vite";
import {
  DOCKER_HUB,
  LAUNCHER_DOWNLOAD,
  dockerCommands,
} from "../src/content.js";

test("each platform copies separate documented commands from the extracted ZIP root", async () => {
  const guide = await readFile(
    new URL("../../docs/docker.md", import.meta.url),
    "utf8",
  );
  for (const [platform, commands] of Object.entries(dockerCommands)) {
    assert.deepEqual(Object.keys(commands), ["setup", "run"]);
    for (const line of Object.values(commands)) {
      assert.doesNotMatch(
        line,
        /\r|\n/,
        "Each copy button must copy only one command",
      );
      assert.ok(
        guide.includes(line),
        `${platform}: command must match the shipped Docker guide: ${line}`,
      );
    }
    assert.match(commands.setup, /setup$/);
    assert.doesNotMatch(
      Object.values(commands).join("\n"),
      /docker pull|docker create|docker cp|git clone|make install|npm install|export PATH/,
    );
    if (platform === "Windows") {
      assert.match(commands.run, /^\.\\scripts\\magent-docker\.ps1 -Project '/);
      assert.equal(commands.setup, `${commands.run} -Command setup`);
    } else {
      assert.match(
        commands.run,
        /^sh \.\/scripts\/magent-docker\.sh --project '/,
      );
      assert.equal(commands.setup, `${commands.run} setup`);
    }
  }
});

test("the rendered page leads with the launcher download and explains terminal setup", async () => {
  const server = await createServer({
    server: { middlewareMode: true },
    appType: "custom",
  });
  try {
    const { default: App, GettingStarted } =
      await server.ssrLoadModule("/src/App.jsx");
    const html = renderToStaticMarkup(App());
    assert.ok(html.includes(`href="${DOCKER_HUB}"`));
    const startSection = html.match(
      /<section[^>]*id="start"[^>]*>(.*?)<\/section>/,
    )?.[1];
    assert.ok(
      startSection?.includes(
        `class="button button-lime" href="${LAUNCHER_DOWNLOAD}"`,
      ),
    );
    assert.ok(startSection?.includes("Download launcher ZIP"));
    assert.ok(startSection?.includes("Docker Desktop’s Run button"));
    assert.ok(startSection?.includes("inside the extracted launcher folder"));
    assert.match(html, /Copy setup command/);
    assert.match(html, /Copy launch command/);
    assert.doesNotMatch(
      startSection,
      /docker create|docker cp|docker pull|Get the Docker image/,
    );
    assert.match(html, /Git is optional/);
    assert.match(html, /multiple projects with separate Git repositories/);
    assert.doesNotMatch(html, /Git repository root|your Git repository/);
    assert.match(html, /private Docker volume/);
    assert.match(html, /does not automatically inherit logins/);
    assert.match(html, /WSL 2 behind the scenes/);
    assert.match(
      html,
      /native Linux amd64\/arm64 container checks have passed/,
    );
    assert.match(html, /OpenCode is optional/);
    const linux = renderToStaticMarkup(
      createElement(GettingStarted, { initialPlatform: "Linux" }),
    );
    assert.match(linux, /Get Linux launcher &amp; setup/);
    assert.match(linux, /docker\.md#linux-apparmor-setup/);
    assert.doesNotMatch(
      linux,
      new RegExp(LAUNCHER_DOWNLOAD.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")),
    );
    assert.doesNotMatch(html, /BUILD FROM SOURCE|Windows via WSL|make install/);
    const faqHeading = html.match(/<h2 id="faq-title">(.*?)<\/h2>/)?.[1];
    assert.equal(
      faqHeading?.replace(/<br\s*\/?>/g, ""),
      "Before you bring the team in.",
      "Hiding the desktop line break must preserve the word boundary on mobile",
    );
  } finally {
    await server.close();
  }
});
