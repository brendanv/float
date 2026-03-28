import { test, expect } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("home page", async ({ page }) => {
  await page.goto("/#/");
  // Wait for data to load (balance summary or account list)
  await page.waitForSelector(".balance-summary, article, table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/home.png", fullPage: true });
});

test("transactions page", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/transactions.png", fullPage: true });
});

test("add transaction page", async ({ page }) => {
  await page.goto("/#/add");
  await page.waitForSelector("form", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/add-transaction.png", fullPage: true });
});

test("trends page", async ({ page }) => {
  await page.goto("/#/trends");
  await page.waitForSelector(".trends-chart canvas", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(1000);
  await page.screenshot({ path: "test-results/trends.png", fullPage: true });
});
