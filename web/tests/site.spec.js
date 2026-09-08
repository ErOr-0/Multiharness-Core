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
  await page.getByRole("link", { name: "Download configuration" }).click();
  const download = await pending;
  expect(await download.failure()).toBeNull();
  expect(download.suggestedFilename()).toBe("multiharness-docker.zip");
  for (const platform of ["Windows", "macOS", "Linux"]) {
    await page.getByRole("button", { name: platform, exact: true }).click();
    await expect(page.locator(".install-steps > li")).toHaveCount(4);
    await page.getByRole("button", { name: "Copy Open settings" }).click();
    const edit = await page.evaluate(() => navigator.clipboard.readText());
    expect(edit).toContain(".env.example");
    expect(edit).toContain(platform === "Windows" ? "Test-Path" : "test -f");
    expect(edit).toContain(
      platform === "Windows"
        ? "notepad"
        : platform === "macOS"
          ? "open -e"
          : "nano",
    );
    await page
      .getByRole("button", { name: "Copy Create once", exact: true })
      .click();
    const create = await page.evaluate(() => navigator.clipboard.readText());
    expect(create).toContain("up --no-start");
    expect(create).toContain(
      platform === "Windows" ? "$env:USERPROFILE" : "$HOME",
    );
    if (platform === "Linux") {
      expect(create).toContain("compose.linux.yaml");
      await page
        .getByRole("checkbox", { name: "This Linux host uses AppArmor" })
        .uncheck();
      await page
        .getByRole("button", { name: "Copy Create once", exact: true })
        .click();
      expect(
        await page.evaluate(() => navigator.clipboard.readText()),
      ).not.toContain("compose.linux.yaml");
      await expect(
        page.getByRole("button", { name: "Copy Install Linux policy once" }),
      ).toHaveCount(0);
    }
    await page
      .getByRole("button", { name: "Copy Start from any folder" })
      .click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      "docker start -ai multiharness",
    );
  }
  await expect(
    page.getByRole("link", { name: "Download configuration" }),
  ).toHaveAttribute("href", "/downloads/multiharness-docker.zip");
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
