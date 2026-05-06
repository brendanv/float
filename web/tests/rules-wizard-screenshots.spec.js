import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("rules wizard - step 1 source", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  const btn = page.getByRole("button", { name: /suggest with ai/i });
  await btn.click();
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/rules-wizard-step1.png", fullPage: true });
});

test("rules wizard - step 2 suggestions", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  const btn = page.getByRole("button", { name: /suggest with ai/i });
  await btn.click();
  await page.waitForTimeout(400);
  const analyzeBtn = page.getByRole("button", { name: /analyze/i });
  await analyzeBtn.click();
  await page.waitForTimeout(1000);
  await page.screenshot({ path: "test-results/rules-wizard-step2.png", fullPage: true });
});
