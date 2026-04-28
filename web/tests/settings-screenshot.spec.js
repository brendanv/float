import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test("settings page", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/settings");
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/settings.png", fullPage: true });
});
