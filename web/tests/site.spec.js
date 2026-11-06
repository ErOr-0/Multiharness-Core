import AxeBuilder from "@axe-core/playwright";
import { test, expect } from "@playwright/test";
import {
  agents,
  platforms,
  sourceInstallCommand,
} from "../src/bridge-setup.js";

async function readClipboard(page) {
  return (await page.evaluate(() => navigator.clipboard.readText())).replace(
    /\r\n/g,
    "\n",
  );
}

const installLine = Object.fromEntries(
  platforms.map((item) => [item.label, item.install]),
);

test("install line matches the chosen system and needs no Go", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#start");
  const setup = page.locator("#start");
  for (const [label, shell] of [
    ["macOS & Linux", "Terminal"],
    ["Windows", "PowerShell"],
  ]) {
    await page.getByRole("button", { name: label, exact: true }).click();
    await expect(
      page.getByRole("button", { name: label, exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
    await page.getByRole("button", { name: "Copy Install line" }).click();
    expect(await readClipboard(page)).toBe(installLine[label]);
    await expect(page.locator(".setup-feedback")).toHaveText(
      "Install line copied.",
    );
    await expect(setup).toContainText(`Paste it into ${shell}`);
  }
  expect(installLine["macOS & Linux"]).toBe(
    "curl -fsSL https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/main/install.sh | sh",
  );
  expect(installLine.Windows).toBe(
    "irm https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/main/install.ps1 | iex",
  );
  await expect(setup.locator(".install-steps")).not.toContainText("go install");
  await expect(setup.locator('a[href*="/releases"]')).toHaveCount(0);
});

test("setup steps are the three questions, one confirmation, then the planner", async ({
  page,
}) => {
  await page.goto("/#start");
  const questions = page
    .getByRole("list", { name: "Setup questions" })
    .getByRole("listitem");
  await expect(questions).toHaveCount(3);
  const planners = questions.nth(0);
  await expect(planners).toContainText(
    "Which agent should plan and review your work?",
  );
  for (const name of ["Claude Code", "Codex CLI", "OpenCode", "Command Code"])
    await expect(planners).toContainText(name);
  await expect(planners).not.toContainText("Muse Code");
  const implementers = questions.nth(1);
  await expect(implementers).toContainText(
    "Which agent should write the code?",
  );
  for (const name of [
    "Codex CLI",
    "Claude Code",
    "Muse Code",
    "OpenCode",
    "Command Code",
  ])
    await expect(implementers).toContainText(name);
  await expect(questions.nth(2)).toContainText(
    "Route requests through Laya or Jev first?",
  );
  await expect(questions.nth(2)).toContainText("Optional");
  const steps = page.locator(".install-steps");
  await expect(steps).toContainText("Continue? [Y/n]");
  await expect(steps).toContainText("Missing agents install in the background");
  await expect(steps).toContainText("sign-in opens in your browser");
  await expect(steps).toContainText("as an MCP server for all your projects");
  await expect(steps).toContainText("open your planner in any project");
  await expect(steps).toContainText("Nothing is written into your projects");
  await expect(steps).not.toContainText("AGENTS.md");
  await expect(steps).not.toContainText("--dir");
});

test("setup without questions shows the matching flags", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#start");
  await page
    .locator("summary", { hasText: "Set up without questions" })
    .click();
  const planner = page.getByLabel("Planner", { exact: true });
  const implementer = page.getByLabel("Implementer", { exact: true });
  await expect(planner).toHaveValue("claude");
  await expect(implementer).toHaveValue("codex");
  await expect(planner.locator("option")).toHaveText([
    "Codex CLI",
    "Claude Code",
    "OpenCode",
    "Command Code",
  ]);
  await page.getByRole("button", { name: "Copy Setup command" }).click();
  expect(await readClipboard(page)).toBe(
    "harness-bridge setup --planner claude --implementer codex",
  );
  await expect(page.locator(".setup-mode")).toContainText(
    "Connects Claude Code for all projects",
  );
  await page.getByRole("button", { name: "Copy Sign in once" }).click();
  expect(await readClipboard(page)).toBe("claude auth login\ncodex login");

  await page.getByLabel("Route requests first").selectOption("laya");
  await planner.selectOption("opencode");
  await implementer.selectOption("muse");
  await page.getByRole("button", { name: "Copy Setup command" }).click();
  expect(await readClipboard(page)).toBe(
    "harness-bridge setup --planner opencode --implementer muse --router laya",
  );
  await expect(page.locator(".setup-mode")).toContainText(
    "Connects OpenCode for all projects",
  );
  await page.getByRole("button", { name: "Copy Sign in once" }).click();
  expect(await readClipboard(page)).toBe("opencode auth login\nmuse login");
});

test("Claude Code is not offered as its own implementer", async ({ page }) => {
  await page.goto("/#start");
  await page
    .locator("summary", { hasText: "Set up without questions" })
    .click();
  const planner = page.getByLabel("Planner", { exact: true });
  const implementer = page.getByLabel("Implementer", { exact: true });
  await expect(implementer.locator("option")).toHaveText([
    "Codex CLI",
    "Muse Code",
    "OpenCode",
    "Command Code",
  ]);
  await planner.selectOption("codex");
  await implementer.selectOption("claude");
  await expect(page.getByRole("region", { name: "Setup command" })).toHaveText(
    "harness-bridge setup --planner codex --implementer claude",
  );
  await planner.selectOption("claude");
  await expect(implementer).toHaveValue("codex");
  await expect(page.locator("#start").getByRole("alert")).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Setup command" })).toHaveText(
    "harness-bridge setup --planner claude --implementer codex",
  );
});

