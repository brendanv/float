import { test, expect } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

// Records the request bodies of one RPC (mocked by mockLedgerApi).
function captureRpc(page, method) {
  const bodies = [];
  page.on("request", (req) => {
    if (req.url().endsWith(`/float.v1.LedgerService/${method}`)) {
      bodies.push(JSON.parse(req.postData() || "{}"));
    }
  });
  return bodies;
}

function accountRow(page, name) {
  return page.locator('[data-slot="linked-account"]').filter({ hasText: name });
}

test("linked account shows its import start date", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/connections");
  await expect(accountRow(page, "Bank of America").getByRole("button", { name: "Imports from 2026-04-01" })).toBeVisible();
});

test("sets an import start date on a linked account", async ({ page }) => {
  await mockLedgerApi(page);
  const calls = captureRpc(page, "SetStripeAccountImportFrom");
  await page.goto("/#/connections");

  const row = accountRow(page, "Chase Checking");
  await row.hover();
  await row.getByRole("button", { name: "Set import start date" }).click();

  const dialog = page.getByRole("dialog", { name: "Import start date" });
  await dialog.getByLabel("Import from").fill("2026-11-01");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(dialog).toBeHidden();

  expect(calls).toEqual([{ stripeAccountId: "fca_chase_abc123", importFrom: "2026-11-01" }]);
});

test("clears an import start date", async ({ page }) => {
  await mockLedgerApi(page);
  const calls = captureRpc(page, "SetStripeAccountImportFrom");
  await page.goto("/#/connections");

  await accountRow(page, "Bank of America").getByRole("button", { name: "Imports from 2026-04-01" }).click();
  const dialog = page.getByRole("dialog", { name: "Import start date" });
  await expect(dialog.getByLabel("Import from")).toHaveValue("2026-04-01");
  await dialog.getByRole("button", { name: "Clear" }).click();
  await expect(dialog).toBeHidden();

  expect(calls).toEqual([{ stripeAccountId: "fca_bofa_xyz789" }]);
});

test("fetch results name the import start date", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/connections");
  const row = accountRow(page, "Bank of America");
  await row.getByRole("button", { name: "Fetch Transactions" }).click();
  await expect(row.getByText(/new transactions? since 2026-04-01/)).toBeVisible();
});
