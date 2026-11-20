import { expect, test } from "./fixtures.js";
import { mockLedgerApi, mockUnmatchedTransfers } from "./mock-api.js";

test.describe("with unmatched transfers", () => {
  test.beforeEach(async ({ page }) => {
    await mockLedgerApi(page, { unmatchedTransfers: mockUnmatchedTransfers });
  });

  test("assertions page lists unmatched transfers", async ({ page }) => {
    await page.goto("/#/assertions");
    const section = page.getByRole("region", { name: "Unmatched transfers" });
    await expect(section).toBeVisible();
    const rows = section.locator("[data-slot=unmatched-transfer-row]");
    await expect(rows).toHaveCount(3);
    await expect(rows.nth(1)).toContainText("Missing");
    await expect(rows.nth(1)).toContainText("Payment thank you");
    await expect(rows.nth(0)).toContainText("Recent");
    await expect(section).toContainText("equity:transfers");

    await page.screenshot({ path: "test-results/unmatched-transfers.png", fullPage: true });

    await rows.nth(1).getByRole("link", { name: "Payment thank you" }).click();
    await expect(page).toHaveURL(/#\/transactions\?search=code%3Af1e2d3c5/);
  });

  test("home shows a notice for missing transfers only", async ({ page }) => {
    await page.goto("/#/");
    const notice = page.getByRole("status").filter({ hasText: "missing the other side" });
    await expect(notice).toContainText("2 transfers are");
    await notice.getByRole("link", { name: "Check" }).click();
    await expect(page).toHaveURL(/#\/assertions/);
  });

  test("assertions page - mobile", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/#/assertions");
    await expect(page.getByRole("region", { name: "Unmatched transfers" })).toBeVisible();
    await page.screenshot({ path: "test-results/unmatched-transfers-mobile.png", fullPage: true });
  });
});

test("no unmatched transfers renders nothing", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/assertions");
  await expect(page.getByRole("region", { name: "Accounts" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Unmatched transfers" })).toHaveCount(0);
  await page.goto("/#/");
  await expect(page.getByText("missing the other side")).toHaveCount(0);
});