test("building from source is an option, not the main path", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#start");
  await expect(
    page.getByRole("button", { name: "Copy Build from source" }),
  ).toBeHidden();
  await page.locator("summary", { hasText: "Build from source" }).click();
  await page.getByRole("button", { name: "Copy Build from source" }).click();
  const source = await readClipboard(page);
  expect(source).toBe(sourceInstallCommand);
  expect(source).toContain("go install ./cmd/harness-bridge");
});

test("workflow preview completes, replays, and cancels without backend calls", async ({
  page,
}) => {
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.goto("/");
  await page.evaluate(() => document.fonts.ready);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(page.getByRole("heading", { level: 1 })).toContainText(
    "One agent plans.",
  );
  await page.getByRole("button", { name: "Run workflow preview" }).click();
  await expect(
    page.getByRole("button", { name: "Cancel workflow preview" }),
  ).toBeVisible();
  await expect(page.locator(".demo-status.complete")).toContainText("Done", {
    timeout: 12000,
  });
  await page.getByRole("button", { name: "Replay workflow preview" }).click();
  await page.getByRole("button", { name: "Cancel workflow preview" }).click();
  await expect(page.locator(".demo-status")).toContainText("Cancelled");
  expect(pageErrors).toEqual([]);
});

test("desktop setup panel fits in one view on every system", async ({
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "desktop",
    "Small screens keep readable stacked steps.",
  );
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/#start");
  await page.evaluate(() => document.fonts.ready);
  const panel = page.locator(".quick-install");
  for (const viewport of [
    { width: 1440, height: 900 },
    { width: 1280, height: 800 },
    { width: 1024, height: 768 },
  ]) {
    await page.setViewportSize(viewport);
    for (const label of ["macOS & Linux", "Windows"]) {
      await page.getByRole("button", { name: label, exact: true }).click();
      await panel.evaluate((el) => el.scrollIntoView());
      const box = await panel.boundingBox();
      const where = `${label} at ${viewport.width}`;
      expect(box.y, `${where}: top`).toBeGreaterThanOrEqual(0);
      expect(box.y + box.height, `${where}: bottom`).toBeLessThanOrEqual(
        viewport.height,
      );
    }
  }
});

test("navigation is usable and the page has no horizontal overflow", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.evaluate(() => document.fonts.ready);
  await page.emulateMedia({ reducedMotion: "reduce" });
  if (testInfo.project.name === "mobile") {
    await page.getByRole("button", { name: "Open navigation" }).click();
    await expect(page.getByRole("navigation")).toBeVisible();
    await page
      .getByRole("navigation")
      .getByRole("link", { name: "How it works" })
      .click();
    await expect(
      page.getByRole("button", { name: "Open navigation" }),
    ).toHaveAttribute("aria-expanded", "false");
  } else {
    await page
      .getByRole("navigation")
      .getByRole("link", { name: "How it works" })
      .click();
  }
  await expect(page).toHaveURL(/#how$/);
  const widths =
    testInfo.project.name === "mobile"
      ? [280, 320, 375, 390, 430]
      : [768, 1024, 1440, 1920];
  for (const width of widths) {
    await page.setViewportSize({ width, height: 1000 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
      `overflow at ${width}px`,
    ).toBe(true);
    const containers = page.locator(".container");
    const count = await containers.count();
    for (let index = 0; index < count; index++) {
      const sizes = await containers
        .nth(index)
        .evaluate((el) => ({ scroll: el.scrollWidth, client: el.clientWidth }));
      expect(
        sizes.scroll,
        `container ${index} scrollWidth at ${width}px`,
      ).toBeLessThanOrEqual(sizes.client + 1);
    }
    const copy = await page.locator(".hero-copy").boundingBox();
    expect(copy, `hero-copy at ${width}px`).not.toBeNull();
    expect(
      copy.x + copy.width,
      `hero-copy right edge at ${width}px`,
    ).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth));
  }
});

