export const queryKeys = {
  accounts:             () => ["accounts"],
  balances:             (params) => ["balances", params],
  transactions:         (params) => ["transactions", params],
  accountRegister:      (params) => ["accountRegister", params],
  tags:                 () => ["tags"],
  lots:                 (query) => ["lots", query],
  prices:               () => ["prices"],
  accountDeclarations:  () => ["accountDeclarations"],
  balanceAssertionStatus: () => ["balanceAssertionStatus"],
  unmatchedTransfers:   () => ["unmatchedTransfers"],
  snapshots:            () => ["snapshots"],
  snapshotDiff:         (hash) => ["snapshotDiff", hash],
  bankProfiles:         () => ["bankProfiles"],
  rules:                () => ["rules"],
  netWorthTimeseries:              (begin) => ["netWorthTimeseries", begin],
  incomeStatementTimeseries:       (begin, end) => ["incomeStatementTimeseries", begin, end],
  imports:              () => ["imports"],
  importedTransactions: (batchId) => ["importedTransactions", batchId],
  payees:               () => ["payees"],
  noPayeeTransactions:  () => ["noPayeeTransactions"],
  portfolioHoldings:    (accountPrefix = "") => ["portfolioHoldings", accountPrefix],
  portfolioTimeseries:  (accountPrefix = "", begin = "") => ["portfolioTimeseries", accountPrefix, begin],
  portfolioSettings:    () => ["portfolioSettings"],
  alphaVantageConfig:   () => ["alphaVantageConfig"],
  aiConfig:             () => ["aiConfig"],
  generalConfig:        () => ["generalConfig"],
  stripeConfig:         () => ["stripeConfig"],
  stripeLinkedAccounts: () => ["stripeLinkedAccounts"],
  templates:            () => ["templates"],
  unreviewedCount:      () => ["unreviewedCount"],
  newestUnreviewed:     (count) => ["newestUnreviewed", count],
};

// Every successful floatd write bumps its cache generation, so any ledger-derived
// query (balances, registers, payees, timeseries, portfolio, ...) may now be
// stale. Mirror that after a mutation: invalidate everything, so active queries
// refetch now and inactive ones refetch on their next mount.
export function invalidateLedgerQueries(queryClient) {
  return queryClient.invalidateQueries();
}
