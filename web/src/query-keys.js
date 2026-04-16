export const queryKeys = {
  accounts:           () => ["accounts"],
  balances:           (params) => ["balances", params],
  transactions:       (params) => ["transactions", params],
  accountRegister:    (params) => ["accountRegister", params],
  tags:               () => ["tags"],
  prices:             () => ["prices"],
  snapshots:          () => ["snapshots"],
  bankProfiles:       () => ["bankProfiles"],
  rules:              () => ["rules"],
  netWorthTimeseries: (begin) => ["netWorthTimeseries", begin],
};