test("main page and changed content have no automated accessibility violations", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.evaluate(() => document.fonts.ready);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const initial = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(initial.violations).toEqual([]);
  await page.getByRole("button", { name: "Does it work on Windows?" }).click();
  await page.getByRole("tab", { name: /Report/ }).click();
  await page.getByRole("button", { name: "Windows", exact: true }).click();
  await page
    .locator("summary", { hasText: "Set up without questions" })
    .click();
  await page.locator("summary", { hasText: "Build from source" }).click();
  await page.getByLabel("Planner", { exact: true }).selectOption("codex");
  await page.getByLabel("Implementer", { exact: true }).selectOption("claude");
  const changed = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(changed.violations).toEqual([]);
  await page.getByLabel("Planner", { exact: true }).selectOption("opencode");
  await page.getByLabel("Route requests first").selectOption("jev");
  const rerouted = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(rerouted.violations).toEqual([]);
  if (testInfo.project.name === "mobile") {
    await page.getByRole("button", { name: "Open navigation" }).click();
    const menuOpen = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
      .analyze();
    expect(menuOpen.violations).toEqual([]);
    await page.getByRole("button", { name: "Close navigation" }).click();
  }
});

test("page describes harness-bridge and names the old app only in one line", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".hero-description")).toContainText(
    "harness-bridge lets one coding agent hand work to another agent CLI",
  );
  const strip = page.locator(".integration-inner");
  for (const name of [
    "Codex CLI",
    "Claude Code",
    "Muse Code",
    "OpenCode",
    "Command Code",
    "Jev",
    "Laya",
  ]) {
    await expect(strip).toContainText(name);
  }
  await expect(strip.locator('svg[data-brand="meta"]')).toHaveCount(1);
  await expect(strip.locator(".integration-note")).toHaveText("self-hosted");
  await expect(page.locator('a[href$="README.md#install"]')).not.toHaveCount(0);
  await expect(page.locator('a[href*="#docker"]')).toHaveCount(0);

  const text = await page.evaluate(() => {
    const body = document.body.cloneNode(true);
    const legacy = body.querySelectorAll(".footer-legacy");
    const legacyText = [...legacy].map((node) => node.textContent);
    legacy.forEach((node) => node.remove());
    return {
      head: document.head.innerHTML,
      body: body.textContent,
      legacy: legacyText,
    };
  });
  expect(text.body).not.toMatch(/docker|compose\.ya?ml|magent/i);
  expect(text.head).not.toMatch(/docker|compose\.ya?ml|magent/i);
  expect(text.legacy).toEqual([
    "The previous app, magent, lives at tag magent-final.",
  ]);
  await expect(
    page.locator(".footer-legacy").getByRole("link", { name: "magent-final" }),
  ).toHaveAttribute("href", /\/tree\/magent-final$/);
});

test("agents section lists the five --to names", async ({ page }) => {
  await page.goto("/#agents");
  const cards = page.locator(".agent-card");
  await expect(cards).toHaveCount(5);
  for (const name of ["codex", "claude", "muse", "opencode", "commandcode"]) {
    await expect(page.locator("#agents")).toContainText(`--to ${name}`);
  }
  const muse = cards.filter({ hasText: "Muse Code" });
  await expect(muse).toContainText("Implements only");
  await expect(muse).toContainText("New job per fix");
  await expect(cards.filter({ hasText: "Plans and implements" })).toHaveCount(
    4,
  );
  await expect(page.locator(".agents-note")).toContainText(
    "Claude Code can’t implement for itself",
  );
});

