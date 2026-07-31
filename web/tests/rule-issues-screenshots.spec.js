import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("rule issues dialog - found issues", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  const btn = page.getByRole("button", { name: /find issues/i });
  await btn.click();
  await page.waitForTimeout(700);
  await page.screenshot({ path: "test-results/rule-issues-found.png", fullPage: true });
});

test("rule issues dialog - no issues", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);

  await page.route("**/float.v1.LedgerService/FindRuleIssues", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ issues: [] }),
    });
  });

  const btn = page.getByRole("button", { name: /find issues/i });
  await btn.click();
  await page.waitForTimeout(700);
  await page.screenshot({ path: "test-results/rule-issues-empty.png", fullPage: true });
});
