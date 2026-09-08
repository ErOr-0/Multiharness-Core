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

test("exploration, platform commands, model example and FAQs respond to input", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");
  await page.evaluate(() => document.fonts.ready);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.getByRole("tab", { name: "04 Review & repair" }).click();
  await expect(page.getByRole("tabpanel")).toContainText(
    "A second perspective. A clear finish.",
  );
  await page.getByRole("tab", { name: "04 Review & repair" }).press("Home");
  await expect(page.getByRole("tab", { name: "01 Plan" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.getByLabel("Example planner model").selectOption("gpt-6-astra");
  await page.getByLabel("Example builder model").selectOption("");
  await expect(page.getByLabel("Example planner model")).toHaveValue(
    "gpt-6-astra",
  );
  await expect(page.getByLabel("Example builder model")).toHaveValue("");
  await expect(page.getByLabel("Example reviewer model")).toHaveValue(
    "gpt-6-astra",
  );
  await page.getByLabel("Example reviewer model").selectOption("gpt-5.6-sol");
  await expect(page.getByLabel("Example planner model")).toHaveValue(
    "gpt-6-astra",
  );
  await page.getByLabel("Example builder model").selectOption("big-pickle");
  await expect(page.getByLabel("Example builder model")).toHaveValue(
    "big-pickle",
  );
  await expect(page.getByLabel("Example reviewer model")).toHaveValue(
    "gpt-5.6-sol",
  );
  await page.getByRole("button", { name: "Windows", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "1. First-time setup command" }),
  ).toContainText(".\\scripts\\magent-docker.ps1");
  await page.getByRole("button", { name: "Copy setup command" }).click();
  await expect(
    page.getByRole("button", { name: "Copy setup command" }),
  ).toContainText("Copied!");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    ".\\scripts\\magent-docker.ps1 -Project 'D:\\Projects\\My App' -Command setup",
  );
  await page.getByRole("button", { name: "Copy launch command" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    ".\\scripts\\magent-docker.ps1 -Project 'D:\\Projects\\My App'",
  );
  await page.getByRole("button", { name: "Linux", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Copy launch command" }),
  ).toHaveText("Copy");
  await page.getByRole("button", { name: "Copy launch command" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    "sh ./scripts/magent-docker.sh --project '/path/to/My App'",
  );
  await page.getByRole("button", { name: "Can I run it on Windows?" }).click();
  await expect(
    page.getByRole("region", { name: "Can I run it on Windows?" }),
  ).toContainText("Docker Desktop in Linux-container mode");
  await expect(
    page.getByRole("link", { name: "Download launcher ZIP" }),
  ).toHaveAttribute(
    "href",
    "https://github.com/ErOr-0/Multiharness-Core/releases/download/v0.1.0-alpha.3/magent_docker_0.1.0-alpha.3.zip",
  );
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
