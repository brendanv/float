// Server-streaming RPCs end to end against the mocks: fulfillStream checks
// each message against the proto, and the UI must act on the result frame.
import { test, expect } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("apply rules streams progress then the applied count", async ({ page }) => {
  await page.goto("/#/rules");
  await page.getByRole("button", { name: "Preview Changes" }).click();
  await page.getByRole("button", { name: /^Apply to \d+ Transaction/ }).click();
  await expect(page.getByText("Applied changes to 3 transaction(s).")).toBeVisible();
});

test("refresh all streams progress and results per account", async ({ page }) => {
  await page.goto("/#/connections");
  await page.getByRole("button", { name: "Refresh & Fetch All" }).click();
  await expect(page.getByText("Refreshed").first()).toBeVisible();
});
