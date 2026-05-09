import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test("error banner - multiline hledger error", async ({ page }) => {
  await mockLedgerApi(page);

  // Override AddTransaction (not CreateTransaction) to return an error
  await page.route("**/float.v1.LedgerService/AddTransaction", async (route) => {
    await route.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({
        code: "invalid_argument",
        message:
          'hledger: Error: transaction fails balance assertion\nin file "/data/2026/05.journal" at line 42:\n\n  2026-05-09 Grocery Store\n    expenses:food          $50.00\n    assets:checking       $-50.00 = $1234.56  ; balance assertion failed\n\nExpected: $1234.56, got: $1200.00',
      }),
    });
  });

  await page.goto("/#/add");
  await page.waitForSelector("form", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);

  await page.fill('input[placeholder="e.g. Grocery store"]', "Grocery Store");

  const combos = page.locator('button[role="combobox"]');
  await combos.nth(0).click();
  await page.waitForTimeout(200);
  await page.fill('input[placeholder*="Search"]', "expenses:food");
  await page.waitForTimeout(200);
  await page.locator('[role="option"]').first().click();
  await page.waitForTimeout(200);
  await page.locator('input[placeholder="0.00"]').first().fill("50");

  await combos.nth(1).click();
  await page.waitForTimeout(200);
  await page.fill('input[placeholder*="Search"]', "assets:checking");
  await page.waitForTimeout(200);
  await page.locator('[role="option"]').first().click();
  await page.waitForTimeout(200);

  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(800);

  await page.screenshot({ path: "test-results/error-banner.png", fullPage: true });
});
