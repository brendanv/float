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

export const mockNetWorthSnapshots = [
  { date: "2025-04-01", assets: [{ commodity: "USD", quantity: "9200.00" }], liabilities: [{ commodity: "USD", quantity: "-1100.00" }], netWorth: [{ commodity: "USD", quantity: "8100.00" }] },
  { date: "2025-05-01", assets: [{ commodity: "USD", quantity: "9450.00" }], liabilities: [{ commodity: "USD", quantity: "-1050.00" }], netWorth: [{ commodity: "USD", quantity: "8400.00" }] },
  { date: "2025-06-01", assets: [{ commodity: "USD", quantity: "9600.00" }], liabilities: [{ commodity: "USD", quantity: "-1000.00" }], netWorth: [{ commodity: "USD", quantity: "8600.00" }] },
  { date: "2025-07-01", assets: [{ commodity: "USD", quantity: "9800.00" }], liabilities: [{ commodity: "USD", quantity: "-980.00" }], netWorth: [{ commodity: "USD", quantity: "8820.00" }] },
  { date: "2025-08-01", assets: [{ commodity: "USD", quantity: "10100.00" }], liabilities: [{ commodity: "USD", quantity: "-950.00" }], netWorth: [{ commodity: "USD", quantity: "9150.00" }] },
  { date: "2025-09-01", assets: [{ commodity: "USD", quantity: "10350.00" }], liabilities: [{ commodity: "USD", quantity: "-920.00" }], netWorth: [{ commodity: "USD", quantity: "9430.00" }] },
  { date: "2025-10-01", assets: [{ commodity: "USD", quantity: "10600.00" }], liabilities: [{ commodity: "USD", quantity: "-900.00" }], netWorth: [{ commodity: "USD", quantity: "9700.00" }] },
  { date: "2025-11-01", assets: [{ commodity: "USD", quantity: "10850.00" }], liabilities: [{ commodity: "USD", quantity: "-870.00" }], netWorth: [{ commodity: "USD", quantity: "9980.00" }] },
  { date: "2025-12-01", assets: [{ commodity: "USD", quantity: "11100.00" }], liabilities: [{ commodity: "USD", quantity: "-840.00" }], netWorth: [{ commodity: "USD", quantity: "10260.00" }] },
  { date: "2026-01-01", assets: [{ commodity: "USD", quantity: "11500.00" }], liabilities: [{ commodity: "USD", quantity: "-1230.00" }], netWorth: [{ commodity: "USD", quantity: "10270.00" }] },
  { date: "2026-02-01", assets: [{ commodity: "USD", quantity: "11800.00" }], liabilities: [{ commodity: "USD", quantity: "-1230.00" }], netWorth: [{ commodity: "USD", quantity: "10570.00" }] },
  { date: "2026-03-01", assets: [{ commodity: "USD", quantity: "12450.00" }], liabilities: [{ commodity: "USD", quantity: "-1230.00" }], netWorth: [{ commodity: "USD", quantity: "11220.00" }] },
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
        body = { snapshots: mockNetWorthSnapshots };
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
