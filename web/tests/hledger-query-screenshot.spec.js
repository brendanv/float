import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("hledger query page - empty", async ({ page }) => {
  await page.goto("/#/hledger-query");
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/hledger-query-empty.png", fullPage: true });
});

test("hledger query page - with result", async ({ page }) => {
  await page.goto("/#/hledger-query");
  await page.waitForTimeout(300);
  await page.fill("textarea", "bal --depth 2");
  await page.waitForTimeout(100);
  await page.click("button:has-text('Run')");
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/hledger-query-result.png", fullPage: true });
});
