import { test } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("templates page", async ({ page }) => {
  await page.goto("/#/templates");
  await page.waitForSelector("table, h2", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/templates.png", fullPage: true });
});

test("templates page - new template form", async ({ page }) => {
  await page.goto("/#/templates");
  await page.waitForSelector("table, h2", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Click "New Template" button
  await page.getByRole("button", { name: "New Template" }).click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/templates-new-form.png", fullPage: true });
});

test("transactions page - duplicate button", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  // Click first transaction row to expand it
  await page.locator("tbody tr").first().click();
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/transactions-duplicate.png", fullPage: true });
});

test("lots page", async ({ page }) => {
  await page.goto("/#/lots");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/lots.png", fullPage: true });
});

test("lots page - mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/lots");
  await page.waitForSelector("text=drip", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/lots-mobile.png", fullPage: true });
});

test("transactions page - lot disposal expanded", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.locator("tbody tr", { hasText: "Sell AAPL" }).first().getByText("2026-03-20").click();
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/transactions-lot-disposal.png", fullPage: true });
});
