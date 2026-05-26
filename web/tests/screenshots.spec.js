import { test, expect } from "@playwright/test";
import { mockLedgerApi, mockStripeImportCandidates } from "./mock-api.js";

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
  // Click the date cell (a stable target that won't intercept with the inline tag editor)
  await page.click("tbody tr:first-child td:nth-child(2)");
  await page.waitForTimeout(200);
  await page.click("button:has-text('Delete')");
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-delete.png", fullPage: true });
});

test("transactions page - expanded edit row", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.click("tbody tr:first-child td:nth-child(2)");
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/transactions-expanded-edit.png", fullPage: true });
});

test("transactions page - from cell inline edit", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  // Activate the inline "From" cell on the first row (a normal 2-posting
  // transaction). The From column is the 6th cell: select, date, status,
  // description, tags, from.
  await page.click("tbody tr:first-child td:nth-child(6) > span");
  await page.waitForTimeout(200);
  // Open the typeahead popover
  await page.click("tbody tr:first-child td:nth-child(6) button[role='combobox']");
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-from-edit.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 600 } });
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

test("monthly dashboard page", async ({ page }) => {
  await page.goto("/#/monthly");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/monthly-dashboard.png", fullPage: true });
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

test("accounts page - rename dialog input", async ({ page }) => {
  await page.goto("/#/accounts");
  await page.waitForSelector("h2, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(600);
  await page.locator("button", { hasText: "Rename Account" }).first().click();
  await page.waitForSelector('[data-slot="dialog-content"]');
  await page.waitForTimeout(200);
  await page.locator('[data-slot="dialog-content"] [role="combobox"]').click();
  await page.waitForSelector('[role="option"]');
  await page.locator('[role="option"]', { hasText: "assets:checking" }).first().click();
  const newNameInput = page.locator('#rename-new');
  await newNameInput.click();
  await newNameInput.fill("assets:bank:checking");
  await page.waitForTimeout(150);
  await page.screenshot({ path: "test-results/accounts-rename-input.png", fullPage: false });
});

test("accounts page - rename dialog typeahead open", async ({ page }) => {
  await page.goto("/#/accounts");
  await page.waitForSelector("h2, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(600);
  await page.locator("button", { hasText: "Rename Account" }).first().click();
  await page.waitForSelector('[data-slot="dialog-content"]');
  await page.waitForTimeout(200);
  await page.locator('[data-slot="dialog-content"] [role="combobox"]').click();
  await page.waitForSelector('[role="option"]');
  await page.waitForTimeout(150);
  await page.screenshot({ path: "test-results/accounts-rename-typeahead.png", fullPage: false });
});

test("accounts page - rename dialog confirm", async ({ page }) => {
  await page.goto("/#/accounts");
  await page.waitForSelector("h2, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(600);
  await page.locator("button", { hasText: "Rename Account" }).first().click();
  await page.waitForSelector('[data-slot="dialog-content"]');
  await page.locator('[data-slot="dialog-content"] [role="combobox"]').click();
  await page.waitForSelector('[role="option"]');
  await page.locator('[role="option"]', { hasText: "assets:checking" }).first().click();
  const newNameInput = page.locator('#rename-new');
  await newNameInput.click();
  await newNameInput.fill("assets:bank:checking");
  await page.locator('[data-slot="dialog-content"]').locator("button", { hasText: "Continue" }).click();
  await page.waitForSelector('button:has-text("Confirm Rename")');
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/accounts-rename-confirm.png", fullPage: false });
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

test("transactions page - account register bulk edit toolbar", async ({ page }) => {
  await page.goto("/#/transactions?account=assets%3Achecking");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  const checkboxes = await page.locator("tbody [data-slot='checkbox']").all();
  for (const cb of checkboxes.slice(0, 3)) {
    await cb.click();
  }
  await expect(page.getByText("Mark reviewed")).toBeVisible();
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/transactions-account-register-bulk-edit.png", fullPage: true });
});

test("transactions page - account register edit account", async ({ page }) => {
  await page.goto("/#/transactions?account=assets%3Achecking");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  // Click the first editable other-account cell
  await page.locator('[title="Click to change account"]').first().click();
  await page.waitForSelector("button[role='combobox']", { timeout: 5000 });
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/transactions-account-register-edit-account.png", fullPage: true });
});

test("transactions page - account register edit account dropdown open", async ({ page }) => {
  await page.goto("/#/transactions?account=assets%3Achecking");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  // Click the first editable other-account cell to open the editor
  await page.locator('[title="Click to change account"]').first().click();
  await page.waitForSelector("button[role='combobox']", { timeout: 5000 });
  await page.waitForTimeout(200);
  // Open the AccountInput dropdown (it's inside the table, unlike the pagination select)
  await page.locator("table button[role='combobox']").click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-account-register-edit-account-open.png", fullPage: true });
});

test("transactions page - account register mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/transactions?account=assets%3Achecking");
  await page.waitForSelector(".card", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/transactions-account-register-mobile.png", fullPage: true });
});

test("transactions page - account register mobile edit account", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/transactions?account=assets%3Achecking");
  await page.waitForSelector(".card", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  // Target visible elements only — the desktop table is display:none on mobile viewports
  await page.locator('[title="Click to change account"]:visible').first().click();
  await page.locator("button[role='combobox']:visible").first().waitFor({ timeout: 5000 });
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/transactions-account-register-mobile-edit.png", fullPage: true });
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
  // Check the first three transaction checkboxes (base-ui renders button[data-slot="checkbox"], not input)
  const checkboxes = await page.locator("tbody [data-slot='checkbox']").all();
  for (const cb of checkboxes.slice(0, 3)) {
    await cb.click();
  }
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/transactions-bulk-edit.png", fullPage: true });
});

test("transactions page - bulk add-tag mode", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(300);
  const checkboxes = await page.locator("tbody [data-slot='checkbox']").all();
  for (const cb of checkboxes.slice(0, 2)) {
    await cb.click();
  }
  await page.waitForTimeout(150);
  await page.click("button:has-text('Add tag')");
  await page.waitForTimeout(150);
  await page.screenshot({ path: "test-results/transactions-bulk-add-tag.png", fullPage: true });
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
  await page.locator(".lucide-pencil").click();
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
  await page.locator(".lucide-trash-2").click();
  await page.waitForSelector('[role="dialog"]', { timeout: 5000 });
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-delete-profile-dialog.png", fullPage: true });
});

test("import page - create profile modal", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector('[role="combobox"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  // Click the "+" (Plus) button to open the create profile modal
  await page.locator(".lucide-plus").click();
  await page.waitForSelector('[role="dialog"]', { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-create-profile-modal.png", fullPage: true });
});

test("import page - create profile modal with CSV wizard", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector('[role="combobox"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.locator(".lucide-plus").click();
  await page.waitForSelector('[role="dialog"]', { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Fill in profile name (Bank Account is now a Combobox — skip filling it
  // here; we just want to see the CSV mapping UI appear).
  await page.fill('input[placeholder="e.g. Chase Checking"]', "Chase Checking");
  // Paste a sample CSV to trigger column mapping UI
  await page.fill('textarea[placeholder*="Date,Description,Amount"]',
    "Date,Description,Amount\n2026-04-01,AMAZON.COM,-45.00\n2026-04-02,PAYROLL DIRECT DEPOSIT,2000.00"
  );
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-create-profile-modal-wizard.png", fullPage: true });
});

test("import page - create profile modal with file upload and generated rules", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector('[role="combobox"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.locator(".lucide-plus").click();
  await page.waitForSelector('[role="dialog"]', { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Fill profile name and account
  await page.fill('input[placeholder="e.g. Chase Checking"]', "Chase Checking");
  // Upload a mock CSV file via the hidden file input inside the dialog
  const fileInput = page.getByRole("dialog").locator('input[type="file"]');
  await fileInput.setInputFiles({
    name: "chase-checking.csv",
    mimeType: "text/csv",
    buffer: Buffer.from("Date,Description,Amount\n2026-04-01,AMAZON.COM,-45.00\n2026-04-02,PAYROLL DIRECT DEPOSIT,2000.00\n"),
  });
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-create-profile-modal-file-upload.png", fullPage: true });
  // Click "Generate Rules from File" to trigger backend call (mocked)
  await page.locator('button:has-text("Generate Rules from File")').click();
  await page.waitForTimeout(500);
  // Scroll the dialog to the bottom to show the generated rules textarea
  const dialog = page.getByRole("dialog");
  await dialog.evaluate((el) => { el.scrollTop = el.scrollHeight; });
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/import-create-profile-modal-generated-rules.png", fullPage: true });
});

async function loadImportPreview(page) {
  await page.goto("/#/import");
  await page.waitForSelector('[role="combobox"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.locator('[role="combobox"]').first().click();
  await page.waitForTimeout(200);
  await page.locator('[role="option"]').first().click();
  await page.waitForTimeout(200);
  await page.locator('#csv-file').setInputFiles({
    name: "bank.csv",
    mimeType: "text/csv",
    buffer: Buffer.from("date,amount,description\n2026-03-28,-42.99,AMAZON"),
  });
  await page.waitForTimeout(200);
  await page.locator('button[type="submit"]').click();
  await page.waitForSelector("table", { timeout: 5000 });
  await page.waitForTimeout(400);
}

test("import page - preview loaded", async ({ page }) => {
  await loadImportPreview(page);
  await page.screenshot({ path: "test-results/import-preview.png", fullPage: true });
});

test("import page - preview sorted by description", async ({ page }) => {
  await loadImportPreview(page);
  await page.locator("th button").filter({ hasText: "Description" }).click();
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/import-preview-sorted.png", fullPage: true });
});

test("import page - preview filtered by rule match", async ({ page }) => {
  await loadImportPreview(page);
  await page.locator("button", { hasText: "Rule matched" }).click();
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/import-preview-filtered-matched.png", fullPage: true });
});

test("import history page", async ({ page }) => {
  await page.goto("/#/imports");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-history.png", fullPage: true });
});

test("import history page - file viewer dialog", async ({ page }) => {
  await page.goto("/#/imports");
  await page.waitForSelector("table", { timeout: 5000 });
  await page.waitForTimeout(400);
  await page.locator("button", { hasText: "View file" }).first().click();
  await page.waitForSelector("pre", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/import-history-file-viewer.png", fullPage: true });
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
  // AccountInput renders as button[role="combobox"]; click the first one
  // (Set Category Account)
  await page.locator('button[role="combobox"]').first().click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/rules-account-typeahead.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 360 } });
});

test("rules page - account typeahead filtered", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Open the first popover then type into the CommandInput inside it
  await page.locator('button[role="combobox"]').first().click();
  await page.waitForTimeout(200);
  await page.fill('input[placeholder="Search expenses:shopping..."]', "exp");
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/rules-account-typeahead-filtered.png", fullPage: false, clip: { x: 0, y: 0, width: 1280, height: 360 } });
});

test("rules page - apply preview drawer", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Click "Preview Changes" button — opens a bottom drawer
  await page.click('button:has-text("Preview Changes")');
  // Wait for drawer content to appear
  await page.waitForSelector('[data-slot="drawer-content"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/rules-apply-preview.png", fullPage: false });
});

test("rules page - apply preview drawer mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/rules");
  await page.waitForSelector("table tbody tr", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Click Preview Changes — opens drawer
  await page.click('button:has-text("Preview Changes")');
  await page.waitForSelector('[data-slot="drawer-content"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/rules-apply-preview-zoomed.png", fullPage: false });
});

test("rules page - mobile form", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/rules");
  await page.waitForSelector("input[placeholder*='AMAZON']", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/rules-mobile-form.png", fullPage: true });
});

test("rules page - suggest rules wizard source step", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.click('button:has-text("Suggest Rules")');
  await page.waitForSelector('[data-slot="dialog-content"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/rules-ai-wizard-source.png", fullPage: false });
});

test("rules page - suggest rules wizard suggestions step", async ({ page }) => {
  await page.goto("/#/rules");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.click('button:has-text("Suggest Rules")');
  await page.waitForSelector('[data-slot="dialog-content"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.click('button:has-text("Analyze")');
  await page.waitForSelector('text=Suggested Rules', { timeout: 8000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/rules-ai-wizard-suggestions.png", fullPage: false });
});

test("portfolio page", async ({ page }) => {
  await page.goto("/#/portfolio");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(800);
  await page.screenshot({ path: "test-results/portfolio.png", fullPage: true });
});

test("payees page", async ({ page }) => {
  await page.goto("/#/payees");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/payees.png", fullPage: true });
});

test("payees page - set payee inline form open", async ({ page }) => {
  await page.goto("/#/payees");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(500);
  // Click "Set payee" on the first description row
  await page.locator("button:has-text('Set payee')").first().click();
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/payees-set-payee.png", fullPage: true });
});

test("payees page - suggest rules wizard source step", async ({ page }) => {
  await page.goto("/#/payees");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(500);
  await page.locator("button:has-text('Suggest Rules')").click();
  await page.waitForSelector("[role='dialog']", { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/payees-suggest-rules-source.png", fullPage: true });
});

test("payees page - suggest rules wizard suggestions step", async ({ page }) => {
  await page.goto("/#/payees");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(500);
  await page.locator("button:has-text('Suggest Rules')").click();
  await page.waitForSelector("[role='dialog']", { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(200);
  await page.locator("[role='dialog'] button:has-text('Analyze')").click();
  await page.waitForTimeout(800);
  await page.screenshot({ path: "test-results/payees-suggest-rules-suggestions.png", fullPage: true });
});

test("payees page - uncategorized filter active", async ({ page }) => {
  await page.goto("/#/payees");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(500);
  // Enable the "Uncategorized only" checkbox
  await page.locator("[data-slot='checkbox']").click();
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/payees-uncategorized-filter.png", fullPage: true });
});

test("payees page - mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/payees");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/payees-mobile.png", fullPage: true });
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

test("snapshots page", async ({ page }) => {
  await page.goto("/#/snapshots");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/snapshots.png", fullPage: true });
});

test("snapshots page - diff dialog (modified)", async ({ page }) => {
  await page.goto("/#/snapshots");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Click the first "View diff" button (most recent snapshot — modified file)
  await page.locator("button:has-text('View diff')").first().click();
  await page.waitForSelector("[role='dialog']", { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/snapshots-diff.png", fullPage: true });
});

test("snapshots page - diff dialog (added + modified)", async ({ page }) => {
  await page.goto("/#/snapshots");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Click the third "View diff" button — the import snapshot has both an added file and a modified file
  await page.locator("button:has-text('View diff')").nth(2).click();
  await page.waitForSelector("[role='dialog']", { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/snapshots-diff-multi.png", fullPage: true });
});

test("settings page", async ({ page }) => {
  await page.goto("/#/settings");
  await page.waitForSelector("h2", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/settings.png", fullPage: true });
});

test("settings page - collapsed cards", async ({ page }) => {
  await page.goto("/#/settings");
  await page.waitForSelector("h2", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Collapse Stripe and AI cards by clicking their triggers
  const triggers = page.locator("[data-slot='collapsible-trigger']");
  await triggers.nth(0).click();
  await triggers.nth(1).click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/settings-collapsed.png", fullPage: true });
});

test("settings page - features disabled", async ({ page }) => {
  await mockLedgerApi(page, { stripeEnabled: false, aiEnabled: false });
  await page.goto("/#/settings");
  await page.waitForSelector("h2", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/settings-disabled.png", fullPage: true });
});

test("query page - hledger tab", async ({ page }) => {
  await page.goto("/#/hledger-query");
  await page.waitForSelector("textarea", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/query-hledger.png", fullPage: true });
});

test("query page - natural language tab", async ({ page }) => {
  await page.goto("/#/hledger-query");
  await page.waitForSelector("[role='tablist']", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.click("[role='tab']:has-text('Natural language')");
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/query-natural-language.png", fullPage: true });
});

test("query page - natural language with answer", async ({ page }) => {
  await page.goto("/#/hledger-query");
  await page.waitForSelector("[role='tablist']", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.click("[role='tab']:has-text('Natural language')");
  await page.waitForTimeout(300);
  await page.fill("textarea", "How much did I spend on groceries last month?");
  await page.click("button:has-text('Run')");
  await page.waitForSelector("text=You spent", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/query-natural-language-answer.png", fullPage: true });
});

test("query page - natural language with details open", async ({ page }) => {
  await page.goto("/#/hledger-query");
  await page.waitForSelector("[role='tablist']", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.click("[role='tab']:has-text('Natural language')");
  await page.waitForTimeout(300);
  await page.fill("textarea", "How much did I spend on groceries last month?");
  await page.click("button:has-text('Run')");
  await page.waitForSelector("text=You spent", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.click("button:has-text('Query details')");
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/query-natural-language-details.png", fullPage: true });
});

test("connections page - with linked accounts", async ({ page }) => {
  await page.goto("/#/connections");
  await page.waitForSelector("h1, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/connections.png", fullPage: true });
});

test("connections page - disabled (no env var)", async ({ page }) => {
  await mockLedgerApi(page, {
    stripeEnabled: false,
  });
  await page.goto("/#/connections");
  await page.waitForSelector("h1, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/connections-disabled.png", fullPage: true });
});

test("connections page - fetch transactions panel", async ({ page }) => {
  await page.goto("/#/connections");
  await page.waitForSelector("h1, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(500);
  const fetchBtn = page.locator("button:has-text('Fetch Transactions')").first();
  await fetchBtn.click();
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/connections-fetch-panel.png", fullPage: true });
});

test("connections page - fetch all panel", async ({ page }) => {
  await page.goto("/#/connections");
  await page.waitForSelector("h1, .loading", { timeout: 5000 }).catch(() => {});
  await page.evaluate(() => document.querySelector("vite-error-overlay")?.remove());
  await page.waitForTimeout(500);
  const fetchAllBtn = page.getByRole("button", { name: "Fetch All", exact: true }).first();
  await fetchAllBtn.click();
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/connections-fetch-all.png", fullPage: true });
});

test("logs page", async ({ page }) => {
  await page.goto("/#/logs?demoLogs=1");
  await page.waitForSelector("h2", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/logs.png", fullPage: true });
});