test("jobs section lists the planner's tools, statuses and report fields", async ({
  page,
}) => {
  await page.goto("/#jobs");
  const jobs = page.locator("#jobs");
  await expect(
    page.getByRole("list", { name: "MCP tools" }).locator("li > code"),
  ).toHaveText(["delegate", "wait", "cancel", "route", "agents"]);
  for (const command of [
    "harness-bridge start < plan.md",
    "harness-bridge wait JOB --max 9m",
    "harness-bridge start --session ID < fixes.md",
    "harness-bridge cancel JOB",
    "harness-bridge run --to opencode < plan.md",
  ]) {
    await expect(jobs.locator(".shell-commands")).toContainText(command);
  }
  await expect(
    page.getByRole("list", { name: "Job statuses" }).getByRole("listitem"),
  ).toHaveText([
    "running",
    "completed",
    "failed",
    "needs_permission",
    "timed_out",
    "cancelled",
    "lost",
  ]);
  for (const field of ["session_id", "permission_denied", "dirty_before"]) {
    await expect(jobs).toContainText(field);
  }
  await expect(jobs).toContainText("HARNESS_BRIDGE_HOME");
});

test("routing section shows what harness-bridge route answers", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.evaluate(() => document.fonts.ready);
  await page.emulateMedia({ reducedMotion: "reduce" });
  if (testInfo.project.name === "mobile") {
    await page.getByRole("button", { name: "Open navigation" }).click();
  }
  await page
    .getByRole("navigation")
    .getByRole("link", { name: "Routing" })
    .click();
  await expect(page).toHaveURL(/#routing$/);
  const section = page.locator("#routing");
  await expect(
    section.getByRole("heading", { name: /Ask first/ }),
  ).toBeVisible();
  for (const kicker of [
    "01 · YOUR REQUEST",
    "02 · DECISION MODEL",
    "03 · ROUTE → PLANNER",
  ]) {
    await expect(section).toContainText(kicker);
  }
  await expect(
    section.getByRole("heading", { name: "Jev", exact: true }),
  ).toBeVisible();
  await expect(
    section.getByRole("heading", { name: /Laya self-hosted/ }),
  ).toBeVisible();
  const routes = section.locator(".routing-route");
  await expect(routes).toHaveCount(3);
  await expect(routes.nth(0)).toContainText("answer");
  await expect(routes.nth(0)).toContainText("Read-only answer");
  await expect(routes.nth(1)).toContainText("needs_planning");
  await expect(routes.nth(2)).toContainText("direct_implement");
  await expect(section).toContainText("harness-bridge route --provider laya");
  await expect(section.locator(".routing-fallback")).toContainText(
    "returns the safe choice, needs_planning",
  );
  await expect(section).toContainText("needs_full_review");
  const stripLink = page.locator(".integration-group-routing a", {
    hasText: "Laya",
  });
  await expect(stripLink).toHaveAttribute("href", "#routing");
});

test("faq explains routing with Jev and self-hosted Laya", async ({ page }) => {
  await page.goto("/#faq");
  await page
    .getByRole("button", { name: "Can a decision model route my requests?" })
    .click();
  const faq = page.locator(".faq-section");
  await expect(faq).toContainText("Jev is TypeSafe's hosted model");
  await expect(faq).toContainText("Laya is self-hosted");
  await expect(faq).toContainText("http://127.0.0.1:8765/v1/systemone");
  await expect(faq).toContainText("OPENROUTER_API_KEY");
  await page.getByRole("button", { name: "Do I need Go?" }).click();
  await expect(faq).toContainText("downloads a prebuilt harness-bridge");
  await page
    .getByRole("button", { name: "Does it write anything into my projects?" })
    .click();
  await expect(faq).toContainText("harness-bridge/config.json");
});

test("hero install command copies the line for this system", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");
  const platformId = await page.evaluate(() =>
    /Windows/i.test(navigator.userAgent) ? "windows" : "unix",
  );
  const expected = platforms.find((item) => item.id === platformId).install;
  await page.getByRole("button", { name: "Copy the install command" }).click();
  expect(await readClipboard(page)).toBe(expected);
});

test("stats band counts the agents", async ({ page }) => {
  await page.goto("/");
  const stats = page.getByRole("list", { name: "At a glance" });
  await expect(stats.getByRole("listitem")).toHaveCount(4);
  await expect(stats.getByRole("listitem").first()).toContainText(
    String(agents.length),
  );
});

test("mobile menu has a get started button and touch-sized links", async ({
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "mobile",
    "The sheet menu only exists on mobile.",
  );
  await page.goto("/");
  await page.getByRole("button", { name: "Open navigation" }).click();
  const navigation = page.getByRole("navigation");
  await expect(
    navigation.getByRole("link", { name: "Get started" }),
  ).toBeVisible();
  const links = navigation.getByRole("link");
  const count = await links.count();
  for (let index = 0; index < count; index++) {
    const box = await links.nth(index).boundingBox();
    expect(box.height, `link ${index}`).toBeGreaterThanOrEqual(44);
  }
});
