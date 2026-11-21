// Every route renders against a real floatd + hledger with the fixture
// ledger: no page errors, no failed RPCs, and some content that can only come
// from the server's real responses.
import { test, expect, settle } from "./fixtures.js";

const routes = [
  { path: "/", text: [/Net worth/i, "checking"] },
  { path: "/transactions", text: ["Mystery Merchant", "Latte"] },
  { path: "/add", text: ["Add transaction"] },
  { path: "/trends", text: [/Net worth/i] },
  { path: "/monthly", text: ["rent"] },
  { path: "/portfolio", text: ["AAPL"] },
  { path: "/lots", text: ["AAPL"] },
  { path: "/prices", text: ["AAPL"] },
  { path: "/accounts", text: ["brokerage", "credit-card"] },
  { path: "/payees", text: ["Landlord"] },
  { path: "/rules", text: [] },
  { path: "/templates", text: [] },
  { path: "/connections", text: [] },
  { path: "/import", text: ["Upload CSV"] },
  { path: "/imports", text: [] },
  { path: "/settings", text: [] },
  { path: "/assertions", text: [] },
  { path: "/transfers", text: [] },
  { path: "/snapshots", text: ["startup migrations"] },
  { path: "/hledger-query", text: [] },
  { path: "/logs", text: [] },
];

for (const { path, text } of routes) {
  test(`renders ${path} with real data`, async ({ page }) => {
    await page.goto(`/#${path}`);
    await expect(page.locator("h1").first()).toBeVisible();
    await settle(page);
    for (const t of text) {
      await expect(page.getByText(t).first()).toBeVisible();
    }
  });
}
