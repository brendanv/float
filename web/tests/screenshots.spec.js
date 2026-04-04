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
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
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

test("prices page", async ({ page }) => {
  await page.goto("/#/prices");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/prices.png", fullPage: true });
});

test("transactions page - filter dropdown open", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  // Click the quick filter dropdown button (last btn-ghost/btn-primary in the first row)
  const filterBtn = page.locator("button").filter({ hasText: /^(All|Reviewed|Unreviewed|No payee set|Filter)\s*▾?$/ }).first();
  await filterBtn.click();
  await page.waitForTimeout(150);
  await page.screenshot({ path: "test-results/transactions-filter-open.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 300 } });
});

test("transactions page - mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/transactions");
  await page.waitForSelector(".card", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-mobile.png", fullPage: true });
});

test("transactions page - payee filter", async ({ page }) => {
  await page.goto("/#/transactions?payee=Whole+Foods+Market");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-payee-filter.png", fullPage: true });
});

test("transactions page - mobile bulk edit toolbar", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/transactions");
  await page.waitForSelector(".card", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  const checkboxes = await page.locator(".card input[type=checkbox]").all();
  for (const cb of checkboxes.slice(0, 3)) {
    await cb.click();
  }
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/transactions-mobile-bulk-edit.png", fullPage: true });
});

test("transactions page - bulk edit toolbar", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  // Check the first three transaction checkboxes
  const checkboxes = await page.locator("tbody input[type=checkbox]").all();
  for (const cb of checkboxes.slice(0, 3)) {
    await cb.click();
  }
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/transactions-bulk-edit.png", fullPage: true });
});

test("hamburger icon - closed state", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/");
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/hamburger-closed.png", clip: { x: 0, y: 0, width: 390, height: 80 } });
});

test("hamburger icon - open state", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/");
  await page.waitForTimeout(400);
  // Dismiss any Vite error overlay
  await page.keyboard.press("Escape");
  await page.waitForTimeout(200);
  // Check the swap checkbox directly to toggle to open state
  await page.evaluate(() => {
    const cb = document.querySelector("label.swap-rotate input[type=checkbox]");
    if (cb) { cb.checked = true; cb.dispatchEvent(new Event("change")); }
  });
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/hamburger-open.png", clip: { x: 0, y: 0, width: 390, height: 80 } });
});
