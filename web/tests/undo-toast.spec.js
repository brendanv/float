// Undo toast: a mutation floatd marks as undoable (float-snapshot trailer)
// offers Undo, which calls UndoSnapshot with that snapshot and then offers
// Redo on the snapshot the undo created. Deletes no longer ask to confirm.
import { test, expect } from "./fixtures.js";
import { mockLedgerApi, mockSnapshotHash } from "./mock-api.js";

function usd(quantity) {
  return [{ commodity: "USD", quantity }];
}

const latte = {
  fid: "1a77e001",
  date: "2026-03-20",
  description: "Cafe | latte",
  payee: "Cafe",
  note: "latte",
  status: "",
  postings: [
    { account: "expenses:dining", amounts: usd("5.00") },
    { account: "assets:checking", amounts: usd("-5.00") },
  ],
  tags: {},
};

function captureRpc(page, method) {
  const bodies = [];
  page.on("request", (req) => {
    if (req.url().endsWith(`/float.v1.LedgerService/${method}`)) {
      bodies.push(JSON.parse(req.postData() || "{}"));
    }
  });
  return bodies;
}

function toast(page) {
  return page.locator("[data-slot=undo-toast]");
}

async function deleteLatte(page) {
  await page.goto("/#/transactions");
  const row = page.locator("tbody tr").filter({ hasText: "Cafe" }).first();
  await row.getByRole("button", { name: /^Edit / }).click();
  await page.locator("tbody form").getByRole("button", { name: "Delete" }).click();
}

test("deleting a transaction needs no confirmation and offers undo, then redo", async ({ page }) => {
  await mockLedgerApi(page, { transactions: [latte] });
  const deletes = captureRpc(page, "DeleteTransaction");
  const undos = captureRpc(page, "UndoSnapshot");

  await deleteLatte(page);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect.poll(() => deletes).toEqual([{ fid: latte.fid }]);

  await expect(toast(page)).toContainText("Transaction deleted");
  await toast(page).getByRole("button", { name: "Undo" }).click();
  await expect.poll(() => undos).toEqual([{ hash: mockSnapshotHash(1) }]);

  await expect(toast(page)).toContainText("Change undone");
  await toast(page).getByRole("button", { name: "Redo" }).click();
  // Redo undoes the snapshot the undo created.
  await expect.poll(() => undos.length).toBe(2);
  expect(undos[1]).toEqual({ hash: mockSnapshotHash(2) });
  await expect(toast(page)).toContainText("Transaction deleted");
  await expect(toast(page).getByRole("button", { name: "Undo" })).toBeVisible();
});

test("a newer undoable change replaces the toast", async ({ page }) => {
  await mockLedgerApi(page, { transactions: [latte] });
  await deleteLatte(page);
  await expect(toast(page)).toContainText("Transaction deleted");

  await page.goto("/#/rules");
  const row = page.locator("tbody tr").filter({ has: page.getByRole("button", { name: "Delete" }) }).first();
  await row.hover();
  await row.getByRole("button", { name: "Delete" }).click();
  await expect(toast(page)).toHaveCount(1);
  await expect(toast(page)).toContainText("Rule deleted");
});

test.describe("stale undo", () => {
  test.use({ allowedErrors: [/status of 400/] });

  test("a failed undo explains and links to Snapshots", async ({ page }) => {
    await mockLedgerApi(page, { transactions: [latte] });
    await page.route("**/float.v1.LedgerService/UndoSnapshot", (route) =>
      route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({
          code: "failed_precondition",
          message: "other changes have been saved since, so this can no longer be undone",
        }),
      }),
    );
    await deleteLatte(page);
    await toast(page).getByRole("button", { name: "Undo" }).click();
    await expect(toast(page)).toContainText("Couldn’t undo: other changes have been saved since");
    await toast(page).getByRole("button", { name: "Snapshots" }).click();
    await expect(page).toHaveURL(/#\/snapshots$/);
  });
});

test("bulk delete still confirms, then offers undo", async ({ page }) => {
  await mockLedgerApi(page, { transactions: [latte] });
  await page.goto("/#/transactions");
  await page.locator("tbody tr").filter({ hasText: "Cafe" }).first().getByRole("checkbox").click();
  await page.locator("[data-slot=bulk-action-bar]").getByRole("button", { name: "Delete" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("You can undo it right after.");
  await dialog.getByRole("button", { name: "Delete" }).click();
  await expect(toast(page)).toContainText("Transactions deleted");
});

test("deleting a template needs no confirmation", async ({ page }) => {
  await mockLedgerApi(page);
  const deletes = captureRpc(page, "DeleteTemplate");
  await page.goto("/#/templates");
  await page.getByRole("button", { name: /^Delete template / }).first().click({ force: true });
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect.poll(() => deletes.length).toBe(1);
  await expect(toast(page)).toContainText("Template deleted");
});

test.describe("phone", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  test("the toast sits above the tab bar with a 40px undo target", async ({ page }) => {
    await mockLedgerApi(page);
    await page.goto("/#/templates");
    await page.getByRole("button", { name: /^Delete template / }).first().click();
    const t = toast(page);
    await expect(t).toContainText("Template deleted");
    const tabBar = await page.locator("[data-slot=mobile-tab-bar]").boundingBox();
    // Poll: the toast slides in from below.
    await expect.poll(async () => {
      const box = await t.boundingBox();
      return box.y + box.height;
    }).toBeLessThanOrEqual(tabBar.y);
    const undo = await t.getByRole("button", { name: "Undo" }).boundingBox();
    expect(undo.height).toBeGreaterThanOrEqual(40);
  });
});
