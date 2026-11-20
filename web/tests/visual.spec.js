// Visual regression baselines for a few key pages (mocked data, pinned clock).
// Unlike the other screenshot specs, these compare against committed images in
// test-snapshots/ and fail on a visible change. After an intended UI change,
// regenerate with `bun run test:update-snapshots` and review the image diff in
// the commit.
//
// The tolerance absorbs anti-aliasing differences between Chromium builds
// (the CI image vs. a local browser); layout, color or text changes still
// fail.
import { test, expect } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

const SCREENSHOT = { fullPage: true, maxDiffPixels: 50, threshold: 0.3 };

test.beforeEach(async ({ page }) => {
  await page.clock.setFixedTime(new Date("2026-03-28T12:00:00"));
  await mockLedgerApi(page);
});

async function open(page, path) {
  await page.goto(`/#${path}`);
  await expect(page.locator("h1").first()).toBeVisible();
  await page.waitForLoadState("networkidle");
  await expect(page.locator('[role="status"][aria-label="Loading"]')).toHaveCount(0);
  // Fonts load asynchronously; a fallback-font frame would be a false diff.
  await page.evaluate(() => document.fonts.ready);
}

const pages = [
  ["overview", "/"],
  ["transactions", "/transactions"],
  ["add-transaction", "/add"],
  ["monthly", "/monthly"],
  ["accounts", "/accounts"],
  ["portfolio", "/portfolio"],
];

for (const [name, path] of pages) {
  test(`visual: ${name}`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await open(page, path);
    await expect(page).toHaveScreenshot(`${name}.png`, SCREENSHOT);
  });
}

test("visual: overview dark", async ({ page }) => {
  // The theme is a stored preference (app-shell.jsx), not the OS setting.
  await page.addInitScript(() => localStorage.setItem("float-theme", "dark"));
  await page.setViewportSize({ width: 1280, height: 800 });
  await open(page, "/");
  await expect(page).toHaveScreenshot("overview-dark.png", SCREENSHOT);
});

test.describe("phone", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  test("visual: transactions phone", async ({ page }) => {
    await open(page, "/transactions");
    // Viewport only: a fullPage capture drops touch emulation in Chromium.
    await expect(page).toHaveScreenshot("transactions-phone.png", { ...SCREENSHOT, fullPage: false });
  });
});
