import AxeBuilder from "@axe-core/playwright";
import { test, expect } from "@playwright/test";
import { installCommand } from "../src/bridge-setup.js";

async function readClipboard(page) {
  return (await page.evaluate(() => navigator.clipboard.readText())).replace(
    /\r\n/g,
    "\n",
  );
}

const lockLabel = "Lock the planner (Claude Code only)";

test("setup picker shows the matching harness-bridge setup command", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#start");
  const setup = page.locator("#start");
  const planner = page.getByLabel("Planner", { exact: true });
  const implementer = page.getByLabel("Implementer", { exact: true });
  await expect(planner).toHaveValue("claude");
  await expect(implementer).toHaveValue("codex");
  await page.getByRole("button", { name: "Copy Setup command" }).click();
  expect(await readClipboard(page)).toBe(
    "harness-bridge setup --planner claude --implementer codex",
  );
  await expect(setup).toContainText(".claude/agents/codex-implementer.md");
  await expect(setup).toContainText("CLAUDE.md");
  await page.getByRole("button", { name: "Copy Sign in once" }).click();
  expect(await readClipboard(page)).toBe("claude auth login\ncodex login");

  await page.getByLabel(lockLabel).check();
  await page.getByLabel("Route requests first").selectOption("laya");
  await page.getByRole("button", { name: "Copy Setup command" }).click();
  expect(await readClipboard(page)).toBe(
    "harness-bridge setup --planner claude --implementer codex --lock-planner --router laya",
  );
  await expect(setup).toContainText(".claude/settings.json");

  await planner.selectOption("opencode");
  await implementer.selectOption("muse");
  await expect(page.getByLabel(lockLabel)).toBeDisabled();
  await expect(page.getByLabel(lockLabel)).not.toBeChecked();
  await page.getByRole("button", { name: "Copy Setup command" }).click();
  expect(await readClipboard(page)).toBe(
    "harness-bridge setup --planner opencode --implementer muse --router laya",
  );
  await expect(setup).toContainText("setup writes AGENTS.md.");
  await expect(setup).toContainText("harness-bridge start --to muse");
  await expect(setup).toContainText("Muse Code can’t continue a session");
  await page.getByRole("button", { name: "Copy Sign in once" }).click();
  expect(await readClipboard(page)).toBe("opencode auth login\nmuse login");
});

test("Claude Code cannot be both planner and implementer", async ({ page }) => {
  await page.goto("/#start");
  await page.getByLabel("Implementer", { exact: true }).selectOption("claude");
  await expect(page.locator("#start").getByRole("alert")).toContainText(
    "can’t both be Claude Code",
  );
  await expect(
    page.getByRole("button", { name: "Copy Setup command" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Copy Sign in once" }),
  ).toHaveCount(0);
  await page.getByLabel("Planner", { exact: true }).selectOption("codex");
  await expect(
    page.getByRole("button", { name: "Copy Setup command" }),
  ).toHaveCount(1);
});

test("install step clones the repository and runs go install", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#start");
  await page
    .getByRole("button", { name: "Copy Install harness-bridge" })
    .click();
  const install = await readClipboard(page);
  expect(install).toBe(installCommand);
  expect(install).toContain("go install ./cmd/harness-bridge");
  await expect(page.locator(".setup-feedback")).toHaveText(
    "Install harness-bridge copied.",
  );
  await expect(page.locator("#start")).toContainText("$(go env GOPATH)/bin");
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

test("desktop setup panel fits in one view for every pairing", async ({
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
    for (const [planner, implementer] of [
      ["claude", "muse"],
      ["codex", "claude"],
    ]) {
      await page.getByLabel("Planner", { exact: true }).selectOption(planner);
      await page
        .getByLabel("Implementer", { exact: true })
        .selectOption(implementer);
      await page.getByLabel("Route requests first").selectOption("laya");
      await panel.evaluate((el) => el.scrollIntoView());
      const box = await panel.boundingBox();
      const label = `${planner}/${implementer} at ${viewport.width}`;
      expect(box.y, `${label}: top`).toBeGreaterThanOrEqual(0);
      expect(box.y + box.height, `${label}: bottom`).toBeLessThanOrEqual(
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
  }
});

test("main page and changed content have no automated accessibility violations", async ({
  page,
}) => {
  await page.goto("/");
  await page.evaluate(() => document.fonts.ready);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const initial = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(initial.violations).toEqual([]);
  await page.getByRole("button", { name: "Does it work on Windows?" }).click();
  await page.getByRole("tab", { name: /Report/ }).click();
  await page.getByLabel("Planner", { exact: true }).selectOption("muse");
  await page.getByLabel("Implementer", { exact: true }).selectOption("claude");
  const changed = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(changed.violations).toEqual([]);
  await page.getByLabel("Planner", { exact: true }).selectOption("claude");
  const invalid = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(invalid.violations).toEqual([]);
});

test("page describes harness-bridge and names the old app only in one line", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".hero-description")).toContainText(
    "harness-bridge is a small Go command",
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
  await expect(
    page.locator('a[href$="README.md#set-up-a-project"]'),
  ).not.toHaveCount(0);
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
  await expect(cards.filter({ hasText: "Muse Code" })).toContainText(
    "New job per fix",
  );
  await expect(page.locator(".agents-note")).toContainText(
    "can’t both be Claude Code",
  );
});

test("jobs section lists the job commands, statuses and report fields", async ({
  page,
}) => {
  await page.goto("/#jobs");
  const jobs = page.locator("#jobs");
  for (const command of [
    "harness-bridge start --to codex < plan.md",
    "harness-bridge wait JOB --max 9m",
    "harness-bridge start --to codex --session ID < fixes.md",
    "harness-bridge cancel JOB",
    "harness-bridge run --to codex < plan.md",
  ]) {
    await expect(jobs).toContainText(command);
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
});
