// Unmatched-transfer check against a real floatd: one side of a transfer
// through equity:transfers is flagged until the other side is imported.
import { test, expect, settle, balanceOf } from "./fixtures.js";

function transferSide(date, description, account, quantity) {
  return {
    date,
    description,
    postings: [
      { account, commodity: "USD", quantity },
      { account: "equity:transfers" },
    ],
  };
}

test("a lone transfer side is flagged until its other side is imported", async ({ page, floatd }) => {
  // The checking side of a card payment arrives first. floatd's clock is
  // the real date, long after this, so the side is "missing", not "recent".
  await floatd.rpc("ImportStructuredTransactions", {
    transactions: [transferSide("2026-02-10", "ONLINE PMT CREDIT CARD", "assets:checking", "-300.00")],
  });
  const first = await floatd.rpc("FindUnmatchedTransfers", {});
  expect(first.account).toBe("equity:transfers");
  expect(first.transfers.map((t) => [t.transaction.description, t.status])).toEqual([
    ["ONLINE PMT CREDIT CARD", "missing"],
  ]);
  expect(await balanceOf(floatd, "equity:transfers")).toEqual(["300.00 USD"]);

  await page.goto("/#/");
  await settle(page);
  const notice = page.getByRole("status").filter({ hasText: "missing the other side" });
  await expect(notice).toContainText("1 transfer is");
  await notice.getByRole("link", { name: "Check", exact: true }).click();
  await settle(page);
  const section = page.getByRole("region", { name: "Unmatched transfers" });
  const row = section.locator("[data-slot=unmatched-transfer-row]");
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("Missing");
  await expect(row).toContainText("ONLINE PMT CREDIT CARD");

  // The card side arrives two days later: the pair nets the clearing account
  // to zero and the flag clears.
  await floatd.rpc("ImportStructuredTransactions", {
    transactions: [transferSide("2026-02-12", "PAYMENT THANK YOU", "liabilities:credit-card", "300.00")],
  });
  expect((await floatd.rpc("FindUnmatchedTransfers", {})).transfers ?? []).toEqual([]);
  expect(await balanceOf(floatd, "equity:transfers")).toEqual([]);

  await page.reload();
  await settle(page);
  await expect(page.getByRole("region", { name: "Accounts" })).toBeVisible();
  await expect(section).toHaveCount(0);
});
