import { test, expect } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

test("connections page with fetch date controls", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/connections");
  await page.waitForSelector("text=Chase Checking", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/connections-fetch-date.png", fullPage: true });
});

// Exercises the server-streaming mocks end to end: the fixture validates each
// streamed message against the proto, and the UI must read the result frame.
test("refresh all streams progress and results per account", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/connections");
  await page.getByRole("button", { name: "Refresh & Fetch All" }).click();
  await expect(page.getByText("Refreshed").first()).toBeVisible();
});
