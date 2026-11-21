import { expect, test } from "./fixtures.js";
import { mockLedgerApi, mockAccounts } from "./mock-api.js";

test("rule category picker offers equity accounts such as transfer clearing accounts", async ({ page }) => {
  await mockLedgerApi(page, {
    accounts: [
      ...mockAccounts,
      { name: "transfers", fullName: "equity:transfers", type: "E" },
      { name: "Checking-Savings", fullName: "equity:transfers:Checking-Savings", type: "E" },
    ],
  });
  await page.goto("/#/rules");
  await page.getByRole("combobox", { name: "Set category account" }).click();
  const popover = page.locator('[data-slot="popover-content"]:visible');
  await expect(popover.getByRole("option", { name: "equity:transfers:Checking-Savings", exact: true })).toBeVisible();
  await expect(popover.getByRole("option", { name: "expenses:groceries", exact: true })).toBeVisible();
  // Source accounts stay out of the category picker.
  await expect(popover.getByRole("option", { name: "assets:checking", exact: true })).toHaveCount(0);
});
