import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test("connections page with fetch date controls", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/connections");
  await page.waitForSelector("text=Chase Checking", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/connections-fetch-date.png", fullPage: true });
});

test("connections page - update fetch date dialog open", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/connections");
  await page.waitForSelector("text=Chase Checking", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);

  // Click the calendar button on the first account
  await page.locator("button[title='Update last fetched date']").first().click();
  await page.waitForSelector("text=Update Fetch Date", { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/connections-update-fetch-date-dialog.png", fullPage: true });
});

test("connections page - update fetch date dialog with date set", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/connections");
  await page.waitForSelector("text=Chase Checking", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);

  // Open dialog and fill in a date
  await page.locator("button[title='Update last fetched date']").first().click();
  await page.waitForSelector("text=Update Fetch Date", { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(200);
  await page.locator("input[type='date']").fill("2026-01-01");
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/connections-update-fetch-date-filled.png", fullPage: true });
});
