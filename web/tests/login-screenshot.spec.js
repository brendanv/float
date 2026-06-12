import { test, expect } from "@playwright/test";
import { mockLedgerApi } from "./mock-api.js";

test("login page renders without app shell", async ({ page }) => {
  await mockLedgerApi(page);
  await page.route("**/api/auth", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify({ enabled: true }) })
  );
  await page.goto("/#/login");
  await expect(page.getByLabel("Passphrase")).toBeVisible();
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/login.png", fullPage: true });
});

test("login page shows error on wrong passphrase", async ({ page }) => {
  await mockLedgerApi(page);
  await page.route("**/api/auth", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify({ enabled: true }) })
  );
  await page.route("**/api/login", (route) => route.fulfill({ status: 401, body: "incorrect passphrase" }));
  await page.goto("/#/login");
  await page.getByLabel("Passphrase").fill("wrong");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByText("Incorrect passphrase.")).toBeVisible();
  await page.screenshot({ path: "test-results/login-error.png", fullPage: true });
});

test("unauthenticated RPC redirects to login", async ({ page }) => {
  await page.route("**/api/auth", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify({ enabled: true }) })
  );
  // All RPCs fail with the Connect unauthenticated error shape.
  await page.route("**/float.v1.LedgerService/**", (route) =>
    route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ code: "unauthenticated", message: "authentication required" }),
    })
  );
  await page.goto("/#/");
  await expect(page.getByLabel("Passphrase")).toBeVisible({ timeout: 5000 });
  await expect(page).toHaveURL(/#\/login/);
});
