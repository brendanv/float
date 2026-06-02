import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test.use({ viewport: { width: 390, height: 844 } });

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("rules page mobile", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/mobile-rules.png", fullPage: true });
});

test("payees page mobile", async ({ page }) => {
  await page.goto("/#/payees");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/mobile-payees.png", fullPage: true });
});

test("monthly dashboard mobile", async ({ page }) => {
  await page.goto("/#/monthly");
  await page.waitForSelector("table, canvas, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(800);
  await page.screenshot({ path: "test-results/mobile-monthly.png", fullPage: true });
});

test("balance assertions mobile", async ({ page }) => {
  await page.goto("/#/assertions");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-assertions.png", fullPage: true });
});

test("portfolio mobile", async ({ page }) => {
  await page.goto("/#/portfolio");
  await page.waitForSelector("table, canvas, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(800);
  await page.screenshot({ path: "test-results/mobile-portfolio.png", fullPage: true });
});

test("imports history mobile", async ({ page }) => {
  await page.goto("/#/imports");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-imports-history.png", fullPage: true });
});

test("snapshots mobile", async ({ page }) => {
  await page.goto("/#/snapshots");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-snapshots.png", fullPage: true });
});

test("accounts page mobile", async ({ page }) => {
  await page.goto("/#/accounts");
  await page.waitForSelector('[data-slot="card"], .loading', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-accounts.png", fullPage: true });
});
