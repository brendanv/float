import { test } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

test("connections page with fetch date controls", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/connections");
  await page.waitForSelector("text=Chase Checking", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/connections-fetch-date.png", fullPage: true });
});
