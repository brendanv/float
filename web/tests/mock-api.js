// Shared mock data and API interception helpers for screenshot tests.
// The Connect protocol sends POST requests with JSON bodies; responses are
// plain JSON objects matching the proto message shapes.

export const mockAccounts = [
  { name: "assets", fullName: "assets", type: "A", depth: 1 },
  { name: "checking", fullName: "assets:checking", type: "A", depth: 2 },
  { name: "savings", fullName: "assets:savings", type: "A", depth: 2 },
  { name: "liabilities", fullName: "liabilities", type: "L", depth: 1 },
  { name: "creditcard", fullName: "liabilities:creditcard", type: "L", depth: 2 },
  { name: "expenses", fullName: "expenses", type: "C", depth: 1 },
  { name: "groceries", fullName: "expenses:groceries", type: "C", depth: 2 },
  { name: "dining", fullName: "expenses:dining", type: "C", depth: 2 },
  { name: "utilities", fullName: "expenses:utilities", type: "C", depth: 2 },
  { name: "income", fullName: "income", type: "C", depth: 1 },
  { name: "salary", fullName: "income:salary", type: "C", depth: 2 },
];

export const mockBalanceRows = [
  { account: "assets", amounts: [{ commodity: "USD", quantity: "12450.00" }] },
  { account: "liabilities", amounts: [{ commodity: "USD", quantity: "-1230.00" }] },
  { account: "expenses", amounts: [{ commodity: "USD", quantity: "1840.00" }] },
  { account: "income", amounts: [{ commodity: "USD", quantity: "-5200.00" }] },
];

export const mockAccountBalanceRows = [
  { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "8450.00" }] },
  { account: "assets:savings", amounts: [{ commodity: "USD", quantity: "4000.00" }] },
  { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-1230.00" }] },
];

export const mockTransactions = [
  {
    fid: "a1b2c3d4",
    date: "2026-03-25",
    description: "Whole Foods Market",
    postings: [
      { account: "expenses:groceries", amounts: [{ commodity: "$", quantity: "87.43" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "$", quantity: "-87.43" }] },
    ],
    tags: [],
  },
  {
    fid: "b2c3d4e5",
    date: "2026-03-24",
    description: "Monthly Salary",
    postings: [
      { account: "assets:checking", amounts: [{ commodity: "$", quantity: "5200.00" }] },
      { account: "income:salary", amounts: [{ commodity: "$", quantity: "-5200.00" }] },
    ],
    tags: [],
  },
  {
    fid: "c3d4e5f6",
    date: "2026-03-22",
    description: "Chipotle",
    postings: [
      { account: "expenses:dining", amounts: [{ commodity: "$", quantity: "14.75" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "$", quantity: "-14.75" }] },
    ],
    tags: [],
  },
  {
    fid: "d4e5f6g7",
    date: "2026-03-20",
    description: "Electric Bill",
    postings: [
      { account: "expenses:utilities", amounts: [{ commodity: "$", quantity: "95.00" }] },
      { account: "assets:checking", amounts: [{ commodity: "$", quantity: "-95.00" }] },
    ],
    tags: [],
  },
  {
    fid: "e5f6g7h8",
    date: "2026-03-18",
    description: "Trader Joe's",
    postings: [
      { account: "expenses:groceries", amounts: [{ commodity: "$", quantity: "62.18" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "$", quantity: "-62.18" }] },
    ],
    tags: [],
  },
];

/**
 * Intercept all LedgerService Connect RPC calls and return mock data.
 * @param {import('@playwright/test').Page} page
 */
export async function mockLedgerApi(page) {
  await page.route("**/float.v1.LedgerService/**", async (route) => {
    const url = route.request().url();
    const method = url.split("/").pop();

    let body = {};

    let reqBody = {};
    try {
      reqBody = JSON.parse(route.request().postData() || "{}");
    } catch (_) {}

    switch (method) {
      case "ListAccounts":
        body = { accounts: mockAccounts };
        break;
      case "GetBalances":
        body = {
          report: {
            rows: reqBody.depth === 1 ? mockBalanceRows : mockAccountBalanceRows,
          },
        };
        break;
      case "ListTransactions":
        body = { transactions: mockTransactions };
        break;
      case "GetNetWorthTimeseries":
        body = { snapshots: [] };
        break;
      default:
        body = {};
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
}
