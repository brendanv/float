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
const lotRows = mockFullAccountTreeRows.map((r) => (r.fullName !== "assets:investments:aapl" ? r : {
  ...r,
  amounts: [
    { commodity: "USD", quantity: "1750.00" },
    ...[100, 110, 120, 130].map((cost, i) => ({
      commodity: "VTI",
      quantity: String(i + 1),
      costBasis: { cost: { commodity: "USD", quantity: String(cost) } },
    })),
  ],
}));

test("accounts tree shows market value and collapses long amount lists", async ({ page }) => {
  await mockLedgerApi(page, { balanceTreeRows: lotRows });
  const balances = captureRpc(page, "GetBalances");
  await page.goto("/#/accounts");

  const row = page.locator('[data-slot="account-row"]').filter({ hasText: /^aapl/ });
  await expect(row.locator('[data-slot="amount"]')).toHaveCount(1);
  await expect.poll(() => balances.find((b) => b.tree)?.value).toBe("now,USD");

  await row.getByRole("button", { name: "Show 4 more amounts for assets:investments:aapl" }).click();
  await expect(row.locator('[data-slot="amount"]')).toHaveCount(5);
  await page.screenshot({ path: "test-results/accounts-collapsed-lots.png", fullPage: true });
  await row.getByRole("button", { name: "Show fewer amounts for assets:investments:aapl" }).click();
  await expect(row.locator('[data-slot="amount"]')).toHaveCount(1);
});

const holding = (symbol, account = "assets:brokerage") => ({ account, symbol, quantity: "1" });

test("prices page lists stale held commodities first", async ({ page }) => {
  await page.clock.setFixedTime(new Date("2026-03-20T12:00:00"));
  await mockLedgerApi(page, {
    prices: [
      { pid: "p0000001", date: "2026-03-19", commodity: "AAPL", price: { commodity: "USD", quantity: "181.10" } },
      { pid: "p0000002", date: "2026-01-02", commodity: "AAPL", price: { commodity: "USD", quantity: "170.00" } },
      { pid: "p0000003", date: "2026-02-03", commodity: "VTI", price: { commodity: "USD", quantity: "290.00" } },
      { pid: "p0000004", date: "2025-11-01", commodity: "BTC", price: { commodity: "USD", quantity: "68000" } },
      { pid: "p0000005", date: "2026-03-17", commodity: "MSFT", price: { commodity: "USD", quantity: "402.30" } },
      // Sold long ago: old, but not worth updating.
      { pid: "p0000006", date: "2024-06-03", commodity: "GE", price: { commodity: "USD", quantity: "160.00" } },
    ],
    portfolioHoldings: {
      holdings: [
        holding("AAPL"), holding("AAPL", "assets:ira"), holding("VTI"), holding("BTC"), holding("MSFT"),
        // Held but never priced.
        holding("FXAIX", "assets:401k"),
      ],
    },
  });
  await page.goto("/#/prices");

  const rows = page.locator('[data-slot="latest-price"]');
  await expect(rows.locator("td:first-child")).toHaveText(["FXAIX", "BTC", "VTI", "MSFT", "AAPL", /^GE/]);
  await expect(page.locator('[data-slot="latest-price"][data-stale]')).toHaveCount(3);
  await expect(rows.first()).toContainText("no price");
  await expect(rows.last()).toContainText("not held");
  await expect(page.getByText("3 of 5 held need a price")).toBeVisible();
  await page.screenshot({ path: "test-results/prices-latest.png", fullPage: true });

  await page.getByRole("button", { name: "Backfill stale" }).click();
  await expect(page.locator("#backfill-commodities")).toHaveValue("FXAIX, BTC, VTI");
  await expect(page.locator("#backfill-start")).toHaveValue("2025-03-20");
});

test("prices page collapses many commodities to the stale ones first", async ({ page }) => {
  await page.clock.setFixedTime(new Date("2026-03-20T12:00:00"));
  const symbols = Array.from({ length: 24 }, (_, i) => `SYM${String(i).padStart(2, "0")}`);
  await mockLedgerApi(page, {
    // Even-numbered symbols are fresh, odd ones a month old.
    prices: symbols.map((commodity, i) => ({
      pid: `q${String(i).padStart(7, "0")}`,
      date: i % 2 ? "2026-02-15" : "2026-03-19",
      commodity,
      price: { commodity: "USD", quantity: String(10 + i) },
    })),
    portfolioHoldings: { holdings: symbols.map((s) => holding(s)) },
  });
  await page.goto("/#/prices");

  const rows = page.locator('[data-slot="latest-price"]');
  await expect(rows).toHaveCount(10);
  await expect(page.locator('[data-slot="latest-price"][data-stale]')).toHaveCount(10);
  await expect(page.getByText("12 of 24 held need a price")).toBeVisible();
  await page.screenshot({ path: "test-results/prices-latest-many.png", fullPage: true });

  await page.getByRole("button", { name: "Show all 24 commodities" }).click();
  await expect(rows).toHaveCount(24);
  await expect(page.locator('[data-slot="latest-price"][data-stale]')).toHaveCount(12);
  await page.getByRole("button", { name: "Show fewer" }).click();
  await expect(rows).toHaveCount(10);
});
