import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test("custom dashboards page", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/dashboards");
  await page.waitForTimeout(600);
  await page.screenshot({
    path: "test-results/custom-dashboards.png",
    fullPage: true,
  });
});
