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
  { name: "investments", fullName: "assets:investments", type: "A", depth: 2 },
  { name: "aapl", fullName: "assets:investments:aapl", type: "A", depth: 3 },
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
    stripeTransactionId: "txn_3OxKLM2eZvKYlo2C0abc1234",
    postings: [
      { account: "expenses:groceries", amounts: [{ commodity: "USD", quantity: "87.43" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-87.43" }] },
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
    stripeTransactionId: "txn_3OxKLM2eZvKYlo2C0def5678",
    postings: [
      { account: "expenses:shopping", amounts: [{ commodity: "USD", quantity: "34.99" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-34.99" }] },
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
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "5200.00" }] },
      { account: "income:salary", amounts: [{ commodity: "USD", quantity: "-5200.00" }] },
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
      { account: "expenses:dining", amounts: [{ commodity: "USD", quantity: "14.75" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-14.75" }] },
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
      { account: "expenses:dining", amounts: [{ commodity: "USD", quantity: "6.50" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-6.50" }] },
    ],
    tags: {},
  },
  {
    fid: "c3d4e5f8",
    date: "2026-03-22",
    description: "Metro Transit",
    status: "Cleared",
    postings: [
      { account: "expenses:transport", amounts: [{ commodity: "USD", quantity: "3.25" }] },
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-3.25" }] },
    ],
    tags: {},
  },
  {
    fid: "d4e5f6g7",
    date: "2026-03-20",
    description: "Electric Bill",
    status: "Cleared",
    postings: [
      { account: "expenses:utilities", amounts: [{ commodity: "USD", quantity: "95.00" }] },
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-95.00" }] },
    ],
    tags: {},
  },
  {
    fid: "o5p6q7r8",
    date: "2026-03-19",
    description: "Fidelity | buy AAPL",
    payee: "Fidelity",
    note: "buy AAPL",
    status: "Cleared",
    postings: [
      {
        account: "assets:investments:aapl",
        amounts: [{ commodity: "AAPL", quantity: "10", cost: { commodity: "USD", quantity: "175.00", isTotal: false } }],
      },
      {
        account: "assets:checking",
        amounts: [{ commodity: "USD", quantity: "-1750.00" }],
        balanceAssertion: { amount: { commodity: "USD", quantity: "5250.00" } },
      },
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
      { account: "expenses:groceries", amounts: [{ commodity: "USD", quantity: "62.18" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-62.18" }] },
    ],
    tags: {},
  },
  {
    fid: "f6g7h8i9",
    date: "2026-03-15",
    description: "Rent Payment",
    status: "Cleared",
    postings: [
      { account: "expenses:rent", amounts: [{ commodity: "USD", quantity: "1500.00" }] },
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-1500.00" }] },
    ],
    tags: {},
  },
  {
    fid: "g7h8i9j0",
    date: "2026-03-14",
    description: "Netflix",
    status: "Cleared",
    postings: [
      { account: "expenses:subscriptions", amounts: [{ commodity: "USD", quantity: "17.99" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-17.99" }] },
    ],
    tags: { auto: "yes" },
  },
  {
    fid: "h8i9j0k1",
    date: "2026-03-12",
    description: "Gas Station",
    status: "Cleared",
    postings: [
      { account: "expenses:transport", amounts: [{ commodity: "USD", quantity: "54.20" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-54.20" }] },
    ],
    tags: {},
  },
  {
    fid: "i9j0k1l2",
    date: "2026-03-10",
    description: "Target | household supplies",
    payee: "Target",
    note: "household supplies",
    status: "Cleared",
    postings: [
      { account: "expenses:household", amounts: [{ commodity: "USD", quantity: "43.57" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-43.57" }] },
    ],
    tags: { reimbursable: "" },
  },
  {
    fid: "j0k1l2m3",
    date: "2026-03-08",
    description: "Spotify",
    status: "Cleared",
    postings: [
      { account: "expenses:subscriptions", amounts: [{ commodity: "USD", quantity: "10.99" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-10.99" }] },
    ],
    tags: { auto: "yes" },
  },
  {
    fid: "k1l2m3n4",
    date: "2026-03-07",
    description: "Chipotle | team lunch",
    payee: "Chipotle",
    note: "team lunch",
    status: "Cleared",
    postings: [
      { account: "expenses:dining", amounts: [{ commodity: "USD", quantity: "38.50" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-38.50" }] },
    ],
    tags: { reimbursable: "" },
  },
  {
    fid: "l2m3n4o5",
    date: "2026-03-05",
    description: "Internet Bill",
    status: "Cleared",
    postings: [
      { account: "expenses:utilities", amounts: [{ commodity: "USD", quantity: "79.99" }] },
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-79.99" }] },
    ],
    tags: {},
  },
  {
    fid: "m3n4o5p6",
    date: "2026-03-03",
    description: "Whole Foods Market | weekly shop",
    payee: "Whole Foods Market",
    note: "weekly shop",
    status: "Pending",
    postings: [
      { account: "expenses:groceries", amounts: [{ commodity: "USD", quantity: "91.33" }] },
      { account: "liabilities:creditcard", amounts: [{ commodity: "USD", quantity: "-91.33" }] },
    ],
    tags: {},
  },
  {
    fid: "n4o5p6q7",
    date: "2026-03-01",
    description: "Phone Bill",
    status: "Cleared",
    postings: [
      { account: "expenses:utilities", amounts: [{ commodity: "USD", quantity: "45.00" }] },
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-45.00" }] },
    ],
    tags: {},
  },
];

export const mockPrices = [
  { pid: "a1b2c3d4", date: "2026-01-02", commodity: "AAPL", price: { commodity: "USD", quantity: "182.63" } },
  { pid: "b2c3d4e5", date: "2026-01-02", commodity: "MSFT", price: { commodity: "USD", quantity: "425.22" } },
  { pid: "b2c3d4e6", date: "2026-01-02", commodity: "GOOG", price: { commodity: "USD", quantity: "193.45" } },
  { pid: "c3d4e5f6", date: "2026-02-03", commodity: "AAPL", price: { commodity: "USD", quantity: "188.44" } },
  { pid: "d4e5f6a7", date: "2026-02-03", commodity: "MSFT", price: { commodity: "USD", quantity: "415.10" } },
  { pid: "d4e5f6a8", date: "2026-02-03", commodity: "GOOG", price: { commodity: "USD", quantity: "197.82" } },
  { pid: "e5f6a7b8", date: "2026-03-01", commodity: "AAPL", price: { commodity: "USD", quantity: "178.50" } },
  { pid: "f6a7b8c9", date: "2026-03-01", commodity: "MSFT", price: { commodity: "USD", quantity: "398.75" } },
  { pid: "f6a7b8ca", date: "2026-03-01", commodity: "GOOG", price: { commodity: "USD", quantity: "185.20" } },
  { pid: "g7b8c9d1", date: "2026-03-15", commodity: "AAPL", price: { commodity: "USD", quantity: "181.10" } },
  { pid: "g7b8c9d2", date: "2026-03-15", commodity: "MSFT", price: { commodity: "USD", quantity: "402.30" } },
  { pid: "g7b8c9d3", date: "2026-03-15", commodity: "GOOG", price: { commodity: "USD", quantity: "187.65" } },
  { pid: "h8c9d1e2", date: "2026-04-01", commodity: "AAPL", price: { commodity: "USD", quantity: "175.90" } },
  { pid: "h8c9d1e3", date: "2026-04-01", commodity: "MSFT", price: { commodity: "USD", quantity: "391.44" } },
  { pid: "h8c9d1e4", date: "2026-04-01", commodity: "GOOG", price: { commodity: "USD", quantity: "180.11" } },
  { pid: "i9d1e2f3", date: "2026-04-15", commodity: "AAPL", price: { commodity: "USD", quantity: "172.30" } },
  { pid: "i9d1e2f4", date: "2026-04-15", commodity: "MSFT", price: { commodity: "USD", quantity: "385.60" } },
  { pid: "i9d1e2f5", date: "2026-04-15", commodity: "GOOG", price: { commodity: "USD", quantity: "176.40" } },
  { pid: "j1e2f3g4", date: "2026-05-01", commodity: "AAPL", price: { commodity: "USD", quantity: "179.88" } },
  { pid: "j1e2f3g5", date: "2026-05-01", commodity: "MSFT", price: { commodity: "USD", quantity: "412.77" } },
  { pid: "j1e2f3g6", date: "2026-05-01", commodity: "GOOG", price: { commodity: "USD", quantity: "184.32" } },
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

export const mockAccountRegisterRows = [
  {
    fid: "b2c3d4e5",
    date: "2026-03-24",
    description: "Acme Corp | March salary",
    payee: "Acme Corp",
    note: "March salary",
    status: "Cleared",
    otherAccounts: ["income:salary"],
    change: [{ commodity: "USD", quantity: "5200.00" }],
    runningTotal: [{ commodity: "USD", quantity: "5200.00" }],
    tags: {},
  },
  {
    fid: "c3d4e5f8",
    date: "2026-03-22",
    description: "Metro Transit",
    status: "Cleared",
    stripeTransactionId: "txn_3OxKLM2eZvKYlo2C0ghi9012",
    otherAccounts: ["expenses:transport"],
    change: [{ commodity: "USD", quantity: "-3.25" }],
    runningTotal: [{ commodity: "USD", quantity: "5196.75" }],
    tags: { reimbursable: "" },
  },
  {
    fid: "d4e5f6g7",
    date: "2026-03-20",
    description: "Electric Bill",
    status: "Cleared",
    otherAccounts: ["expenses:utilities"],
    change: [{ commodity: "USD", quantity: "-95.00" }],
    runningTotal: [{ commodity: "USD", quantity: "5101.75" }],
    tags: {},
  },
  {
    fid: "e5f6g7h9",
    date: "2026-03-15",
    description: "Grocery Store",
    status: "Cleared",
    otherAccounts: ["expenses:groceries"],
    change: [{ commodity: "USD", quantity: "-62.18" }],
    runningTotal: [{ commodity: "USD", quantity: "5039.57" }],
    tags: { category: "food", reimbursable: "" },
  },
  {
    fid: "f6g7h8i9",
    date: "2026-03-10",
    description: "Rent Payment",
    status: "Cleared",
    otherAccounts: ["expenses:rent"],
    change: [{ commodity: "USD", quantity: "-1500.00" }],
    runningTotal: [{ commodity: "USD", quantity: "3539.57" }],
    tags: {},
  },
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
        { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-42.99" }] },
        { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "42.99" }] },
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
        { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-6.75" }] },
        { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "6.75" }] },
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
        { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-87.43" }] },
        { account: "expenses:groceries", amounts: [{ commodity: "USD", quantity: "87.43" }] },
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
        { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-84.00" }] },
        { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "84.00" }] },
      ],
      tags: {},
    },
    isDuplicate: false,
    matchedRuleId: "",
  },
];

export const mockAccountDeclarations = [
  { aid: "a1b2c3d4", name: "assets:checking", hasPostings: true },
  { aid: "a1b2c3d5", name: "assets:savings", hasPostings: true },
  { aid: "b2c3d4e5", name: "expenses:dining", hasPostings: true },
  { aid: "c3d4e5f6", name: "expenses:groceries", hasPostings: true },
  { aid: "d4e5f6a7", name: "expenses:household", hasPostings: false },
  { aid: "e5f6a7b8", name: "expenses:rent", hasPostings: true },
  { aid: "f6a7b8c9", name: "expenses:shopping", hasPostings: true },
  { aid: "g7b8c9d0", name: "expenses:subscriptions", hasPostings: false },
  { aid: "h8c9d0e1", name: "expenses:transport", hasPostings: true },
  { aid: "i9d0e1f2", name: "expenses:utilities", hasPostings: false },
  { aid: "l1a2b3c4", name: "expenses:housing:mortgage:principal", hasPostings: true },
  { aid: "m2b3c4d5", name: "expenses:housing:mortgage:interest", hasPostings: true },
  { aid: "n3c4d5e6", name: "expenses:housing:insurance", hasPostings: false },
  { aid: "j0e1f2a3", name: "income:salary", hasPostings: true },
  { aid: "k1f2a3b4", name: "liabilities:creditcard", hasPostings: true },
];

export const mockPortfolioHoldings = {
  holdings: [
    {
      account: "assets:investments:aapl",
      symbol: "AAPL",
      quantity: "10",
      latestPrice: { commodity: "USD", quantity: "178.50" },
      currentValue: { commodity: "USD", quantity: "1785.00" },
      portfolioPct: 46.4,
      priceDate: "2026-03-01",
      bookValue: { commodity: "USD", quantity: "1750.00" },
      unrealizedGain: { commodity: "USD", quantity: "35.00" },
      unrealizedGainPct: 2.0,
    },
    {
      account: "assets:investments:msft",
      symbol: "MSFT",
      quantity: "5",
      latestPrice: { commodity: "USD", quantity: "398.75" },
      currentValue: { commodity: "USD", quantity: "1993.75" },
      portfolioPct: 51.8,
      priceDate: "2026-03-01",
      bookValue: { commodity: "USD", quantity: "2000.00" },
      unrealizedGain: { commodity: "USD", quantity: "-6.25" },
      unrealizedGainPct: -0.31,
    },
    {
      account: "assets:investments:voo",
      symbol: "VOO",
      quantity: "2",
      latestPrice: null,
      currentValue: null,
      portfolioPct: 0,
      priceDate: "",
      bookValue: null,
      unrealizedGain: null,
      unrealizedGainPct: 0,
    },
  ],
  totalValue: { commodity: "USD", quantity: "3778.75" },
  asOfDate: "2026-03-01",
};

export const mockPortfolioTimeseries = {
  snapshots: [
    { date: "2026-01-01", totalValue: { commodity: "USD", quantity: "1785.00" } },
    { date: "2026-02-01", totalValue: { commodity: "USD", quantity: "3650.00" } },
    { date: "2026-03-01", totalValue: { commodity: "USD", quantity: "3778.75" } },
  ],
};

export const mockImports = [
  { importBatchId: "2026-03-28-a1b2c3d4", date: "2026-03-28", transactionCount: 3 },
  { importBatchId: "2026-03-15-b2c3d4e5", date: "2026-03-15", transactionCount: 5 },
  { importBatchId: "2026-02-28-c3d4e5f6", date: "2026-02-28", transactionCount: 8 },
  { importBatchId: "2026-01-31-d4e5f6a7", date: "2026-01-31", transactionCount: 12 },
];

export const mockSnapshots = [
  {
    hash: "9f3a1c4b8e2d7a6f5b4c3d2e1a0b9c8d7e6f5a40",
    message: "float: edit transaction (a1b2c3d4)",
    timestamp: "2026-04-29T10:14:08-04:00",
  },
  {
    hash: "7c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c01",
    message: "float: add transaction (e5f6a7b8)",
    timestamp: "2026-04-28T16:42:31-04:00",
  },
  {
    hash: "4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a30",
    message: "float: import 2026-04-15-b2c3d4e5",
    timestamp: "2026-04-15T09:03:55-04:00",
  },
  {
    hash: "1f2e3d4c5b6a7980abcdef0123456789abcdef01",
    message: "float: init",
    timestamp: "2026-01-02T12:00:00-04:00",
  },
];

export const mockSnapshotDiffs = {
  "9f3a1c4b8e2d7a6f5b4c3d2e1a0b9c8d7e6f5a40": {
    hash: "9f3a1c4b8e2d7a6f5b4c3d2e1a0b9c8d7e6f5a40",
    files: [
      {
        path: "2026/04.journal",
        oldPath: "",
        changeType: "modified",
        isBinary: false,
        patch: `diff --git a/2026/04.journal b/2026/04.journal
index 4a58007052a65fbc..ad116abbb07be5c0 100644
--- a/2026/04.journal
+++ b/2026/04.journal
@@ -12,9 +12,9 @@
 2026-04-22 (e5f6a7b8) BLUE BOTTLE COFFEE
     expenses:food:coffee     5.75 USD
     assets:checking         -5.75 USD

-2026-04-25 (a1b2c3d4) AMAZON.COM PURCHASE
-    expenses:shopping       42.99 USD
-    assets:checking        -42.99 USD
+2026-04-25 (a1b2c3d4) AMAZON.COM PURCHASE | books
+    expenses:books          42.99 USD
+    assets:checking        -42.99 USD

 2026-04-27 (c3d4e5f6) WHOLE FOODS MARKET
     expenses:food:groceries  87.50 USD
`,
      },
    ],
  },
  "4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a30": {
    hash: "4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a30",
    files: [
      {
        path: "2026/04.journal",
        oldPath: "",
        changeType: "added",
        isBinary: false,
        patch: `diff --git a/2026/04.journal b/2026/04.journal
new file mode 100644
index 0000000000000000..4a58007052a65fbc
--- /dev/null
+++ b/2026/04.journal
@@ -0,0 +1,5 @@
+2026-04-15 (b2c3d4e5) ACME CORP DIRECT DEPOSIT
+    income:salary       -2500.00 USD
+    assets:checking      2500.00 USD
+
+; imported from 2026-04-15-b2c3d4e5
`,
      },
      {
        path: "rules.json",
        oldPath: "",
        changeType: "modified",
        isBinary: false,
        patch: `diff --git a/rules.json b/rules.json
index abc123..def456 100644
--- a/rules.json
+++ b/rules.json
@@ -3,6 +3,11 @@
       "pattern": "AMAZON",
       "account": "expenses:shopping",
       "priority": 10
+    },
+    {
+      "pattern": "ACME CORP",
+      "account": "income:salary",
+      "priority": 5
     }
   ]
 }
`,
      },
    ],
  },
};

export const mockImportedTransactions = [
  {
    fid: "a1b2c3d4",
    date: "2026-03-28",
    description: "AMAZON.COM PURCHASE",
    postings: [
      { account: "expenses:shopping", amounts: [{ commodity: "USD", quantity: "42.99" }] },
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-42.99" }] },
    ],
    tags: {},
    status: "Pending",
  },
  {
    fid: "a1b2c3d5",
    date: "2026-03-28",
    description: "STARBUCKS #4821",
    postings: [
      { account: "expenses:dining", amounts: [{ commodity: "USD", quantity: "6.75" }] },
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-6.75" }] },
    ],
    tags: {},
    status: "Pending",
  },
  {
    fid: "a1b2c3d6",
    date: "2026-03-27",
    description: "MONTHLY GAS BILL",
    postings: [
      { account: "expenses:utilities", amounts: [{ commodity: "USD", quantity: "84.00" }] },
      { account: "assets:checking", amounts: [{ commodity: "USD", quantity: "-84.00" }] },
    ],
    tags: {},
    status: "Pending",
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

export const mockStripeLinkedAccounts = [
  {
    stripeAccountId: "fca_chase_abc123",
    hledgerAccount: "assets:checking:chase",
    displayName: "Chase Checking ****1234",
    lastFetchedAt: "2026-05-10T00:00:00Z",
  },
  {
    stripeAccountId: "fca_bofa_xyz789",
    hledgerAccount: "assets:savings:bofa",
    displayName: "Bank of America Savings ****5678",
    lastFetchedAt: "2026-05-09T00:00:00Z",
  },
];

export const mockStripeImportCandidates = [
  {
    transaction: {
      fid: "",
      date: "2026-05-10",
      description: "STARBUCKS #1234",
      postings: [
        { account: "assets:checking:chase", amounts: [{ commodity: "USD", quantity: "-6.75" }] },
        { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "6.75" }] },
      ],
      tags: {},
    },
    isDuplicate: false,
    matchedRuleId: "ccdd3344",
  },
  {
    transaction: {
      fid: "",
      date: "2026-05-09",
      description: "WHOLE FOODS MARKET",
      postings: [
        { account: "assets:checking:chase", amounts: [{ commodity: "USD", quantity: "-87.43" }] },
        { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "87.43" }] },
      ],
      tags: {},
    },
    isDuplicate: true,
    matchedRuleId: "eeff5566",
  },
  {
    transaction: {
      fid: "",
      date: "2026-05-08",
      description: "ELECTRIC BILL",
      postings: [
        { account: "assets:checking:chase", amounts: [{ commodity: "USD", quantity: "-95.00" }] },
        { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "95.00" }] },
      ],
      tags: {},
    },
    isDuplicate: false,
    matchedRuleId: "",
  },
];

function makeAmountList(quantity) {
  return { amounts: [{ commodity: "USD", quantity: String(quantity) }] };
}

const MOCK_IS_PERIODS = [
  "2025-04-01","2025-05-01","2025-06-01","2025-07-01",
  "2025-08-01","2025-09-01","2025-10-01","2025-11-01",
  "2025-12-01","2026-01-01","2026-02-01","2026-03-01",
];

export const mockIncomeStatementTimeseries = {
  periods: MOCK_IS_PERIODS,
  rows: [
    // Revenues section
    {
      displayName: "salary", fullName: "income:salary", indent: 1,
      section: "Revenues", isTotal: false,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("-5200.00")),
      totalAmounts: [{ commodity: "USD", quantity: "-62400.00" }],
    },
    {
      displayName: "Total Revenues", fullName: "", indent: 0,
      section: "Revenues", isTotal: true,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("-5200.00")),
      totalAmounts: [],
    },
    // Expenses section
    {
      displayName: "expenses", fullName: "expenses", indent: 0,
      section: "Expenses", isTotal: false,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("2340.43")),
      totalAmounts: [{ commodity: "USD", quantity: "28085.16" }],
    },
    {
      displayName: "groceries", fullName: "expenses:groceries", indent: 1,
      section: "Expenses", isTotal: false,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("450.00")),
      totalAmounts: [{ commodity: "USD", quantity: "5400.00" }],
    },
    {
      displayName: "dining", fullName: "expenses:dining", indent: 1,
      section: "Expenses", isTotal: false,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("210.00")),
      totalAmounts: [{ commodity: "USD", quantity: "2520.00" }],
    },
    {
      displayName: "utilities", fullName: "expenses:utilities", indent: 1,
      section: "Expenses", isTotal: false,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("95.00")),
      totalAmounts: [{ commodity: "USD", quantity: "1140.00" }],
    },
    {
      displayName: "rent", fullName: "expenses:rent", indent: 1,
      section: "Expenses", isTotal: false,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("1500.00")),
      totalAmounts: [{ commodity: "USD", quantity: "18000.00" }],
    },
    {
      displayName: "subscriptions", fullName: "expenses:subscriptions", indent: 1,
      section: "Expenses", isTotal: false,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("57.43")),
      totalAmounts: [{ commodity: "USD", quantity: "689.16" }],
    },
    {
      displayName: "transport", fullName: "expenses:transport", indent: 1,
      section: "Expenses", isTotal: false,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("28.00")),
      totalAmounts: [{ commodity: "USD", quantity: "336.00" }],
    },
    {
      displayName: "Total Expenses", fullName: "", indent: 0,
      section: "Expenses", isTotal: true,
      perPeriodAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("2340.43")),
      totalAmounts: [],
    },
  ],
  netAmounts: MOCK_IS_PERIODS.map(() => makeAmountList("2859.57")),
};

export async function mockLedgerApi(page, { accountRegisterRows, accountDeclarations, portfolioHoldings, stripeEnabled = true } = {}) {
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
      case "ListPayees": {
        const allPayees = [...new Set(mockTransactions.filter((tx) => tx.payee).map((tx) => tx.payee))].sort();
        body = { payees: allPayees };
        break;
      }
      case "ListTransactions": {
        let txs = mockTransactions;
        const query = reqBody.query || [];
        for (const token of query) {
          if (token === "not:desc:.*[|].*") {
            txs = txs.filter((tx) => !tx.payee);
          } else if (token.startsWith("payee:")) {
            const payeeFilter = token.slice("payee:".length).toLowerCase();
            txs = txs.filter((tx) => tx.payee && tx.payee.toLowerCase().includes(payeeFilter));
          } else if (token.startsWith("code:")) {
            const fidFilter = token.slice("code:".length).toLowerCase();
            txs = txs.filter((tx) => tx.fid && tx.fid.toLowerCase().startsWith(fidFilter));
          }
          if (token.startsWith("acct:")) {
            const acctFilter = token.slice("acct:".length).toLowerCase();
            txs = txs.filter((tx) => tx.postings && tx.postings.some((p) => p.account.toLowerCase().includes(acctFilter)));
          }
        }
        body = { transactions: txs };
        break;
      }
      case "GetAccountRegister":
        body = { rows: accountRegisterRows ?? mockAccountRegisterRows, total: (accountRegisterRows ?? mockAccountRegisterRows).length, hasNext: false };
        break;
      case "GetNetWorthTimeseries":
        body = { snapshots: mockNetWorthSnapshots };
        break;
      case "GetIncomeStatementTimeseries":
        body = mockIncomeStatementTimeseries;
        break;
      case "UpdateTransactionStatus":
        body = {};
        break;
      case "UpdateTransaction":
        body = {};
        break;
      case "DeleteTransaction":
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
      case "GetBankProfileContent":
        body = {
          rulesFile: mockBankProfiles.find((p) => p.name === reqBody.name)?.rulesFile ?? "rules/chase.rules",
          rulesContent: btoa("# hledger CSV import rules\nskip 1\nfields date, description, amount\naccount1 assets:checking\n"),
        };
        break;
      case "UpdateBankProfile":
        body = { profile: { name: reqBody.newName || reqBody.name, rulesFile: "rules/chase.rules" } };
        break;
      case "DeleteBankProfile":
        body = {};
        break;
      case "PreviewImport":
        body = { candidates: mockImportCandidates };
        break;
      case "ImportTransactions":
        body = { importedCount: 3, transactions: [] };
        break;
      case "ListImports":
        body = { imports: mockImports };
        break;
      case "GetImportedTransactions":
        body = { transactions: mockImportedTransactions, total: mockImportedTransactions.length, hasNext: false };
        break;
      case "GetImportFile":
        body = {
          csvContent: btoa("Date,Description,Amount\n2026-03-28,AMAZON.COM PURCHASE,-42.99\n2026-03-28,STARBUCKS #4821,-6.75\n2026-03-27,MONTHLY GAS BILL,-84.00\n"),
          filename: (reqBody.importBatchId || "2026-03-28-a1b2c3d4") + ".csv",
        };
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
      case "AskQuestion":
        body = {
          hledgerArgs: "balance expenses:groceries date:lastmonth",
          answer: "You spent $450.00 on groceries last month. This includes $287.34 at Whole Foods Market and $162.66 at other grocery stores.",
          rawOutput: "               $450.00  expenses:groceries\n--------------------\n               $450.00",
          querySuccess: true,
        };
        break;
      case "SuggestRules":
        body = {
          suggestions: [
            {
              pattern: "AMAZON|amazon\\.com",
              payee: "Amazon",
              account: "expenses:shopping",
              tags: {},
              reasoning: "Multiple transactions with 'AMAZON' in the description, all categorized inconsistently. A single rule would normalize these.",
              exampleFids: ["a1b2c3d4", "e5f6g7h8", "i9j0k1l2"],
            },
            {
              pattern: "STARBUCKS|SBUX",
              payee: "Starbucks",
              account: "expenses:food:coffee",
              tags: {},
              reasoning: "Recurring Starbucks purchases appearing as unreviewed. Clear merchant pattern with consistent spend category.",
              exampleFids: ["m3n4o5p6", "q7r8s9t0"],
            },
            {
              pattern: "WHOLEFDS|WHOLE FOODS",
              payee: "Whole Foods",
              account: "expenses:food:groceries",
              tags: {},
              reasoning: "Whole Foods purchases appear regularly. Pattern covers both POS description variants.",
              exampleFids: ["u1v2w3x4"],
            },
          ],
        };
        break;
      case "ListAccountDeclarations":
        body = { declarations: accountDeclarations ?? mockAccountDeclarations };
        break;
      case "DeclareAccount":
        body = { declaration: { aid: "new00001", name: reqBody.name } };
        break;
      case "DeleteAccountDeclaration":
        body = {};
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
      case "BackfillPrices":
        body = {
          prices: [
            { pid: "", date: "2026-01-06", commodity: reqBody.commodity || "AAPL", price: { commodity: reqBody.currency || "USD", quantity: "185.00" } },
            { pid: "", date: "2026-01-13", commodity: reqBody.commodity || "AAPL", price: { commodity: reqBody.currency || "USD", quantity: "183.50" } },
          ],
          skippedCount: 1,
        };
        break;
      case "GetPortfolioHoldings":
        body = portfolioHoldings ?? mockPortfolioHoldings;
        break;
      case "GetPortfolioTimeseries":
        body = mockPortfolioTimeseries;
        break;
      case "GetStripeConfig":
        body = stripeEnabled
          ? { enabled: true, publishableKey: "pk_test_mock_key", linkedAccountCount: mockStripeLinkedAccounts.length, customerId: "cus_mock1234567890", dailyImportEnabled: true, lastDailyImportAt: "2026-05-15T08:30:00Z" }
          : { enabled: false, publishableKey: "", linkedAccountCount: 0, customerId: "", dailyImportEnabled: false, lastDailyImportAt: "" };
        break;
      case "SetStripeCustomerId":
        body = {};
        break;
      case "SetStripeDailyImportEnabled":
        body = {};
        break;
      case "CreateStripeLinkSession":
        body = { clientSecret: "fcsess_mock_secret_test" };
        break;
      case "CompleteStripeLinking":
        body = { linkedAccounts: mockStripeLinkedAccounts };
        break;
      case "ListStripeLinkedAccounts":
        body = { accounts: mockStripeLinkedAccounts };
        break;
      case "UnlinkStripeAccount":
        body = {};
        break;
      case "FetchStripeTransactions":
        body = { candidates: mockStripeImportCandidates };
        break;
      case "FetchAllStripeTransactions":
        body = {
          accountCandidates: [
            {
              account: mockStripeLinkedAccounts[0],
              candidates: mockStripeImportCandidates,
            },
            {
              account: mockStripeLinkedAccounts[1],
              candidates: [
                {
                  transaction: {
                    fid: "",
                    date: "2026-05-11",
                    description: "DIRECT DEPOSIT PAYROLL",
                    postings: [
                      { account: "assets:savings:bofa", amounts: [{ commodity: "USD", quantity: "2500.00" }] },
                      { account: "expenses:unknown", amounts: [{ commodity: "USD", quantity: "-2500.00" }] },
                    ],
                    tags: {},
                  },
                  isDuplicate: false,
                  matchedRuleId: "",
                },
              ],
            },
          ],
        };
        break;
      case "ImportStripeTransactions":
        body = { importedCount: 2, transactions: [] };
        break;
      case "ImportAllStripeTransactions":
        body = { importedCount: 3, transactions: [] };
        break;
      case "UpdateStripeAccountLastFetchedAt":
        body = {};
        break;
      case "GetAlphaVantageConfig":
        body = { apiKeyConfigured: true, apiKeyPreview: "ABCD..." };
        break;
      case "GetAIConfig":
        body = { model: "anthropic/claude-sonnet-4-6", effectiveModel: "anthropic/claude-sonnet-4-6", prompt: "My accounts use kebab-case. Groceries go under expenses:food:groceries." };
        break;
      case "ListSnapshots":
        body = { snapshots: mockSnapshots };
        break;
      case "GetSnapshotDiff": {
        const reqHash = reqBody.hash || "";
        body = mockSnapshotDiffs[reqHash] ?? { hash: reqHash, files: [] };
        break;
      }
      case "RestoreSnapshot":
        body = {};
        break;
      case "RunHledgerQuery":
        body = {
          stdout: `              $  12,450.00  assets
              $   8,450.00    checking
              $   4,000.00    savings
             $  -1,230.00  liabilities
             $  -1,230.00    creditcard
--------------------
              $  11,220.00`,
          stderr: "",
          success: true,
          commandLine: "hledger -f /data/main.journal bal --depth 2",
        };
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
