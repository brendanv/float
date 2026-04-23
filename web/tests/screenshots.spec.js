import { test, expect } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("home page", async ({ page }) => {
  await page.goto("/#/");
  // Wait for data to load (balance summary or account list)
  await page.waitForSelector(".balance-summary, article, table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/home.png", fullPage: true });
});

test("transactions page", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions.png", fullPage: true });
});

test("transactions page - delete confirmation", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.click("tbody tr:first-child");
  await page.waitForTimeout(200);
  await page.click("button:has-text('Delete')");
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-delete.png", fullPage: true });
});

test("add transaction page", async ({ page }) => {
  await page.goto("/#/add");
  await page.waitForSelector("form", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/add-transaction.png", fullPage: true });
});

test("add transaction modal", async ({ page }) => {
  await page.goto("/#/");
  await page.waitForTimeout(400);
  await page.click('button:has-text("Add Transaction")');
  await page.waitForSelector('[role="dialog"]', { timeout: 5000 });
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/add-transaction-modal.png", fullPage: true });
});

test("trends page", async ({ page }) => {
  await page.goto("/#/trends");
  await page.waitForSelector(".trends-chart canvas", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(1000);
  await page.screenshot({ path: "test-results/trends.png", fullPage: true });
});

test("prices page", async ({ page }) => {
  await page.goto("/#/prices");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/prices.png", fullPage: true });
});

test("accounts page", async ({ page }) => {
  await page.goto("/#/accounts");
  await page.waitForSelector("h2, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/accounts.png", fullPage: true });
});

test("transactions page - filter dropdown open", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  // Click the quick filter dropdown button (last btn-ghost/btn-primary in the first row)
  const filterBtn = page.locator("button").filter({ hasText: /^(All|Reviewed|Unreviewed|No payee set|Filter)\s*▾?$/ }).first();
  await filterBtn.click();
  await page.waitForTimeout(150);
  await page.screenshot({ path: "test-results/transactions-filter-open.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 300 } });
});

test("transactions page - mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/transactions");
  await page.waitForSelector(".card", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-mobile.png", fullPage: true });
});

test("transactions page - payee filter", async ({ page }) => {
  await page.goto("/#/transactions?payee=Whole+Foods+Market");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-payee-filter.png", fullPage: true });
});

test("transactions page - account register view", async ({ page }) => {
  await page.goto("/#/transactions?account=assets%3Achecking");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-account-register.png", fullPage: true });
});

test("transactions page - account register mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/transactions?account=assets%3Achecking");
  await page.waitForSelector(".card", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-account-register-mobile.png", fullPage: true });
});

test("transactions page - mobile bulk edit toolbar", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/transactions");
  await page.waitForSelector(".card", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  const checkboxes = await page.locator(".card input[type=checkbox]").all();
  for (const cb of checkboxes.slice(0, 3)) {
    await cb.click();
  }
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/transactions-mobile-bulk-edit.png", fullPage: true });
});

test("transactions page - bulk edit toolbar", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  // Check the first three transaction checkboxes
  const checkboxes = await page.locator("tbody input[type=checkbox]").all();
  for (const cb of checkboxes.slice(0, 3)) {
    await cb.click();
  }
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/transactions-bulk-edit.png", fullPage: true });
});

test("import page", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector("select, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import.png", fullPage: true });
});

test("import page - profile selected with edit delete buttons", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector('[data-testid="select-trigger"], button[role="combobox"], [role="combobox"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Open the Select dropdown and pick a profile
  const trigger = page.locator('[role="combobox"]').first();
  await trigger.click();
  await page.waitForTimeout(200);
  await page.locator('[role="option"]').first().click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/import-profile-selected.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 320 } });
});

test("import page - edit profile modal", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector('[role="combobox"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Select a profile
  await page.locator('[role="combobox"]').first().click();
  await page.waitForTimeout(200);
  await page.locator('[role="option"]').first().click();
  await page.waitForTimeout(300);
  // Click edit button
  await page.click('button[title="Edit bank profile"]');
  await page.waitForSelector('[role="dialog"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/import-edit-profile-modal.png", fullPage: true });
});

test("import page - delete profile dialog", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector('[role="combobox"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Select a profile
  await page.locator('[role="combobox"]').first().click();
  await page.waitForTimeout(200);
  await page.locator('[role="option"]').first().click();
  await page.waitForTimeout(300);
  // Use JS click to bypass the file input that overlaps this button at some viewport sizes
  await page.evaluate(() => document.querySelector('button[title="Delete bank profile"]').click());
  await page.waitForSelector('[role="dialog"]', { timeout: 5000 });
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-delete-profile-dialog.png", fullPage: true });
});

test("import page - create profile modal", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector("select", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  // Click the "+" button to open the create profile modal
  await page.click('button[title="Create new bank profile"]');
  await page.waitForSelector("dialog.modal-open", { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/import-create-profile-modal.png", fullPage: true });
});

test("import page - preview loaded", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector("select", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  // Select a profile and attach a fake CSV file
  await page.selectOption("select", { index: 1 });
  await page.evaluate(() => {
    const input = document.querySelector('input[type="file"]');
    if (!input) return;
    const dt = new DataTransfer();
    dt.items.add(new File(["date,amount,description\n2026-03-28,-42.99,AMAZON"], "bank.csv", { type: "text/csv" }));
    Object.defineProperty(input, "files", { value: dt.files });
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await page.waitForTimeout(200);
  await page.click('button[type="submit"]');
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-preview.png", fullPage: true });
});

test("import history page", async ({ page }) => {
  await page.goto("/#/imports");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-history.png", fullPage: true });
});

test("import detail page", async ({ page }) => {
  await page.goto("/#/imports/2026-03-28-a1b2c3d4");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-detail.png", fullPage: true });
});

test("rules page", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/rules.png", fullPage: true });
});

test("rules page - account typeahead", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Focus the account input to trigger the on-focus suggestions
  await page.focus('input[placeholder="expenses:shopping"]');
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/rules-account-typeahead.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 320 } });
});

test("rules page - account typeahead filtered", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.fill('input[placeholder="expenses:shopping"]', "exp");
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/rules-account-typeahead-filtered.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 320 } });
});

test("rules page - apply preview", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Click "Preview Changes" button
  await page.click('button:has-text("Preview Changes")');
  await page.waitForSelector("tbody tr", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/rules-apply-preview.png", fullPage: true });
});

test("rules page - apply preview section zoomed", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 1800 });
  await page.goto("/#/rules");
  // Wait for rule list table to load
  await page.waitForSelector("table tbody tr", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  // Click Preview Changes
  await page.click('button:has-text("Preview Changes")');
  await page.waitForTimeout(800);
  await page.screenshot({ path: "test-results/rules-apply-preview-zoomed.png", fullPage: true });
});

test("rules page - mobile form", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/rules");
  await page.waitForSelector("input[placeholder*='AMAZON']", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/rules-mobile-form.png", fullPage: true });
});

test("hamburger icon - closed state", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/");
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/hamburger-closed.png", clip: { x: 0, y: 0, width: 390, height: 80 } });
});

test("hamburger icon - open state", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/");
  await page.waitForTimeout(400);
  // Dismiss any Vite error overlay
  await page.keyboard.press("Escape");
  await page.waitForTimeout(200);
  // Check the swap checkbox directly to toggle to open state
  await page.evaluate(() => {
    const cb = document.querySelector("label.swap-rotate input[type=checkbox]");
    if (cb) { cb.checked = true; cb.dispatchEvent(new Event("change")); }
  });
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/hamburger-open.png", clip: { x: 0, y: 0, width: 390, height: 80 } });
});
