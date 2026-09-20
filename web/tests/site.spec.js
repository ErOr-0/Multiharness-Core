import AxeBuilder from "@axe-core/playwright";
import { test, expect } from "@playwright/test";
import { launchCommand, linuxPolicyCommand } from "../src/docker-install.js";

async function readClipboard(page) {
  return (await page.evaluate(() => navigator.clipboard.readText())).replace(
    /\r\n/g,
    "\n",
  );
}

test("one agent setup is the default and needs no planner or reviewer", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#start");
  await expect(page.getByRole("group", { name: "Team role" })).toHaveCount(0);
  await page.getByLabel("Agent model").fill("provider/model");
  await page.getByRole("button", { name: "Copy Agent settings" }).click();
  expect(await readClipboard(page)).toBe(
    '/set mode direct\n/set implementer-harness opencode\n/set implementer-model provider/model\n/set implementer-variant ""\n/save',
  );
});

test("mixed team settings survive role switching and copy all roles", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#start");
  await page
    .getByRole("button", { name: "Team workflow", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Copy Team settings" }),
  ).toHaveCount(0);
  await page.getByLabel("Planner model").fill("plan-model");
  await page.getByRole("button", { name: "Implementer", exact: true }).click();
  await page.getByLabel("Implementer model").fill("provider/build");
  await page.getByRole("button", { name: "Reviewer", exact: true }).click();
  await page.getByLabel("Reviewer harness").selectOption("claude");
  await page.getByLabel("Reviewer model").fill("review-model");
  await page.getByRole("button", { name: "Copy Team settings" }).click();
  expect(await readClipboard(page)).toBe(
    [
      "/set mode team",
      "/set planner-harness codex",
      "/set planner-model plan-model",
      "/set planner-reasoning high",
      "/set implementer-harness opencode",
      "/set implementer-model provider/build",
      '/set implementer-variant ""',
      "/set reviewer-harness claude",
      "/set reviewer-model review-model",
      "/set reviewer-reasoning high",
      "/save",
    ].join("\n"),
  );
  await page.getByRole("button", { name: "Planner", exact: true }).click();
  await expect(page.getByLabel("Planner model")).toHaveValue("plan-model");
  await page.getByLabel("Planner harness").selectOption("opencode");
  await expect(page.getByLabel("Planner model")).toHaveValue("");
  await expect(page.getByLabel("Planner variant (optional)")).toHaveValue("");
  await expect(
    page.getByRole("button", { name: "Copy Team settings" }),
  ).toHaveCount(0);
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
    "One task.",
  );
  await page.getByRole("button", { name: "Run workflow preview" }).click();
  await expect(
    page.getByRole("button", { name: "Cancel workflow preview" }),
  ).toBeVisible();
  await expect(page.locator(".demo-status.complete")).toContainText(
    "Approved",
    { timeout: 12000 },
  );
  await page.getByRole("button", { name: "Replay workflow preview" }).click();
  await page.getByRole("button", { name: "Cancel workflow preview" }).click();
  await expect(page.locator(".demo-status")).toContainText("Cancelled");
  expect(pageErrors).toEqual([]);
});

test("Docker commands use the selected path with no setup script", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#start");
  for (const platform of ["Windows", "macOS", "Linux"]) {
    await page.getByRole("button", { name: platform, exact: true }).click();
    await expect(page.locator(".install-steps > li")).toHaveCount(3);
    await expect(
      page.getByRole("button", { name: "Copy Create and open" }),
    ).toHaveCount(0);
    await page.getByLabel("Full projects folder path").fill("relative/path");
    await expect(
      page.getByRole("button", { name: "Copy Create and open" }),
    ).toHaveCount(0);
    const folder =
      platform === "Windows" ? "D:\\My Projects" : "/path/to/My Projects";
    await page.getByLabel("Full projects folder path").fill(folder);
    await page.getByRole("button", { name: "Copy Create and open" }).click();
    const launch = await readClipboard(page);
    expect(launch).toBe(launchCommand(folder, platform, platform === "Linux"));
    expect(launch).not.toMatch(/setup\.(sh|ps1)|unconfined|--privileged/);
    await page.getByRole("button", { name: "Copy Pull image" }).click();
    expect(await readClipboard(page)).toBe("docker pull er0r2/multiharness");
    await page
      .getByRole("button", { name: "Copy Start from any folder" })
      .click();
    expect(await readClipboard(page)).toBe("docker start -ai multiharness");
  }
  await page.getByText("View full Docker command", { exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Create and open", exact: true }),
  ).toContainText("docker compose");
  await page
    .getByText("Install Linux policy once", {
      exact: true,
    })
    .click();
  await page.getByRole("button", { name: "Copy Install Linux policy" }).click();
  expect(await readClipboard(page)).toBe(linuxPolicyCommand);
  await page.getByLabel("AppArmor host (e.g. Ubuntu)").uncheck();
  await page.getByRole("button", { name: "Copy Create and open" }).click();
  expect(await readClipboard(page)).not.toContain("compose.linux.yaml");
});

