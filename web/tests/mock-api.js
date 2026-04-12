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
  { fullName: "assets", displayName: "assets", amounts: [{ commodity: "USD", quantity: "12450.00" }] },
  { fullName: "liabilities", displayName: "liabilities", amounts: [{ commodity: "USD", quantity: "-1230.00" }] },
  { fullName: "expenses", displayName: "expenses", amounts: [{ commodity: "USD", quantity: "1840.00" }] },
  { fullName: "income", displayName: "income", amounts: [{ commodity: "USD", quantity: "-5200.00" }] },
];

export const mockAccountBalanceRows = [
  { fullName: "assets:checking", displayName: "checking", amounts: [{ commodity: "USD", quantity: "8450.00" }] },
  { fullName: "assets:savings", displayName: "savings", amounts: [{ commodity: "USD", quantity: "4000.00" }] },
  { fullName: "liabilities:creditcard", displayName: "creditcard", amounts: [{ commodity: "USD", quantity: "-1230.00" }] },
];

export const mockExpenseBalanceRows = [
  { fullName: "expenses:groceries", displayName: "groceries", amounts: [{ commodity: "USD", quantity: "450.00" }] },
  { fullName: "expenses:dining", displayName: "dining", amounts: [{ commodity: "USD", quantity: "210.00" }] },
  { fullName: "expenses:utilities", displayName: "utilities", amounts: [{ commodity: "USD", quantity: "95.00" }] },
];

export const mockRevenueBalanceRows = [
  { fullName: "income:salary", displayName: "salary", amounts: [{ commodity: "USD", quantity: "5200.00" }] },
];

export const mockTransactions = [
  {
    fid: "a1b2c3d4",
    date: "2026-03-25",
    description: "Whole Foods Market | weekly groceries",
    payee: "Whole Foods Market",
    note: "weekly groceries",
    status: "Pending",
    postings: [
      { account: "expenses:groceries", amounts: [{ commodity: "$", quantity: "87.43" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "$", quantity: "-87.43" }] },
    ],
    tags: { reimbursable: "" },
  },
  {
    fid: "a1b2c3d5",
    date: "2026-03-25",
    description: "Amazon | desk lamp",
    payee: "Amazon",
    note: "desk lamp",
    status: "Pending",
    postings: [
      { account: "expenses:shopping", amounts: [{ commodity: "$", quantity: "34.99" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "$", quantity: "-34.99" }] },
    ],
    tags: { project: "home-office", reimbursable: "" },
  },
  {
    fid: "b2c3d4e5",
    date: "2026-03-24",
    description: "Acme Corp | March salary",
    payee: "Acme Corp",
    note: "March salary",
    status: "Cleared",
    postings: [
      { account: "assets:checking", amounts: [{ commodity: "$", quantity: "5200.00" }] },
      { account: "income:salary", amounts: [{ commodity: "$", quantity: "-5200.00" }] },
    ],
    tags: {},
  },
  {
    fid: "c3d4e5f6",
    date: "2026-03-22",
    description: "Chipotle | lunch",
    payee: "Chipotle",
    note: "lunch",
    status: "Pending",
    postings: [
      { account: "expenses:dining", amounts: [{ commodity: "$", quantity: "14.75" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "$", quantity: "-14.75" }] },
    ],
    tags: {},
  },
  {
    fid: "c3d4e5f7",
    date: "2026-03-22",
    description: "Starbucks | morning coffee",
    payee: "Starbucks",
    note: "morning coffee",
    status: "Cleared",
    postings: [
      { account: "expenses:dining", amounts: [{ commodity: "$", quantity: "6.50" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "$", quantity: "-6.50" }] },
    ],
    tags: {},
  },
  {
    fid: "c3d4e5f8",
    date: "2026-03-22",
    description: "Metro Transit",
    status: "Cleared",
    postings: [
      { account: "expenses:transport", amounts: [{ commodity: "$", quantity: "3.25" }] },
      { account: "assets:checking", amounts: [{ commodity: "$", quantity: "-3.25" }] },
    ],
    tags: {},
  },
  {
    fid: "d4e5f6g7",
    date: "2026-03-20",
    description: "Electric Bill",
    status: "Cleared",
    postings: [
      { account: "expenses:utilities", amounts: [{ commodity: "$", quantity: "95.00" }] },
      { account: "assets:checking", amounts: [{ commodity: "$", quantity: "-95.00" }] },
    ],
    tags: {},
  },
  {
    fid: "e5f6g7h8",
    date: "2026-03-18",
    description: "Whole Foods Market | produce run",
    payee: "Whole Foods Market",
    note: "produce run",
    status: "Pending",
    postings: [
      { account: "expenses:groceries", amounts: [{ commodity: "$", quantity: "62.18" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "$", quantity: "-62.18" }] },
    ],
    tags: {},
  },
];

