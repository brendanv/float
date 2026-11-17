import { test, expect } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

test("sidebar marks the group active without a review badge", async ({ page }) => {
  await page.goto("/#/lots");
  const sidebar = page.locator('[data-slot="sidebar"]');
  await expect(sidebar.locator('[data-slot="sidebar-menu-button"][data-active]')).toHaveText("Investments");
  // TanStack only marks an exact href match; grouped routes set it themselves.
  await expect(sidebar.locator('[aria-current="page"]')).toHaveText("Investments");
  await expect(sidebar.locator('[data-slot="review-count"]')).toHaveCount(0);
});

test("section tabs reach every route in a group", async ({ page }) => {
  await page.goto("/#/trends");
  const tabs = page.getByRole("navigation", { name: "Reports" });
  await expect(tabs.getByRole("link", { name: "Net worth" })).toHaveAttribute("aria-current", "page");
  await tabs.getByRole("link", { name: "Income & spending" }).click();
  await expect(page).toHaveURL(/#\/monthly$/);
  await expect(tabs.getByRole("link", { name: "Income & spending" })).toHaveAttribute("aria-current", "page");

  await page.goto("/#/settings");
  const settingsTabs = page.getByRole("navigation", { name: "Settings" });
  await expect(settingsTabs.getByRole("link")).toHaveText(["General", "Assertions", "Snapshots", "Query", "Logs"]);
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/shell-settings-tabs.png" });
});

test("N opens a new transaction outside inputs", async ({ page }) => {
  await page.goto("/#/accounts");
  await page.locator('[data-slot="sidebar"]').getByRole("link", { name: "Accounts" }).waitFor();
  await page.keyboard.press("n");
  await expect(page.getByRole("dialog").getByText("Add Transaction").first()).toBeVisible();
});

test("Ctrl+K does not open the palette over another dialog", async ({ page }) => {
  await page.goto("/#/accounts");
  await page.locator('[data-slot="sidebar"]').getByRole("link", { name: "Accounts" }).waitFor();
  await page.keyboard.press("n");
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Add Transaction").first()).toBeVisible();
  await page.keyboard.press("Control+k");
  await page.waitForTimeout(200);
  await expect(page.getByPlaceholder(/Jump to/)).toHaveCount(0);
  await expect(dialog).toHaveCount(1);
});

test("Settings marks itself as the current page", async ({ page }) => {
  await page.goto("/#/snapshots");
  const sidebar = page.locator('[data-slot="sidebar"]');
  await expect(sidebar.locator('[aria-current="page"]')).toHaveText("Settings");
});

test("command palette jumps to an account", async ({ page }) => {
  await page.goto("/#/");
  await page.getByRole("button", { name: /Jump to/ }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByPlaceholder(/Jump to/).fill("checking");
  await page.waitForTimeout(200);
  await page.screenshot({ path: "test-results/command-palette.png" });
  await dialog.getByRole("option", { name: /checking/ }).first().click();
  await expect(page).toHaveURL(/#\/transactions\?account=assets/);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("Ctrl+K opens the command palette and lists pages", async ({ page }) => {
  await page.goto("/#/");
  await page.locator('[data-slot="sidebar"]').getByRole("link", { name: "Overview" }).waitFor();
  await page.keyboard.press("Control+k");
  const dialog = page.getByRole("dialog");
  await dialog.getByPlaceholder(/Jump to/).fill("lots");
  await dialog.getByRole("option", { name: /Lots/ }).first().click();
  await expect(page).toHaveURL(/#\/lots$/);
});
