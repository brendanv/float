// Unmatched-transfer check against a real floatd: one side of a transfer
// through an equity:transfers route is flagged, with the account its other
// side was expected on, until that side is imported.
import { test, expect, settle, balanceOf } from "./fixtures.js";

const ROUTE = "equity:transfers:Checking-Card";

function transferSide(date, description, account, quantity, route = ROUTE) {
  return {
    date,
    description,
    postings: [
      { account, commodity: "USD", quantity },
      { account: route },
    ],
  };
}

test("a lone transfer side is flagged until its other side is imported", async ({ page, floatd }) => {
  // History: one complete card payment, so the route knows its accounts.
  // Then the card side of a second payment arrives without the checking side,
  // plus a same-amount checking transfer through another route that must not
  // be taken as its match. floatd's clock is the real date, long after these,
  // so lone sides are "missing", not "recent".
  await floatd.rpc("ImportStructuredTransactions", {
    transactions: [
      transferSide("2026-01-20", "ONLINE PMT CREDIT CARD", "assets:checking", "-150.00"),
      transferSide("2026-01-21", "PAYMENT THANK YOU", "liabilities:credit-card", "150.00"),
      transferSide("2026-02-12", "PAYMENT THANK YOU", "liabilities:credit-card", "300.00"),
      transferSide("2026-02-12", "TRANSFER TO SAVINGS", "assets:checking", "-300.00", "equity:transfers:Checking-Savings"),
    ],
  });
  const first = await floatd.rpc("FindUnmatchedTransfers", {});
  expect(first.account).toBe("equity:transfers");
  // Newest first; same-day sides in reverse journal order.
  expect(first.transfers.map((t) => [t.transaction.description, t.status, t.route, t.expectedAccounts ?? []])).toEqual([
    ["TRANSFER TO SAVINGS", "missing", "equity:transfers:Checking-Savings", []],
    ["PAYMENT THANK YOU", "missing", ROUTE, ["assets:checking"]],
  ]);
  expect(await balanceOf(floatd, ROUTE)).toEqual(["-300.00 USD"]);

  await page.goto("/#/");
  await settle(page);
  const notice = page.getByRole("status").filter({ hasText: "missing the other side" });
  await expect(notice).toContainText("2 transfers are");
  await notice.getByRole("link", { name: "Check", exact: true }).click();
  await settle(page);
  const section = page.getByRole("region", { name: "Unmatched transfers" });
  await expect(section.locator("[data-slot=transfer-route]").filter({ hasText: "Checking-Card" })).toBeVisible();
  const row = section.locator("[data-slot=unmatched-transfer-row]").filter({ hasText: "PAYMENT THANK YOU" });
  await expect(row).toContainText("Missing");
  await expect(row).toContainText("Expected on assets:checking");

  // The checking sides arrive: each route nets to zero and the flags clear.
  await floatd.rpc("ImportStructuredTransactions", {
    transactions: [
      transferSide("2026-02-10", "ONLINE PMT CREDIT CARD", "assets:checking", "-300.00"),
      transferSide("2026-02-13", "TRANSFER FROM CHECKING", "assets:savings", "300.00", "equity:transfers:Checking-Savings"),
    ],
  });
  expect((await floatd.rpc("FindUnmatchedTransfers", {})).transfers ?? []).toEqual([]);
  expect(await balanceOf(floatd, ROUTE)).toEqual([]);

  await page.reload();
  await settle(page);
  await expect(page.getByRole("region", { name: "Accounts" })).toBeVisible();
  await expect(section).toHaveCount(0);
});
