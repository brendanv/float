// Multi-step flows against a real floatd: login, CSV import, categorization
// rules and account rename. Each asserts on the journal files / RPC results,
// not just on what the UI displays.
import { test, expect, settle, balanceOf, pickAccount, PASSPHRASE, filesDir } from "./fixtures.js";
import path from "node:path";

test.describe("login", () => {
  test.use({ loggedIn: false });
  // The app's first RPC before login is rejected (that's what sends it to
  // #/login), and a wrong passphrase is a 401 on /api/login.
  test.use({ allowedErrors: [[/unauthenticated/, /status of 401/]] });

  test("redirects to login, rejects a wrong passphrase, then signs in", async ({ page }) => {
    await page.goto("/#/accounts");
    await expect(page).toHaveURL(/#\/login/);

    await page.getByLabel("Passphrase").fill("wrong");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("alert")).toBeVisible();

    await page.getByLabel("Passphrase").fill(PASSPHRASE);
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page).not.toHaveURL(/#\/login/);
    await expect(page.getByText(/Net worth/i).first()).toBeVisible();
  });
});

test("CSV import writes a batch that shows up in history and dedupes on re-import", async ({ page, floatd }) => {
  const csv = path.join(filesDir, "checking-2026-03.csv");
  await page.goto("/#/import");
  await page.getByLabel("Bank profile", { exact: true }).selectOption("E2E Checking");
  await page.locator('input[type="file"]').setInputFiles(csv);
  await page.getByRole("button", { name: "Preview" }).click();
  await page.getByRole("button", { name: "Import 3 selected" }).click();
  await expect(page.getByText("Imported 3 transactions")).toBeVisible();

  const march = floatd.readFile("2026/03.journal");
  expect(march).toMatch(/PAYROLL DIRECT DEPOSIT\n(?:.*\n)*?\s+assets:checking\s+3000\.00 USD/);
  expect(march).toMatch(/RENT PAYMENT\n(?:.*\n)*?\s+expenses:housing:rent\s+1500\.00 USD/);
  expect(floatd.readFile("main.journal")).toContain("include 2026/03.journal");
  // 6135.90 + 3000 - 1500 - 87.65
  expect(await balanceOf(floatd, "assets:checking")).toEqual(["7548.25 USD"]);

  await page.goto("/#/imports");
  await settle(page);
  const batch = page.locator("tbody tr").filter({ hasText: "e2e-checking/" });
  await expect(batch).toHaveCount(1);
  await expect(batch).toContainText("CSV");
  await expect(batch.getByRole("button", { name: /View file/ })).toBeVisible();
  // The batch opens as a filtered transaction list.
  await batch.getByRole("link").click();
  await expect(page).toHaveURL(/importBatchId=/);
  await settle(page);
  await expect(page.locator("tbody tr[data-slot=transaction-row]")).toHaveCount(3);

  // The same file again: every row is already on disk.
  await page.goto("/#/import");
  await page.getByLabel("Bank profile", { exact: true }).selectOption("E2E Checking");
  await page.locator('input[type="file"]').setInputFiles(csv);
  await page.getByRole("button", { name: "Preview" }).click();
  await settle(page);
  await expect(page.getByRole("button", { name: /Import \d+ selected/ })).toHaveText("Import 0 selected");
  expect(await balanceOf(floatd, "assets:checking")).toEqual(["7548.25 USD"]);
});

test("a categorization rule is created and applied to existing transactions", async ({ page, floatd }) => {
  await page.goto("/#/rules");
  await page.getByLabel("Pattern (regex)").fill("CORNER GROCER");
  await page.getByLabel("Set payee").fill("Corner Grocer");
  await pickAccount(page, "Set category account", "expenses:food:groceries");
  await page.getByRole("button", { name: "Add rule" }).click();
  await expect(page.locator("tbody").getByText("CORNER GROCER").first()).toBeVisible();
  expect(JSON.parse(floatd.readFile("rules.json"))).toEqual(
    expect.arrayContaining([expect.objectContaining({ pattern: "CORNER GROCER", account: "expenses:food:groceries" })]),
  );

  await page.getByRole("button", { name: "Preview Changes" }).click();
  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText("CORNER GROCER #1234")).toBeVisible();
  await drawer.getByRole("button", { name: /Apply to 1 Transaction/ }).click();
  await expect(drawer).toHaveCount(0);

  const feb = floatd.readFile("2026/02.journal");
  expect(feb).toMatch(/Corner Grocer \| .*\n(?:.*\n)*?\s+expenses:food:groceries\s+64\.10 USD/);
  expect(await balanceOf(floatd, "expenses:unknown")).toEqual([]);
  expect(await balanceOf(floatd, "expenses:food:groceries")).toEqual(["187.55 USD"]);
});

test("renaming an account rewrites declarations and postings", async ({ page, floatd }) => {
  await page.goto("/#/accounts");
  await settle(page);
  await page.getByRole("button", { name: "Rename expenses:misc" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("New name").fill("expenses:other");
  await dialog.getByRole("button", { name: "Continue" }).click();
  await dialog.getByRole("button", { name: "Confirm Rename" }).click();
  await expect(dialog).toHaveCount(0);

  expect(floatd.readFile("accounts.journal")).toMatch(/^account expenses:other$/m);
  expect(floatd.readFile("accounts.journal")).not.toMatch(/^account expenses:misc$/m);
  expect(floatd.readFile("2026/02.journal")).toMatch(/expenses:other\s+42\.00 USD/);
  expect(await balanceOf(floatd, "expenses:other")).toEqual(["42.00 USD"]);
  await expect(page.getByText("other", { exact: true }).first()).toBeVisible();
});
