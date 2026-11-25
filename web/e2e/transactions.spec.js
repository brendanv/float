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

test("deleting a transaction removes it from the journal, and Undo/Redo work", async ({ page, floatd }) => {
  const before = floatd.readFile("2026/02.journal");
  await page.goto("/#/transactions");
  await rowFor(page, "Latte").getByRole("button", { name: /^Edit / }).click();
  // No confirmation: the delete happens at once and can be undone.
  await page.locator("tbody form").getByRole("button", { name: "Delete" }).click();
  await expect(rowFor(page, "Latte")).toHaveCount(0);
  expect(floatd.readFile("2026/02.journal")).not.toContain("Latte");
  expect(await balanceOf(floatd, "expenses:food:restaurants")).toEqual([]);

  const toast = page.locator("[data-slot=undo-toast]");
  await expect(toast).toContainText("Transaction deleted");
  await toast.getByRole("button", { name: "Undo" }).click();
  await expect(toast).toContainText("Change undone");
  await expect(rowFor(page, "Latte")).toHaveCount(1);
  expect(floatd.readFile("2026/02.journal")).toBe(before);
  expect(await balanceOf(floatd, "expenses:food:restaurants")).toEqual(["5.75 USD"]);

  await toast.getByRole("button", { name: "Redo" }).click();
  await expect(toast).toContainText("Transaction deleted");
  await expect(rowFor(page, "Latte")).toHaveCount(0);
  expect(floatd.readFile("2026/02.journal")).not.toContain("Latte");

  const messages = (await floatd.rpc("ListSnapshots", {})).snapshots.map((s) => s.message.trim());
  expect(messages[0]).toMatch(/^redo: delete transaction /);
  expect(messages[1]).toMatch(/^undo: delete transaction /);
  expect(messages[2]).toMatch(/^delete transaction /);
});

test("bulk delete (a streaming RPC) can be undone", async ({ page, floatd }) => {
  const before = floatd.readFile("2026/02.journal");
  await page.goto("/#/transactions");
  await rowFor(page, "Latte").getByRole("checkbox").click();
  await page.locator("[data-slot=bulk-action-bar]").getByRole("button", { name: "Delete" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Delete" }).click();
  await expect(rowFor(page, "Latte")).toHaveCount(0);

  const toast = page.locator("[data-slot=undo-toast]");
  await expect(toast).toContainText("Transactions deleted");
  await toast.getByRole("button", { name: "Undo" }).click();
  await expect(rowFor(page, "Latte")).toHaveCount(1);
  expect(floatd.readFile("2026/02.journal")).toBe(before);
});

test.describe("stale undo", () => {
  test.use({ allowedErrors: [[/failed_precondition/, /status of 400/]] });

  test("undo is refused once another change has been saved", async ({ page, floatd }) => {
    await page.goto("/#/transactions");
    await rowFor(page, "Latte").getByRole("button", { name: /^Edit / }).click();
    await page.locator("tbody form").getByRole("button", { name: "Delete" }).click();
    const toast = page.locator("[data-slot=undo-toast]");
    await expect(toast).toContainText("Transaction deleted");

    // Another write lands (e.g. from another tab) before Undo is clicked.
    const latteGone = floatd.readFile("2026/02.journal");
    const mark = rowFor(page, "Mystery Merchant").locator("button[data-slot=status-mark]");
    await mark.click();
    await expect(mark).toHaveAccessibleName("Reviewed — click to mark pending");
    const afterMark = floatd.readFile("2026/02.journal");
    expect(afterMark).not.toBe(latteGone);

    await toast.getByRole("button", { name: "Undo" }).click();
    await expect(toast).toContainText("Couldn’t undo");
    expect(floatd.readFile("2026/02.journal")).toBe(afterMark);
  });
});

test("a write is snapshotted and can be restored from Snapshots", async ({ page, floatd }) => {
  await floatd.rpc("DeleteTransaction", {
    fid: transactionBlock(floatd.readFile("2026/02.journal"), "Latte").match(/\(([0-9a-f]{8})\)/)[1],
  });
  expect(floatd.readFile("2026/02.journal")).not.toContain("Latte");
  const before = (await floatd.rpc("ListSnapshots", {})).snapshots;

  await page.goto("/#/snapshots");
  await settle(page);
  // Restore the snapshot taken before the delete (startup migrations).
  const row = page.locator("tr[data-slot=snapshot-row]").filter({ hasText: "startup migrations" }).first();
  await row.hover();
  await row.getByRole("button", { name: "Restore" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Restore" }).click();
  await expect(dialog).toHaveCount(0);

  await expect.poll(() => floatd.readFile("2026/02.journal")).toContain("Latte");
  expect(await balanceOf(floatd, "expenses:food:restaurants")).toEqual(["5.75 USD"]);

  // The restore is a new snapshot on top; the delete stays in history.
  const after = (await floatd.rpc("ListSnapshots", {})).snapshots;
  expect(after.map((s) => s.hash).slice(1)).toEqual(before.map((s) => s.hash));
  expect(after[0].message).toMatch(/^restore snapshot [0-9a-f]{8} /);
  await expect(page.locator("tr[data-slot=snapshot-row]").first()).toContainText("restore snapshot");
});

test("a hard reset from Snapshots discards later history", async ({ page, floatd }) => {
  await floatd.rpc("DeleteTransaction", {
    fid: transactionBlock(floatd.readFile("2026/02.journal"), "Latte").match(/\(([0-9a-f]{8})\)/)[1],
  });
  const before = (await floatd.rpc("ListSnapshots", {})).snapshots;
  const target = before.findIndex((s) => s.message.includes("startup migrations"));
  expect(target).toBeGreaterThan(0);

  await page.goto("/#/snapshots");
  await settle(page);
  const row = page.locator("tr[data-slot=snapshot-row]").filter({ hasText: "startup migrations" }).first();
  await row.hover();
  await row.getByRole("button", { name: "Restore" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("checkbox").click();
  await dialog.getByRole("button", { name: "Hard reset" }).click();
  await expect(dialog).toHaveCount(0);

  await expect.poll(() => floatd.readFile("2026/02.journal")).toContain("Latte");
  const after = (await floatd.rpc("ListSnapshots", {})).snapshots;
  expect(after.map((s) => s.hash)).toEqual(before.slice(target).map((s) => s.hash));
  await expect(page.locator("tr[data-slot=snapshot-row]")).toHaveCount(after.length);
});
