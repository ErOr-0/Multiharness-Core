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

test("Docker setup downloads a folder configuration and copies the launch command", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");
  await page.getByRole("button", { name: "Windows", exact: true }).click();
  await page
    .getByLabel("Existing folder on your computer")
    .fill("D:\\Projects\\My App");
  const downloadPromise = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Download Docker configuration" })
    .click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("multiharness-docker.zip");
  expect(await download.failure()).toBeNull();
  await page.getByRole("button", { name: "Copy launch command" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    "docker compose run --rm magent",
  );
  await page.getByRole("button", { name: "Linux", exact: true }).click();
  await expect(
    page.getByRole("link", {
      name: "load the scoped AppArmor profile and set your UID/GID",
    }),
  ).toHaveAttribute("href", /#linux-apparmor-setup$/);
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
