# internal/stripe

Thin wrapper around the Stripe Financial Connections API (`github.com/stripe/stripe-go/v82`). All functions are stateless and take credentials as parameters — no global state.

## Functions

- `CreateFCSession(ctx, secretKey, accountID)` — creates a Financial Connections session; returns `client_secret` for the Stripe.js frontend linking flow
- `ListSessionAccounts(ctx, secretKey, sessionID)` — retrieves accounts from a completed FC session
- `ListAccounts(ctx, secretKey, accountID)` — lists all FC accounts under the given Stripe account holder; pass `""` for `accountID` to list all
- `SubscribeTransactions(ctx, secretKey, accountID)` — enables transaction data for an account (must be called before `ListTransactions` works)
- `RefreshTransactions(ctx, secretKey, accountID)` — requests a fresh transaction sync from Stripe (asynchronous; poll with `WaitForRefresh`)
- `WaitForRefresh(ctx, logger, secretKey, accountID)` — polls until the account's `transaction_refresh` reaches `succeeded`/`failed`; returns the refresh ID. Pass `nil` logger to use `slog.Default()`. Exponential backoff capped at 30s; 5-minute timeout. Returns `("", nil)` immediately if no refresh is in progress.
- `WaitForRefreshWithProgress(ctx, logger, secretKey, accountID, onProgress)` — same as `WaitForRefresh` but invokes `onProgress(RefreshProgress)` at start, on each pending poll, and on terminal state. Use this to drive a streaming RPC.
- `GetTransactionRefreshID(ctx, secretKey, accountID)` — returns the account's current `transaction_refresh.id`; `""` if none.
- `DisconnectAccount(ctx, secretKey, accountID)` — revokes access and disconnects the account
- `ListTransactions(ctx, secretKey, accountID, afterRefreshID)` — returns transactions for an account; pass `""` for `afterRefreshID` to fetch all available history, or a refresh ID to only return transactions captured by that refresh or later.

## Types

- `Account` — ID, DisplayName, Institution, Last4, Status (`"active"`, `"inactive"`, `"disconnected"`)
- `Transaction` — ID, AccountID, AmountCents (positive = credit, negative = debit), Currency, Description, TransactedAt, Status (`"posted"` or `"pending"`)

## Notes

- `ListAccounts` and `ListTransactions` use the Stripe iterator — pagination is handled internally
- Callers are responsible for filtering out `disconnected` accounts from `ListAccounts` results
- `AmountCents` follows Stripe's sign convention: positive values are credits to the account, negative are debits
