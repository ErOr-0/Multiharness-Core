import AxeBuilder from "@axe-core/playwright";
import { test, expect } from "@playwright/test";

test("workflow preview completes, replays, and cancels without backend calls", async ({
  page,
}) => {
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.goto("/");
  await page.evaluate(() => document.fonts.ready);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(page.getByRole("heading", { level: 1 })).toContainText(
    "Different agents.",
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

test("single container download and directory-independent start are clear", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");
  const pending = page.waitForEvent("download");
  await page.getByRole("link", { name: "Download setup" }).click();
  const download = await pending;
  expect(await download.failure()).toBeNull();
  expect(download.suggestedFilename()).toBe("multiharness-docker.zip");
  for (const platform of ["Windows", "macOS", "Linux"]) {
    await page.getByRole("button", { name: platform, exact: true }).click();
    await expect(page.locator(".install-steps > li")).toHaveCount(3);
    await page.getByRole("button", { name: "Copy Run setup" }).click();
    const setup = await page.evaluate(() => navigator.clipboard.readText());
    expect(setup).toContain(platform === "Windows" ? "setup.ps1" : "setup.sh");
    expect(setup).toContain(
      platform === "Windows" ? "$env:USERPROFILE" : "$HOME",
    );
    await expect(page.locator(".install-steps")).not.toContainText(".env");
    await page
      .getByRole("button", { name: "Copy Start from any folder" })
      .click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      "docker start -ai multiharness",
    );
  }
  await expect(
    page.getByRole("link", { name: "Download setup" }),
  ).toHaveAttribute("href", "/downloads/multiharness-docker.zip");
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
        "No separate pull command needed",
      );
      await expect(page.locator("#start")).not.toContainText("docker pull");
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
  const expanded = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(expanded.violations).toEqual([]);
});
