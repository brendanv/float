import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

const unknownTransactions = [
  {
    fid: "u1b2c3d4",
    date: "2026-03-25",
    description: "Amazon | desk accessories",
    payee: "Amazon",
    note: "desk accessories",
    status: "Pending",
    postings: [
      { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "34.99" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-34.99" }] },
    ],
    tags: {},
  },
  {
    fid: "u2b2c3d5",
    date: "2026-03-24",
    description: "MONTHLY GAS BILL",
    status: "Pending",
    postings: [
      { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "84.00" }] },
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-84.00" }] },
    ],
    tags: {},
  },
  {
    fid: "u3b2c3d6",
    date: "2026-03-23",
    description: "Spotify",
    status: "Pending",
    postings: [
      { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "10.99" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-10.99" }] },
    ],
    tags: {},
  },
];

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page, { transactions: unknownTransactions });
});

test("bulk update unknown account - toolbar button visible", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);

  // Select all three unknown-account transactions
  const checkboxes = await page.locator("tbody [data-slot='checkbox']").all();
  for (const cb of checkboxes) {
    await cb.click();
  }
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/bulk-unknown-account-toolbar.png", fullPage: true });
});

test("bulk update unknown account - account combobox open", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);

  // Select all unknown transactions
  const checkboxes = await page.locator("tbody [data-slot='checkbox']").all();
  for (const cb of checkboxes) {
    await cb.click();
  }
  await page.waitForTimeout(200);

  // Click the "Update unknown account" button
  await page.click("button:has-text('Update unknown account')");
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/bulk-unknown-account-combobox.png", fullPage: true });
});

test("bulk update unknown account - combobox with dropdown open", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);

  const checkboxes = await page.locator("tbody [data-slot='checkbox']").all();
  for (const cb of checkboxes) {
    await cb.click();
  }
  await page.waitForTimeout(200);
  await page.click("button:has-text('Update unknown account')");
  await page.waitForTimeout(200);

  // Open the account dropdown — target the combobox inside the bulk action bar (bg-muted)
  await page.locator("div.bg-muted button[role='combobox']").click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/bulk-unknown-account-dropdown.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 520 } });
});
