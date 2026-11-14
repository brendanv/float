// Behavioral regression tests for transaction-page interactions: what the UI
// sends to floatd when editing, filtering and paging. Uses the mocked API and
// inspects the outgoing request bodies.
import { test, expect } from "./fixtures.js";
import { mockLedgerApi, mockAccountRegisterRows } from "./mock-api.js";

function usd(quantity) {
  return [{ commodity: "USD", quantity }];
}

const coffee = {
  fid: "c0ffee01",
  date: "2026-03-20",
  description: "Coffee Shop | latte",
  payee: "Coffee Shop",
  note: "latte",
  status: "",
  comment: "receipt in drawer",
  postings: [
    { account: "expenses:dining", comment: "date:2026-03-22, split with Bob", amounts: usd("5.00") },
    { account: "assets:checking", amounts: usd("-5.00") },
  ],
  tags: { trip: "paris" },
};

function simple(fid, date, payee, quantity) {
  return {
    fid,
    date,
    description: `${payee} | item`,
    payee,
    note: "item",
    status: "",
    postings: [
      { account: "expenses:dining", amounts: usd(quantity) },
      { account: "assets:checking", amounts: usd(`-${quantity}`) },
    ],
    tags: {},
  };
}

const transactions = [
  coffee,
  simple("5q000001", "2026-03-19", "SQ *COFFEE", "4.00"),
  simple("5q000002", "2026-03-18", "SQ COFFEE", "3.00"),
  simple("9e000001", "2026-03-17", "Grocer", "20.00"),
];

// Records the decoded JSON body of every call to an RPC.
function captureRpc(page, method) {
  const bodies = [];
  page.on("request", (req) => {
    if (req.url().endsWith(`/float.v1.LedgerService/${method}`)) {
      bodies.push(JSON.parse(req.postData() || "{}"));
    }
  });
  return bodies;
}

function rowFor(page, text) {
  return page.locator("tbody tr").filter({ hasText: text }).first();
}

function inlineInput(page) {
  return page.locator("tbody input:not([type=checkbox])").first();
}

test("inline description edit sends posting comments back", async ({ page }) => {
  await mockLedgerApi(page, { transactions });
  const updates = captureRpc(page, "UpdateTransaction");
  await page.goto("/#/transactions");

  await rowFor(page, "Coffee Shop").getByTitle("Click to edit description").click();
  await inlineInput(page).fill("Coffee Shop | oat latte");
  await inlineInput(page).press("Enter");

  await expect.poll(() => updates.length).toBe(1);
  const [body] = updates;
  expect(body.description).toBe("Coffee Shop | oat latte");
  expect(body.postings.map((p) => p.comment ?? "")).toEqual(["date:2026-03-22, split with Bob", ""]);
  // Neither comment nor tags are sent, so the server keeps both.
  expect(body.replaceComment).toBeUndefined();
  expect(body.replaceTags).toBeUndefined();
});

test("removing the last tag in the detail editor clears tags", async ({ page }) => {
  await mockLedgerApi(page, { transactions });
  const updates = captureRpc(page, "UpdateTransaction");
  await page.goto("/#/transactions");

  await rowFor(page, "Coffee Shop").locator("td").nth(1).click();
  const form = page.locator("tbody form");
  await form.getByTitle('Remove tag "trip"').click();
  await form.getByRole("button", { name: "Save" }).click();

  await expect.poll(() => updates.length).toBe(1);
  const [body] = updates;
  expect(body.replaceTags).toBe(true);
  expect(body.tags ?? {}).toEqual({});
  expect(body.postings[0].comment).toBe("date:2026-03-22, split with Bob");
});

test("clicking a payee on the transactions page filters to that payee", async ({ page }) => {
  await mockLedgerApi(page, { transactions });
  const lists = captureRpc(page, "ListTransactions");
  await page.goto("/#/transactions");
  await expect(rowFor(page, "Grocer")).toBeVisible();

  await page.locator("tbody strong", { hasText: "Coffee Shop" }).click();

  await expect.poll(() => lists.at(-1)?.query).toContain("payee:^Coffee Shop$");
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await expect(page.getByText("payee: Coffee Shop")).toBeVisible();
});

