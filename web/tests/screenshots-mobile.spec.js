import { test } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test.use({ viewport: { width: 390, height: 844 } });

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("home page mobile", async ({ page }) => {
  await page.goto("/#/");
  await page.waitForSelector(".balance-summary, article, table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-home.png", fullPage: true });
});

test("transactions page mobile", async ({ page }) => {
  await page.goto("/#/transactions");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-transactions.png", fullPage: true });
});

test("add transaction page mobile", async ({ page }) => {
  await page.goto("/#/add");
  await page.waitForSelector("form", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/mobile-add-transaction.png", fullPage: true });
});

test("trends page mobile", async ({ page }) => {
  await page.goto("/#/trends");
  await page.waitForSelector(".trends-chart canvas", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(1000);
  await page.screenshot({ path: "test-results/mobile-trends.png", fullPage: true });
});

test("prices page mobile", async ({ page }) => {
  await page.goto("/#/prices");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-prices.png", fullPage: true });
});

test("import page mobile", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector("select, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/mobile-import.png", fullPage: true });
});

test("import page paste CSV mode mobile", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector("select, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.locator("button", { hasText: "Paste CSV instead" }).click();
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/mobile-import-paste-csv.png", fullPage: true });
});

test("add transaction modal mobile (drawer)", async ({ page }) => {
  await page.goto("/#/");
  await page.waitForTimeout(400);
  // Open sidebar via the sidebar trigger button, then click Add Transaction
  await page.locator('[data-slot="sidebar-trigger"]').click();
  await page.waitForTimeout(300);
  await page.locator('[data-slot="sidebar-menu-button"]:has-text("Add Transaction")').click();
  await page.waitForSelector('[role="dialog"]', { timeout: 5000 });
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-add-transaction-modal.png", fullPage: true });
});

test("add transaction modal mobile with template selected", async ({ page }) => {
  await page.goto("/#/");
  await page.waitForTimeout(400);
  await page.locator('[data-slot="sidebar-trigger"]').click();
  await page.waitForTimeout(300);
  await page.locator('[data-slot="sidebar-menu-button"]:has-text("Add Transaction")').click();
  await page.waitForSelector('[role="dialog"]', { timeout: 5000 });
  await page.waitForTimeout(500);
  await page.locator('[role="dialog"] button:has-text("Mortgage Payment")').click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/mobile-add-transaction-modal-template.png", fullPage: true });
});

test("import create profile modal mobile (drawer)", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector("select, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  await page.locator(".lucide-plus").click();
  await page.waitForSelector('[role="dialog"]', { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-import-create-profile.png", fullPage: true });
});

test("import edit profile modal mobile (drawer)", async ({ page }) => {
  await page.goto("/#/import");
  await page.waitForSelector('[data-slot="native-select"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // Select a bank profile using the native select element
  await page.selectOption('[data-slot="native-select"]', { label: "Chase Checking" });
  await page.waitForTimeout(300);
  await page.locator(".lucide-pencil").click();
  await page.waitForSelector('[role="dialog"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(600);
  await page.screenshot({ path: "test-results/mobile-import-edit-profile.png", fullPage: true });
});

test("accounts rename dialog mobile (drawer)", async ({ page }) => {
  await page.goto("/#/accounts");
  await page.waitForSelector('[data-slot="card"]', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.locator(".lucide-pencil").first().click();
  await page.waitForSelector('[role="dialog"]', { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-accounts-rename.png", fullPage: true });
});

test("snapshots diff dialog mobile (dialog)", async ({ page }) => {
  await page.goto("/#/snapshots");
  await page.waitForSelector("table, .loading", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.locator("button", { hasText: "View diff" }).first().click();
  await page.waitForSelector('[role="dialog"]', { timeout: 3000 }).catch(() => {});
  await page.waitForTimeout(500);
  await page.screenshot({ path: "test-results/mobile-snapshots-diff.png", fullPage: true });
});

test("import history file dialog mobile (dialog)", async ({ page }) => {
  await page.goto("/#/imports");
  await page.waitForSelector("table", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  // The View file button may be off-screen on mobile — click the first row to go to transactions,
  // or find the button by its icon if visible
  const viewBtn = page.locator('button:has(.lucide-file-text)').first();
  const btnVisible = await viewBtn.isVisible().catch(() => false);
  if (btnVisible) {
    await viewBtn.click();
    await page.waitForSelector('[role="dialog"]', { timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);
  }
  await page.screenshot({ path: "test-results/mobile-imports-file-dialog.png", fullPage: true });
});
