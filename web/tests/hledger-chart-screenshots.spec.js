import { test, expect } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

// hledger draws charts with its own light/dark palette keyed off
// prefers-color-scheme; floatd rescopes the dark palette to the app's `.dark`
// class, so the chart must follow the in-app theme toggle, not the OS.
const LIGHT_EXPENSE = "rgb(227, 73, 72)"; // hledger .fill.tx0 light (#e34948)
const DARK_EXPENSE = "rgb(230, 103, 103)"; // hledger .fill.tx0 dark (#e66767)

const expenseBarFill = (page) =>
  page
    .locator(".hledger-chart-host .hledger-chart path.fill.tx0")
    .first()
    .evaluate((el) => getComputedStyle(el).fill);

test.describe("light app theme on a dark OS", () => {
  test.use({ colorScheme: "dark" });

  test("home insights charts use the light palette", async ({ page }) => {
    await mockLedgerApi(page);
    await page.goto("/#/");
    await expect(page.locator(".hledger-chart-host svg.hledger-chart")).toHaveCount(2);
    expect(await expenseBarFill(page)).toBe(LIGHT_EXPENSE);
    const bg = await page
      .locator(".hledger-chart-host .hledger-chart rect.bg")
      .first()
      .evaluate((el) => getComputedStyle(el).fill);
    expect(bg).toBe("rgba(0, 0, 0, 0)");
  });
});

test("home insights charts in dark mode", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("float-theme", "dark"));
  await mockLedgerApi(page);
  await page.goto("/#/");
  await expect(page.locator(".hledger-chart-host svg.hledger-chart")).toHaveCount(2);
  expect(await expenseBarFill(page)).toBe(DARK_EXPENSE);
  await page.screenshot({ path: "test-results/home-dark.png", fullPage: true });
});

test("home insights empty state", async ({ page }) => {
  await mockLedgerApi(page, { chartsEmpty: true });
  await page.goto("/#/");
  await expect(page.getByText("No expense or revenue data for this period.")).toBeVisible();
  await expect(page.locator(".hledger-chart-host")).toHaveCount(0);
});
