# internal/alphavantage

Fetches historical market prices from the Alpha Vantage API. Used by the `LedgerService.FetchPrices` RPC to bulk-import commodity prices into `prices.journal`.

## API

- `NewClient(apiKey)` — creates a client with a 15-second HTTP timeout.
- `FetchWeeklyPrices(ctx, symbol, startDate, endDate)` — fetches weekly closing prices for `symbol` (e.g. `"AAPL"`) in the date range `[startDate, endDate]` inclusive. Dates are `"YYYY-MM-DD"`. Results are sorted ascending by date.

## Response

Returns `[]WeeklyPrice`, each with:
- `Date` — week-ending date (`"YYYY-MM-DD"`)
- `Close` — closing price as a string (e.g. `"188.63"`)
- `Currency` — currently hardcoded to `"$"` (Alpha Vantage doesn't return currency in the weekly endpoint)

## Notes

Alpha Vantage free tier has rate limits (25 req/day). Returns an error if the response contains no data, which typically means an invalid symbol or exceeded rate limit.

The API key is configured in `config.toml` (not yet exposed in the config schema — check `internal/config/` for the current field name).
