import { test, expect } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

// Suggestions are computed by floatd when transactions are written; the list
// only shows them (Transaction.suggestedAccount) with a one-click apply.
const transactions = [
  {
    fid: "u1b2c3d4",
    date: "2026-03-25",
    description: "Whole Foods | groceries",
    payee: "Whole Foods",
    note: "groceries",
    status: "",
    postings: [
      { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "34.99" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-34.99" }] },
    ],
    tags: {},
    suggestedAccount: { account: "expenses:food:groceries", confidence: "high", reasoning: "Matches past Whole Foods purchases." },
  },
  {
    fid: "u2b2c3d5",
    date: "2026-03-24",
    description: "Refund from merchant",
    status: "",
    postings: [
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "20.00" }] },
      { account: "income:unknown", amounts: [{ commodity: "USD", quantity: "-20.00" }] },
    ],
    tags: {},
    suggestedAccount: { account: "expenses:shopping", confidence: "low", reasoning: "Likely a return; merchant unclear." },
  },
  {
    fid: "u3b2c3d6",
    date: "2026-03-23",
    description: "SQ *MYSTERY",
    status: "",
    postings: [
      { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "10.99" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-10.99" }] },
    ],
    tags: {},
  },
];

function captureRpc(page, method) {
  const bodies = [];
  page.on("request", (req) => {
    if (req.url().endsWith(`/float.v1.LedgerService/${method}`)) bodies.push(req.postDataJSON());
  });
  return bodies;
}

test("suggested account shows on the row and applies in one click", async ({ page }) => {
  await mockLedgerApi(page, { transactions });
  const edits = captureRpc(page, "BulkEditTransactions");
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto("/#/transactions");

  const groceries = page.getByRole("row", { name: /Whole Foods/ });
  const chip = groceries.getByRole("button", { name: "Apply suggested account expenses:food:groceries" });
  await expect(chip).toHaveText("groceries");
  await expect(page.getByRole("button", { name: "Apply suggested account expenses:shopping" })).toBeVisible();
  // No suggestion, no chip.
  await expect(page.getByRole("row", { name: /SQ \*MYSTERY/ }).locator("[data-slot='account-suggestion']")).toHaveCount(0);

  await chip.hover();
  await expect(page.getByText("Matches past Whole Foods purchases.")).toBeVisible();
  await page.screenshot({ path: "test-results/account-suggestion-desktop.png" });

  await chip.click();
  await expect.poll(() => edits.length).toBe(1);
  expect(edits[0]).toEqual({
    fids: ["u1b2c3d4"],
    operations: [{ updateUnknownAccount: { account: "expenses:food:groceries" } }],
  });
});

test("register rows show the suggestion next to the other account", async ({ page }) => {
  await mockLedgerApi(page, {
    accountRegisterRows: [
      {
        fid: "u1b2c3d4",
        date: "2026-03-25",
        description: "Whole Foods | groceries",
        payee: "Whole Foods",
        note: "groceries",
        status: "",
        otherAccounts: ["expenses:unknown"],
        change: [{ commodity: "USD", quantity: "-34.99" }],
        runningTotal: [{ commodity: "USD", quantity: "-534.99" }],
        tags: {},
        suggestedAccount: { account: "expenses:food:groceries", confidence: "high", reasoning: "r" },
      },
    ],
  });
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto("/#/transactions?account=liabilities%3Acreditcard");
  await expect(page.getByRole("button", { name: "Apply suggested account expenses:food:groceries" })).toBeVisible();
});

test.describe("phone", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  test("tapping the chip applies without opening the editor", async ({ page }) => {
    await mockLedgerApi(page, { transactions });
    const edits = captureRpc(page, "BulkEditTransactions");
    await page.goto("/#/transactions");

    const card = page.locator("[data-slot='transaction-card'][data-fid='u1b2c3d4']");
    await expect(card.getByRole("button", { name: /Apply suggested account/ })).toBeVisible();
    await page.screenshot({ path: "test-results/account-suggestion-phone.png" });

    await card.getByRole("button", { name: /Apply suggested account/ }).tap();
    await expect.poll(() => edits.length).toBe(1);
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });
});

test("settings saves the decision model", async ({ page }) => {
  await mockLedgerApi(page);
  const calls = captureRpc(page, "SetAIDecisionModel");
  await page.goto("/#/settings");
  await expect(page.getByText("Active decision model")).toBeVisible();
  await page.getByLabel("Set decision model").fill("google/gemini-2.5-flash-lite");
  await page.locator("form", { has: page.getByLabel("Set decision model") }).getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("Decision model saved.")).toBeVisible();
  expect(calls).toEqual([{ model: "google/gemini-2.5-flash-lite" }]);
  await page.getByText("Decision model", { exact: true }).evaluate((el) => el.scrollIntoView({ block: "center" }));
  await page.screenshot({ path: "test-results/settings-decision-model.png" });
});