test("desktop installation fits in one view for every platform", async ({
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name !== "desktop",
    "Small screens keep readable stacked steps.",
  );
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/#start");
  await page.evaluate(() => document.fonts.ready);
  for (const viewport of [
    { width: 1440, height: 900 },
    { width: 1280, height: 800 },
    { width: 1024, height: 768 },
  ]) {
    await page.setViewportSize(viewport);
    for (const platform of ["Windows", "macOS", "Linux"]) {
      await page.getByRole("button", { name: platform, exact: true }).click();
      await page
        .getByLabel("Full projects folder path")
        .fill(platform === "Windows" ? "D:\\Projects" : "/path/to/Projects");
      await page.locator("#start").evaluate((el) => el.scrollIntoView());
      const panel = await page.locator("#start").boundingBox();
      expect(
        panel.y,
        `${platform}: top at ${viewport.width}`,
      ).toBeGreaterThanOrEqual(0);
      expect(
        panel.y + panel.height,
        `${platform}: bottom at ${viewport.width}`,
      ).toBeLessThanOrEqual(viewport.height);
      await expect(page.locator("#start")).toContainText(
        "No setup script or ZIP download",
      );
      await expect(page.locator("#start")).not.toContainText("setup.sh");
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
      .getByRole("link", { name: "The workflow" })
      .click();
    await expect(
      page.getByRole("button", { name: "Open navigation" }),
    ).toHaveAttribute("aria-expanded", "false");
  } else {
    await page
      .getByRole("navigation")
      .getByRole("link", { name: "The workflow" })
      .click();
  }
  await expect(page).toHaveURL(/#workflow$/);
  const widths =
    testInfo.project.name === "mobile"
      ? [320, 375, 390, 430]
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

test("main page and expanded content have no automated accessibility violations", async ({
  page,
}) => {
  await page.goto("/");
  await page.evaluate(() => document.fonts.ready);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const initial = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(initial.violations).toEqual([]);
  await page.getByRole("button", { name: "Windows", exact: true }).click();
  await page.getByRole("button", { name: "Can I run it on Windows?" }).click();
  await page.getByLabel("Full projects folder path").fill("D:\\Projects");
  await page.getByText("View full Docker command", { exact: true }).click();
  const expanded = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(expanded.violations).toEqual([]);
});

test("introduction explains local use and independent provider choices", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".hero-description")).toContainText(
    "Run it on your computer with Docker",
  );
  for (const role of ["planner", "builder", "reviewer"]) {
    const select = page.getByLabel(`Example ${role} provider`);
    await expect(select.locator("option")).toHaveText([
      "Codex",
      "OpenCode",
      "Claude Code",
    ]);
  }
  await page.getByLabel("Example planner provider").selectOption("Claude Code");
  await expect(page.getByLabel("Example builder provider")).toHaveValue(
    "OpenCode",
  );
  await expect(page.locator('a[href*="/blob/main/docs"]')).toHaveCount(0);
  await expect(page.locator('a[href*="README.md#docker-setup"]')).toHaveCount(
    1,
  );
});

test("download guide separates requirements, included agents, and optional Jev setup", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#download");
  await expect(
    page.getByRole("link", { name: "Download configuration ZIP" }),
  ).toHaveAttribute(
    "href",
    /releases\/download\/v0\.1\.0-alpha\.17\/multiharness-docker\.zip$/,
  );
  await expect(page.locator("#requirements")).toContainText(
    "Codex, OpenCode, and Claude Code are already included",
  );
  await page
    .getByText("Running natively instead of Docker?", { exact: true })
    .click();
  await expect(page.locator(".native-setup-note")).toContainText(
    "with your confirmation",
  );
  await expect(page.locator(".native-setup-note")).toContainText(
    "sign in and rerun your task",
  );
  await page
    .getByRole("link", { name: "Start installation", exact: true })
    .click();
  await expect(page).toHaveURL(/#start$/);
  await page.getByRole("button", { name: "Copy Enable Jev routing" }).click();
  expect(await readClipboard(page)).toBe(
    "/set mode team\n/set decision-enabled true\n/save",
  );
  await expect(page.locator(".jev-guide")).toContainText("off by default");
  await expect(page.locator(".jev-guide")).toContainText("hidden input");
  await expect(page.locator(".jev-guide")).toContainText(
    "no configured checks keep full review",
  );
  await expect(page.locator(".jev-guide input")).toHaveCount(0);
  for (const route of [
    "Answer a question",
    "Plan a change",
    "Implement directly",
  ]) {
    await expect(page.locator(".jev-routes")).toContainText(route);
  }
  await expect(
    page.getByRole("link", { name: "View the live API test results" }),
  ).toHaveCount(0);
  await expect(page.locator(".config-guide")).toContainText("/configuration");
  await page
    .getByText("Already installed? Update your container", { exact: true })
    .click();
  await expect(page.locator(".quick-update")).toContainText(
    "Enter your existing projects folder above",
  );
  await page.getByLabel("Full projects folder path").fill("D:\\Projects");
  await page.getByRole("button", { name: "Copy Update and open" }).click();
  const update = await readClipboard(page);
  expect(update).toContain("create --pull always --force-recreate");
  expect(update).toContain("if ($LASTEXITCODE -eq 0)");
  expect(update).toContain("D:\\Projects");
});
