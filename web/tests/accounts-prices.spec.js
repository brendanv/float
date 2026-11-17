import { test, expect } from "./fixtures.js";
import { mockFullAccountTreeRows, mockLedgerApi } from "./mock-api.js";

function captureRpc(page, method) {
  const bodies = [];
  page.on("request", (req) => {
    if (req.url().endsWith(`/float.v1.LedgerService/${method}`)) bodies.push(JSON.parse(req.postData() || "{}"));
  });
  return bodies;
}

// An account holding a commodity hledger can't value keeps one amount per lot.
const lotRows = [
  ...mockFullAccountTreeRows,
  {
    fullName: "assets:brokerage",
    displayName: "brokerage",
    indent: 1,
    amounts: [
      { commodity: "USD", quantity: "5210.00" },
      ...[100, 110, 120, 130].map((cost, i) => ({
        commodity: "VTI",
        quantity: String(i + 1),
        costBasis: { cost: { commodity: "USD", quantity: String(cost) } },
      })),
    ],
  },
];

test("accounts tree shows market value and collapses long amount lists", async ({ page }) => {
  await mockLedgerApi(page, { balanceTreeRows: lotRows });
  const balances = captureRpc(page, "GetBalances");
  await page.goto("/#/accounts");

  const row = page.locator('[data-slot="account-row"]').filter({ hasText: "brokerage" });
  await expect(row.locator('[data-slot="amount"]')).toHaveCount(1);
  await expect.poll(() => balances.find((b) => b.tree)?.value).toBe("now,USD");

  await row.getByRole("button", { name: "Show 4 more amounts for assets:brokerage" }).click();
  await expect(row.locator('[data-slot="amount"]')).toHaveCount(5);
  await page.screenshot({ path: "test-results/accounts-collapsed-lots.png", fullPage: true });
  await row.getByRole("button", { name: "Show fewer amounts for assets:brokerage" }).click();
  await expect(row.locator('[data-slot="amount"]')).toHaveCount(1);
});

test("prices page lists stale commodities first", async ({ page }) => {
  await page.clock.setFixedTime(new Date("2026-03-20T12:00:00"));
  await mockLedgerApi(page, {
    prices: [
      { pid: "p0000001", date: "2026-03-19", commodity: "AAPL", price: { commodity: "USD", quantity: "181.10" } },
      { pid: "p0000002", date: "2026-01-02", commodity: "AAPL", price: { commodity: "USD", quantity: "170.00" } },
      { pid: "p0000003", date: "2026-02-03", commodity: "VTI", price: { commodity: "USD", quantity: "290.00" } },
      { pid: "p0000004", date: "2025-11-01", commodity: "BTC", price: { commodity: "USD", quantity: "68000" } },
      { pid: "p0000005", date: "2026-03-17", commodity: "MSFT", price: { commodity: "USD", quantity: "402.30" } },
    ],
  });
  await page.goto("/#/prices");

  const rows = page.locator('[data-slot="latest-price"]');
  await expect(rows).toHaveCount(4);
  await expect(rows.locator("td:first-child")).toHaveText(["BTC", "VTI", "MSFT", "AAPL"]);
  await expect(page.locator('[data-slot="latest-price"][data-stale]')).toHaveCount(2);
  await expect(page.getByText("2 of 4 older than 7 days")).toBeVisible();
  await page.screenshot({ path: "test-results/prices-latest.png", fullPage: true });

  await page.getByRole("button", { name: "Backfill stale" }).click();
  await expect(page.locator("#backfill-commodities")).toHaveValue("BTC, VTI");
  await expect(page.locator("#backfill-start")).toHaveValue("2025-11-02");
});