test("payee filter matches regex metacharacters literally", async ({ page }) => {
  await mockLedgerApi(page, { transactions });
  const lists = captureRpc(page, "ListTransactions");
  await page.goto(`/#/transactions?payee=${encodeURIComponent("SQ *COFFEE")}`);

  await expect.poll(() => lists.at(-1)?.query).toContain("payee:^SQ \\*COFFEE$");
  await expect(page.locator("tbody strong")).toHaveText(["SQ *COFFEE"]);
});

test("a From-only custom range filters by an open-ended date", async ({ page }) => {
  await mockLedgerApi(page, { transactions });
  const lists = captureRpc(page, "ListTransactions");
  await page.goto("/#/transactions");
  await expect(rowFor(page, "Grocer")).toBeVisible();

  await page.getByRole("button", { name: /\d{4}/ }).first().click();
  const [fromInput, toInput] = await page.locator("input[type=date]").all();
  await toInput.fill("");
  await fromInput.fill("2026-03-10");
  await page.keyboard.press("Escape");

  await expect.poll(() => lists.at(-1)?.query).toContain("date:2026-03-10..");
  await expect(page.getByRole("button", { name: /Since Mar 10, 2026/ })).toBeVisible();
});

test("description edit in account-register mode fetches postings first", async ({ page }) => {
  await mockLedgerApi(page);
  const lists = captureRpc(page, "ListTransactions");
  const updates = captureRpc(page, "UpdateTransaction");
  await page.goto("/#/transactions?account=assets:checking");

  const fid = mockAccountRegisterRows[0].fid;
  await page.locator("tbody tr").first().getByTitle("Click to edit description").click();
  await inlineInput(page).fill("Renamed | in register");
  await inlineInput(page).press("Enter");

  await expect.poll(() => updates.length).toBe(1);
  expect(lists.some((b) => b.query?.includes(`code:${fid}`))).toBe(true);
  expect(updates[0].fid).toBe(fid);
  expect(updates[0].postings.length).toBeGreaterThanOrEqual(2);
});

test("a transaction edit refreshes balances shown elsewhere", async ({ page }) => {
  await mockLedgerApi(page, { transactions });
  const balances = captureRpc(page, "GetBalances");
  await page.goto("/#/");
  await expect.poll(() => balances.length).toBeGreaterThan(0);

  await page.goto("/#/transactions");
  await expect(rowFor(page, "Grocer")).toBeVisible();
  const before = balances.length;
  await rowFor(page, "Grocer").locator("button.rounded-full").click();

  await page.goto("/#/");
  await expect.poll(() => balances.length).toBeGreaterThan(before);
});

test("editing a row keeps the current page", async ({ page }) => {
  let many = Array.from({ length: 30 }, (_, i) =>
    simple(`p${String(i).padStart(7, "0")}`, `2026-03-${String((i % 28) + 1).padStart(2, "0")}`, `Payee ${i}`, `${i + 1}.00`),
  );
  await mockLedgerApi(page, { transactions: many });
  // Serve the toggled status back, as floatd would, so the refetch returns
  // new data (identical data would be structurally shared and not re-render).
  await page.route("**/float.v1.LedgerService/ListTransactions", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ transactions: many }) }),
  );
  await page.route("**/float.v1.LedgerService/UpdateTransactionStatus", (route) => {
    const { fid, status } = JSON.parse(route.request().postData() || "{}");
    many = many.map((tx) => (tx.fid === fid ? { ...tx, status } : tx));
    return route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
  });
  const statusCalls = captureRpc(page, "UpdateTransactionStatus");
  await page.goto("/#/transactions");

  await page.getByRole("button", { name: "Go to next page" }).click();
  await expect(page.getByText("26–30 of 30")).toBeVisible();
  await page.locator("tbody tr").first().locator("button.rounded-full").click();

  await expect.poll(() => statusCalls.length).toBe(1);
  await expect(page.locator("tbody tr").first().locator("button.rounded-full svg")).toHaveClass(/text-success/);
  await expect(page.getByText("26–30 of 30")).toBeVisible();
});
