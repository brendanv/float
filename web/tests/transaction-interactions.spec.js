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

  await rowFor(page, "Coffee Shop").getByRole("button", { name: "Edit" }).click();
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
  await expect(page.locator("tbody tr[data-slot=transaction-row]")).toHaveCount(1);
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

test("the review link opens Needs review over all dates", async ({ page }) => {
  await mockLedgerApi(page, { transactions: [...transactions, { ...simple("cl000001", "2026-03-16", "Cleared Co", "1.00"), status: "Cleared" }] });
  const lists = captureRpc(page, "ListTransactions");
  await page.goto(`/#/transactions?search=${encodeURIComponent("not:status:*")}`);

  // The list query is the chip's token alone: no date range, no search.
  const listQueries = () => lists.filter((b) => !b.limit).map((b) => b.query ?? []);
  await expect.poll(() => listQueries().at(-1)).toEqual(["not:status:*"]);
  await expect(page.getByRole("button", { name: /Needs review/ })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: /All time/ })).toBeVisible();
  await expect(page.locator("tbody tr[data-slot=transaction-row]")).toHaveCount(4);
  await expect(page.getByLabel("Search transactions")).toHaveValue("");
  await expect.poll(() => new URL(page.url()).hash).not.toContain("status");

  // Turning the chip off lists everything (still all dates).
  await page.getByRole("button", { name: /Needs review/ }).click();
  await expect.poll(() => listQueries().at(-1)).toEqual([]);
  await expect(page.locator("tbody tr[data-slot=transaction-row]")).toHaveCount(5);
});

test("rows group by day unless sorted by another column", async ({ page }) => {
  await mockLedgerApi(page, { transactions });
  await page.goto("/#/transactions");
  await expect(rowFor(page, "Grocer")).toBeVisible();
  await expect(page.locator("tbody tr[data-slot=day-header]")).toHaveCount(4);
  await expect(page.locator("tbody tr[data-slot=day-header]").first()).toContainText("20 Mar");

  await page.getByRole("button", { name: "Amount" }).click();
  await expect(page.locator("tbody tr[data-slot=day-header]")).toHaveCount(0);
  await expect(rowFor(page, "Grocer")).toContainText("2026-03-17");
});

test("description edit in account-register mode fetches postings first", async ({ page }) => {
  await mockLedgerApi(page);
  const lists = captureRpc(page, "ListTransactions");
  const updates = captureRpc(page, "UpdateTransaction");
  await page.goto("/#/transactions?account=assets:checking");

  const fid = mockAccountRegisterRows[0].fid;
  await page.locator("tbody tr[data-slot=transaction-row]").first().getByTitle("Click to edit description").click();
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
  await rowFor(page, "Grocer").locator("button[data-slot=status-mark]").click();

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
  await page.locator("tbody tr[data-slot=transaction-row]").first().locator("button[data-slot=status-mark]").click();

  await expect.poll(() => statusCalls.length).toBe(1);
  await expect(page.locator("tbody tr[data-slot=transaction-row]").first().locator("button[data-slot=status-mark]")).toHaveAttribute("data-status", "Cleared");
  await expect(page.getByText("26–30 of 30")).toBeVisible();
});

test.describe("mobile", () => {
  test.use({ viewport: { width: 390, height: 844 } });

  function card(page, text) {
    return page.locator("[data-slot=transaction-card]").filter({ hasText: text }).locator("visible=true").first();
  }

  test("tapping a card opens the editor with readable accounts and saves the description", async ({ page }) => {
    await mockLedgerApi(page, { transactions });
    const updates = captureRpc(page, "UpdateTransaction");
    await page.goto("/#/transactions");

    // Tapping the payee no longer navigates or inline-edits: it opens the editor.
    await card(page, "Coffee Shop").getByText("Coffee Shop").click();
    const drawer = page.getByRole("dialog");
    await expect(drawer.getByText("Edit Transaction")).toBeVisible();

    // Each posting's account picker is wide enough to show its account.
    const accounts = drawer.getByRole("combobox");
    await expect(accounts.nth(0)).toHaveText("expenses:dining");
    expect((await accounts.nth(0).boundingBox()).width).toBeGreaterThan(250);

    await drawer.getByLabel("Description").fill("Coffee Shop | oat latte");
    await drawer.getByRole("button", { name: "Save" }).click();

    await expect.poll(() => updates.length).toBe(1);
    expect(updates[0].description).toBe("Coffee Shop | oat latte");
    expect(updates[0].postings[0].comment).toBe("date:2026-03-22, split with Bob");
    await expect(drawer).toBeHidden();
  });

  test("re-picking the selected account keeps it", async ({ page }) => {
    await mockLedgerApi(page, { transactions });
    await page.goto("/#/transactions");

    await card(page, "Coffee Shop").click();
    const account = page.getByRole("dialog").getByRole("combobox").first();
    await account.click();
    await page.getByRole("option", { name: "expenses:dining", exact: true }).click();
    await expect(account).toHaveText("expenses:dining");
  });

  test("the status toggle on a card does not open the editor", async ({ page }) => {
    await mockLedgerApi(page, { transactions });
    const statusCalls = captureRpc(page, "UpdateTransactionStatus");
    await page.goto("/#/transactions");

    const toggle = card(page, "Grocer").locator("button[data-slot=status-mark]");
    expect((await toggle.boundingBox()).width).toBeGreaterThanOrEqual(40);
    await toggle.click();
    await expect.poll(() => statusCalls.length).toBe(1);
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("sidebar New transaction closes the sidebar", async ({ page }) => {
    await mockLedgerApi(page, { transactions });
    await page.goto("/#/transactions");

    await page.getByRole("button", { name: "More" }).click();
    await page.locator('[data-mobile="true"][data-sidebar="sidebar"]').getByRole("button", { name: "New transaction" }).click();
    await expect(page.getByRole("dialog").getByText("Add Transaction").first()).toBeVisible();
    await expect(page.locator('[data-mobile="true"][data-sidebar="sidebar"]')).toHaveCount(0);
  });
});
