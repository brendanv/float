import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("transactions page - expand row with cost-annotated posting", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.click("tr:has-text('buy AAPL')");
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/posting-with-price-edit.png", fullPage: true });
});

test("add transaction page - reveal price (cost) inputs", async ({ page }) => {
  await page.goto("/#/add");
  await page.waitForSelector("form", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  // Click the Tag (Add price) button on the first posting row
  await page.locator("button[aria-label='Add price']").first().click();
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/posting-add-price.png", fullPage: true });
});