export const mockPrices = [
  { pid: "a1b2c3d4", date: "2026-01-02", commodity: "AAPL", price: { commodity: "USD", quantity: "182.63" } },
  { pid: "b2c3d4e5", date: "2026-01-02", commodity: "MSFT", price: { commodity: "USD", quantity: "425.22" } },
  { pid: "c3d4e5f6", date: "2026-02-03", commodity: "AAPL", price: { commodity: "USD", quantity: "188.44" } },
  { pid: "d4e5f6a7", date: "2026-02-03", commodity: "MSFT", price: { commodity: "USD", quantity: "415.10" } },
  { pid: "e5f6a7b8", date: "2026-03-01", commodity: "AAPL", price: { commodity: "USD", quantity: "178.50" } },
  { pid: "f6a7b8c9", date: "2026-03-01", commodity: "MSFT", price: { commodity: "USD", quantity: "398.75" } },
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

export const mockBankProfiles = [
  { name: "Chase Checking", rulesFile: "rules/chase.rules" },
  { name: "Capital One Visa", rulesFile: "rules/capitalone.rules" },
];

export const mockRules = [
  { id: "aabb1122", pattern: "AMAZON|amazon\\.com", payee: "Amazon", account: "expenses:shopping", tags: {}, priority: 5 },
  { id: "ccdd3344", pattern: "STARBUCKS|starbucks", payee: "Starbucks", account: "expenses:dining", tags: { category: "coffee" }, priority: 10 },
  { id: "eeff5566", pattern: "^(WHOLE FOODS|Whole Foods)", payee: "Whole Foods Market", account: "expenses:groceries", tags: {}, priority: 15 },
  { id: "aabb7788", pattern: "NETFLIX", payee: "Netflix", account: "expenses:subscriptions", tags: { auto: "yes" }, priority: 20 },
];

export const mockImportCandidates = [
  {
    transaction: {
      fid: "",
      date: "2026-03-28",
      description: "AMAZON.COM PURCHASE",
      postings: [
        { account: "assets:checking", amounts: [{ commodity: "$", quantity: "-42.99" }] },
        { account: "expenses:unknown", amounts: [{ commodity: "$", quantity: "42.99" }] },
      ],
      tags: {},
    },
    isDuplicate: false,
    matchedRuleId: "aabb1122",
  },
  {
    transaction: {
      fid: "",
      date: "2026-03-27",
      description: "STARBUCKS #4821",
      postings: [
        { account: "assets:checking", amounts: [{ commodity: "$", quantity: "-6.75" }] },
        { account: "expenses:unknown", amounts: [{ commodity: "$", quantity: "6.75" }] },
      ],
      tags: {},
    },
    isDuplicate: false,
    matchedRuleId: "ccdd3344",
  },
  {
    transaction: {
      fid: "",
      date: "2026-03-26",
      description: "Whole Foods Market",
      postings: [
        { account: "assets:checking", amounts: [{ commodity: "$", quantity: "-87.43" }] },
        { account: "expenses:groceries", amounts: [{ commodity: "$", quantity: "87.43" }] },
      ],
      tags: {},
    },
    isDuplicate: true,
    matchedRuleId: "eeff5566",
  },
  {
    transaction: {
      fid: "",
      date: "2026-03-25",
      description: "MONTHLY GAS BILL",
      postings: [
        { account: "assets:checking", amounts: [{ commodity: "$", quantity: "-84.00" }] },
        { account: "expenses:unknown", amounts: [{ commodity: "$", quantity: "84.00" }] },
      ],
      tags: {},
    },
    isDuplicate: false,
    matchedRuleId: "",
  },
];

export const mockApplyPreviews = [
  {
    fid: "a1b2c3d5",
    description: "AMAZON.COM purchase",
    matchedRuleId: "aabb1122",
    currentAccount: "expenses:shopping",
    newAccount: "",
    currentPayee: "Amazon",
    newPayee: "",
    addTags: {},
  },
  {
    fid: "c3d4e5f7",
    description: "Starbucks morning coffee",
    matchedRuleId: "ccdd3344",
    currentAccount: "expenses:dining",
    newAccount: "",
    currentPayee: "",
    newPayee: "Starbucks",
    addTags: { category: "coffee" },
  },
  {
    fid: "e5f6g7h8",
    description: "Whole Foods Market produce run",
    matchedRuleId: "eeff5566",
    currentAccount: "expenses:unknown",
    newAccount: "expenses:groceries",
    currentPayee: "",
    newPayee: "Whole Foods Market",
    addTags: {},
  },
];
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
      case "ListTags":
        body = { tags: ["category", "memo", "reimbursable"] };
        break;
      case "GetBalances": {
        const query = reqBody.query || [];
        const isExpense = query.includes("type:X");
        const isRevenue = query.includes("type:R");
        let rows;
        if (isExpense) rows = mockExpenseBalanceRows;
        else if (isRevenue) rows = mockRevenueBalanceRows;
        else rows = reqBody.depth === 1 ? mockBalanceRows : mockAccountBalanceRows;
        body = { report: { rows } };
        break;
      }
      case "ListTransactions": {
        let txs = mockTransactions;
        const query = reqBody.query || [];
        for (const token of query) {
          if (token.startsWith("payee:")) {
            const payeeFilter = token.slice("payee:".length).toLowerCase();
            txs = txs.filter((tx) => tx.payee && tx.payee.toLowerCase().includes(payeeFilter));
          }
          if (token.startsWith("acct:")) {
            const acctFilter = token.slice("acct:".length).toLowerCase();
            txs = txs.filter((tx) => tx.postings && tx.postings.some((p) => p.account.toLowerCase().includes(acctFilter)));
          }
        }
        body = { transactions: txs };
        break;
      }
      case "GetNetWorthTimeseries":
        body = { snapshots: mockNetWorthSnapshots };
        break;
      case "UpdateTransactionStatus":
        body = {};
        break;
      case "UpdateTransaction":
        body = {};
        break;
      case "ModifyTags":
        body = {};
        break;
      case "BulkEditTransactions":
        body = { transactions: [] };
        break;
      case "ListBankProfiles":
        body = { profiles: mockBankProfiles };
        break;
      case "CreateBankProfile":
        body = { profile: { name: reqBody.name, rulesFile: reqBody.rulesFile } };
        break;
      case "PreviewImport":
        body = { candidates: mockImportCandidates };
        break;
      case "ImportTransactions":
        body = { importedCount: 3, transactions: [] };
        break;
      case "ListRules":
        body = { rules: mockRules };
        break;
      case "AddRule":
        body = { rule: { id: "new00001", pattern: reqBody.pattern, payee: reqBody.payee, account: reqBody.account, tags: reqBody.tags || {}, priority: reqBody.priority || 0 } };
        break;
      case "UpdateRule":
        body = { rule: { id: reqBody.id, pattern: reqBody.pattern, payee: reqBody.payee, account: reqBody.account, tags: reqBody.tags || {}, priority: reqBody.priority || 0 } };
        break;
      case "DeleteRule":
        body = {};
        break;
      case "PreviewApplyRules":
        body = { previews: mockApplyPreviews };
        break;
      case "ApplyRules":
        body = { appliedCount: 3 };
        break;
      case "ListPrices":
        body = { prices: mockPrices };
        break;
      case "AddPrice":
        body = { price: { pid: "new00001", date: reqBody.date || "2026-03-28", commodity: reqBody.commodity, price: { commodity: reqBody.currency, quantity: reqBody.quantity } } };
        break;
      case "DeletePrice":
        body = {};
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
