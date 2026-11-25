// Snapshots page restore dialog: what the UI sends for a forward restore and
// for the hard-reset option, and that the newest snapshot can't be restored.
import { test, expect } from "./fixtures.js";
import { mockLedgerApi, mockSnapshots } from "./mock-api.js";

function captureRpc(page, method) {
  const bodies = [];
  page.on("request", (req) => {
    if (req.url().endsWith(`/float.v1.LedgerService/${method}`)) {
      bodies.push(JSON.parse(req.postData() || "{}"));
    }
  });
  return bodies;
}

function row(page, i) {
  return page.locator("tr[data-slot=snapshot-row]").nth(i);
}

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/snapshots");
  await expect(row(page, 0)).toBeVisible();
});

test("the newest snapshot is marked current instead of restorable", async ({ page }) => {
  await expect(row(page, 0).getByRole("button", { name: "Restore" })).toHaveCount(0);
  await expect(row(page, 0).getByText("Current")).toBeVisible();
});

test("restore sends a forward restore by default", async ({ page }) => {
  const restores = captureRpc(page, "RestoreSnapshot");
  await row(page, 2).hover();
  await row(page, 2).getByRole("button", { name: "Restore" }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Restore this snapshot?")).toBeVisible();
  await expect(dialog).toContainText(mockSnapshots[2].message);
  await expect(dialog).toContainText("deleting the 2 newer snapshots");
  await dialog.getByRole("button", { name: "Restore" }).click();

  await expect(dialog).toHaveCount(0);
  expect(restores).toEqual([{ hash: mockSnapshots[2].hash }]);
});

test("discard later history sends a hard reset", async ({ page }) => {
  const restores = captureRpc(page, "RestoreSnapshot");
  await row(page, 1).hover();
  await row(page, 1).getByRole("button", { name: "Restore" }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("deleting the 1 newer snapshot.");
  await dialog.getByRole("checkbox").click();
  await dialog.getByRole("button", { name: "Hard reset" }).click();

  await expect(dialog).toHaveCount(0);
  expect(restores).toEqual([{ hash: mockSnapshots[1].hash, hardReset: true }]);
});

test("cancel closes without restoring and resets the option", async ({ page }) => {
  const restores = captureRpc(page, "RestoreSnapshot");
  await row(page, 1).hover();
  await row(page, 1).getByRole("button", { name: "Restore" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("checkbox").click();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);

  await row(page, 1).hover();
  await row(page, 1).getByRole("button", { name: "Restore" }).click();
  await expect(dialog.getByRole("checkbox")).not.toBeChecked();
  await expect(dialog.getByRole("button", { name: "Restore" })).toBeVisible();
  expect(restores).toEqual([]);
});
