import { test, expect, settle, balanceOf, pickAccount } from "./fixtures.js";

test("add transaction with a blank commodity writes USD", async ({ page, floatd }) => {
  await page.goto("/#/add");
  await page.getByLabel("Date").fill("2026-02-14");
  await page.getByLabel("Description").fill("E2E Flowers");

  await pickAccount(page, "Posting 1 account", "expenses:misc");
  await page.getByLabel("Posting 1 amount").fill("30.00");
  // Posting 1 commodity is left blank: its placeholder says USD.

  await pickAccount(page, "Posting 2 account", "liabilities:credit-card");

  await page.getByRole("button", { name: "Add transaction" }).click();
  await expect(page.getByRole("status").getByText("Transaction added")).toBeVisible();

  const journal = floatd.readFile("2026/02.journal");
  expect(journal).toMatch(/2026-02-14 .*E2E Flowers\n(?:.*\n)*?\s+expenses:misc\s+30\.00 USD\n/);
  // One commodity only: a bare "30.00" would show up as a second amount.
  expect(await balanceOf(floatd, "expenses:misc")).toEqual(["72.00 USD"]);

  await page.waitForURL(/#\/transactions/);
  await settle(page);
  await expect(page.getByText("E2E Flowers").first()).toBeVisible();
});

function rowFor(page, text) {
  return page.locator("tbody tr[data-slot=transaction-row]").filter({ hasText: text }).first();
}

// transactionBlock returns the journal entry whose header line contains
// `text` (through to the next blank line), or null.
function transactionBlock(journal, text) {
  return journal.split(/\n\s*\n/).find((b) => b.split("\n")[0].includes(text)) ?? null;
}

test("inline description edit is saved and keeps the transaction's code", async ({ page, floatd }) => {
  const before = transactionBlock(floatd.readFile("2026/02.journal"), "Mystery Merchant");
  const code = before.match(/\(([0-9a-f]{8})\)/)[1];

  await page.goto("/#/transactions");
  await rowFor(page, "Mystery Merchant").getByTitle("Click to edit description").click();
  const input = page.locator("tbody input:not([type=checkbox])").first();
  await input.fill("Mystery Merchant | parking");
  await input.press("Enter");
  await expect(rowFor(page, "parking")).toBeVisible();

  const after = transactionBlock(floatd.readFile("2026/02.journal"), `(${code})`);
  expect(after).toContain("Mystery Merchant | parking");
  expect(after).toMatch(/expenses:misc\s+42\.00 USD/);
});

test("detail editor changes an amount and hledger rebalances", async ({ page, floatd }) => {
  await page.goto("/#/transactions");
  await rowFor(page, "Latte").getByRole("button", { name: /^Edit / }).click();
  const form = page.locator("tbody form");
  await form.getByLabel("Posting 1 amount").fill("6.25");
  // The editor shows hledger's inferred -5.75 on the other side; blank it so
  // hledger balances it again.
  await form.getByLabel("Posting 2 amount").fill("");
  await form.getByRole("button", { name: "Save" }).click();
  await expect(form).toHaveCount(0);

  const block = transactionBlock(floatd.readFile("2026/02.journal"), "Latte");
  expect(block).toMatch(/expenses:food:restaurants\s+6\.25 USD/);
  expect(await balanceOf(floatd, "expenses:food:restaurants")).toEqual(["6.25 USD"]);
  // -500 opening, -123.45 groceries, -6.25 latte, -42 misc
  expect(await balanceOf(floatd, "liabilities:credit-card")).toEqual(["-671.70 USD"]);
});

test("marking a transaction reviewed clears it from Needs review", async ({ page, floatd }) => {
  await page.goto("/#/transactions");
  const mark = rowFor(page, "Mystery Merchant").locator("button[data-slot=status-mark]");
  await expect(mark).toHaveAccessibleName("Unreviewed — click to mark reviewed");
  await mark.click();
  await expect(mark).toHaveAccessibleName("Reviewed — click to mark pending");

  expect(transactionBlock(floatd.readFile("2026/02.journal"), "Mystery Merchant")).toMatch(/^2026-02-20 \* /);
  await expect(page.getByRole("button", { name: /Needs review · 1/ })).toBeVisible();
});

test("deleting a transaction removes it from the journal", async ({ page, floatd }) => {
  await page.goto("/#/transactions");
  await rowFor(page, "Latte").getByRole("button", { name: /^Edit / }).click();
  await page.locator("tbody form").getByRole("button", { name: "Delete" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Delete transaction?")).toBeVisible();
  await dialog.getByRole("button", { name: "Delete" }).click();
  await expect(rowFor(page, "Latte")).toHaveCount(0);

  expect(floatd.readFile("2026/02.journal")).not.toContain("Latte");
  expect(await balanceOf(floatd, "expenses:food:restaurants")).toEqual([]);
});

test("a write is snapshotted and can be restored from Snapshots", async ({ page, floatd }) => {
  await floatd.rpc("DeleteTransaction", {
    fid: transactionBlock(floatd.readFile("2026/02.journal"), "Latte").match(/\(([0-9a-f]{8})\)/)[1],
  });
  expect(floatd.readFile("2026/02.journal")).not.toContain("Latte");

  await page.goto("/#/snapshots");
  await settle(page);
  // Restore the snapshot taken before the delete (startup migrations).
  const row = page.locator("tr[data-slot=snapshot-row]").filter({ hasText: "startup migrations" }).first();
  page.once("dialog", (d) => d.accept()); // native confirm()
  await row.hover();
  await row.getByRole("button", { name: "Restore" }).click();

  await expect.poll(() => floatd.readFile("2026/02.journal")).toContain("Latte");
  expect(await balanceOf(floatd, "expenses:food:restaurants")).toEqual(["5.75 USD"]);
});
