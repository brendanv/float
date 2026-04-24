import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("home - date range picker open", async ({ page }) => {
  await page.goto("/#/");
  await page.waitForTimeout(600);
  // Click the date range picker button (shows current month)
  const btn = page.locator('button').filter({ has: page.locator('svg') }).filter({ hasText: /\d{4}/ }).first();
  await btn.click();
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/home-datepicker-open.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 620 } });
});

test("transactions - date range picker open", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(400);
  // Click the date range picker button
  const btn = page.locator('button').filter({ has: page.locator('svg') }).filter({ hasText: /\d{4}/ }).first();
  await btn.click();
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/transactions-datepicker-open.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 640 } });
});

test("home - date range picker mobile closed", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/");
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/home-datepicker-mobile.png", fullPage: false, clip: { x: 0, y: 0, width: 390, height: 160 } });
});

test("home - date range picker mobile open", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/");
  await page.waitForTimeout(600);
  const btn = page.locator('button').filter({ has: page.locator('svg') }).filter({ hasText: /\d{4}/ }).first();
  await btn.click();
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/home-datepicker-mobile-open.png", fullPage: false, clip: { x: 0, y: 0, width: 390, height: 844 } });
});

test("transactions - date range picker mobile open", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/transactions");
  await page.waitForSelector(".card", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(400);
  const btn = page.locator('button').filter({ has: page.locator('svg') }).filter({ hasText: /\d{4}/ }).first();
  await btn.click();
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/transactions-datepicker-mobile-open.png", fullPage: false, clip: { x: 0, y: 0, width: 390, height: 844 } });
});
