import { expect, test } from "./fixtures.js";
import { mockLedgerApi, mockUnmatchedTransfers } from "./mock-api.js";

test.describe("with unmatched transfers", () => {
  test.beforeEach(async ({ page }) => {
    await page.clock.setFixedTime(new Date("2026-04-20T12:00:00"));
    await mockLedgerApi(page, { unmatchedTransfers: mockUnmatchedTransfers });
  });

  test("assertions page lists unmatched transfers by route", async ({ page }) => {
    await page.goto("/#/assertions");
    const section = page.getByRole("region", { name: "Unmatched transfers" });
    await expect(section).toBeVisible();
    await expect(section).toContainText("equity:transfers");

    // Route headers in order of their newest side.
    const routes = section.locator("[data-slot=transfer-route]");
    await expect(routes).toHaveCount(3);
    await expect(routes.nth(0)).toContainText("Checking-Savings");
    await expect(routes.nth(1)).toContainText("Checking-Cards");
    await expect(routes.nth(2)).toContainText("Direct");

    const rows = section.locator("[data-slot=unmatched-transfer-row]");
    await expect(rows).toHaveCount(4);
    await expect(rows.nth(0)).toContainText("Recent");
    await expect(rows.nth(1)).toContainText("Missing");
    await expect(rows.nth(1)).toContainText("Expected on assets:checking");
    await expect(rows.nth(2)).toContainText("Expected on liabilities:creditcard or liabilities:travelcard");
    await expect(rows.nth(3).locator("[data-slot=expected-accounts]")).toHaveCount(0);

    await page.screenshot({ path: "test-results/unmatched-transfers.png", fullPage: true });

    await rows.nth(1).getByRole("link", { name: "Payment thank you" }).click();
    await expect(page).toHaveURL(/#\/transactions\?search=code%3Af1e2d3c5/);
  });

  test("assertions page hides transfers older than six months until show all", async ({ page }) => {
    await page.clock.setFixedTime(new Date("2026-08-10T12:00:00"));
    await page.goto("/#/assertions");
    const section = page.getByRole("region", { name: "Unmatched transfers" });
    const rows = section.locator("[data-slot=unmatched-transfer-row]");
    await expect(rows).toHaveCount(3);
    await expect(section.locator("[data-slot=transfer-route]").filter({ hasText: "Direct" })).toHaveCount(0);

    await section.getByRole("button", { name: "Show all (1 older)" }).click();
    await expect(rows).toHaveCount(4);
    await expect(rows.nth(3)).toContainText("Transfer to Venmo");

    await section.getByRole("button", { name: "Last 6 months" }).click();
    await expect(rows).toHaveCount(3);
  });

  test("assertions page offers show all when every transfer is older than six months", async ({ page }) => {
    await page.clock.setFixedTime(new Date("2026-11-20T12:00:00"));
    await page.goto("/#/assertions");
    const section = page.getByRole("region", { name: "Unmatched transfers" });
    await expect(section).toContainText("No unmatched transfers in the last 6 months");
    await section.getByRole("button", { name: "Show all (4 older)" }).click();
    await expect(section.locator("[data-slot=unmatched-transfer-row]")).toHaveCount(4);
  });

  test("home notice only counts missing transfers from the last six months", async ({ page }) => {
    await page.clock.setFixedTime(new Date("2026-08-10T12:00:00"));
    await page.goto("/#/");
    const notice = page.getByRole("status").filter({ hasText: "missing the other side" });
    await expect(notice).toContainText("2 transfers are");
    await page.clock.setFixedTime(new Date("2026-11-20T12:00:00"));
    await page.reload();
    await expect(page.getByText("missing the other side")).toHaveCount(0);
  });

  test("home shows a notice for missing transfers only", async ({ page }) => {
    await page.goto("/#/");
    const notice = page.getByRole("status").filter({ hasText: "missing the other side" });
    await expect(notice).toContainText("3 transfers are");
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
