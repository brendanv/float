# internal/alphavantage

Fetches historical market prices from the Alpha Vantage API. The ledger handler uses this package in the `BackfillPrices` RPC to bulk-create hledger `P` price directives in `prices.journal`.

## API

- `NewClient(apiKey)` — creates a client with a 15-second HTTP timeout.
- `FetchWeeklyPrices(ctx, symbol, startDate, endDate)` — fetches weekly adjusted closing prices for `symbol` in the inclusive date range `[startDate, endDate]`. Dates are strings in `YYYY-MM-DD` format. Results are sorted ascending by date.

## Response

Returns `[]WeeklyPrice`:

- `Date` — week-ending date (`YYYY-MM-DD`).
- `Close` — closing price as a string, suitable for writing to `prices.journal`.
- `Currency` — currently `USD`; Alpha Vantage's weekly endpoint does not return a per-symbol currency.

## Configuration / Integration

The API key is stored in `config.toml` under `[alpha_vantage].api_key` and can be viewed/updated through `GetAlphaVantageConfig` / `SetAlphaVantageApiKey`. `BackfillPrices` fetches prices, skips dates already present for the commodity, writes new prices inside `txlock.Do()`, and returns the written directives plus a skipped count.

## Notes

Alpha Vantage free-tier limits are low. If the response has no time-series data, this package returns an error that may indicate an invalid symbol, an exhausted quota, or an upstream API response shape change.
