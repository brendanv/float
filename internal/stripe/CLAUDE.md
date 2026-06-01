# internal/stripe

Thin wrapper around Stripe Financial Connections using `github.com/stripe/stripe-go/v82`. Functions are stateless and take credentials as parameters; no package-level Stripe state is stored here.

## Functions

- `CreateCustomer(ctx, secretKey)` — create a Stripe customer for Financial Connections linking.
- `CreateFCSession(ctx, secretKey, customerID)` — create a Financial Connections session and return the `client_secret` used by Stripe.js.
- `ListSessionAccounts(ctx, secretKey, sessionID)` — retrieve accounts from a completed FC session. The current API flow mostly links by listing customer accounts rather than trusting a session ID.
- `ListAccounts(ctx, secretKey, customerID)` — list FC accounts for a customer; filters are handled by callers.
- `SubscribeTransactions(ctx, secretKey, accountID)` — enable transaction data for an account.
- `RefreshTransactions(ctx, secretKey, accountID)` — request an asynchronous transaction refresh.
- `MaybeRefreshTransactions(ctx, logger, secretKey, accountID)` — inspect account refresh state and either start a refresh, join a pending refresh, skip when unavailable, or report throttling with `NextRefreshAvailableAt`.
- `WaitForRefresh(ctx, logger, secretKey, accountID)` — poll until transaction refresh succeeds/fails or times out; returns the refresh ID.
- `WaitForRefreshWithProgress(ctx, logger, secretKey, accountID, onProgress)` — same as `WaitForRefresh`, but emits `RefreshProgress` events for streaming RPCs.
- `DisconnectAccount(ctx, secretKey, accountID)` — revoke access and disconnect the account.
- `ListTransactions(ctx, secretKey, accountID)` — return all available account transactions using Stripe pagination.

## Types

- `Account` — ID, display name, institution, last4, status, and next transaction refresh availability.
- `Transaction` — ID, account ID, amount in cents (positive credit, negative debit), currency, description, transaction/posting times, status (`posted` or `pending`).
- `RefreshKickoff` — status enum plus refresh ID and throttle timestamp for user-triggered refresh flows.
- `RefreshProgress` — status/attempt/elapsed metadata used by streaming refresh RPCs.

## Notes

- `ListAccounts` and `ListTransactions` use Stripe iterators, so pagination is internal to this package.
- Callers are responsible for hiding disconnected accounts, filtering pending transactions, deduplicating by Stripe transaction ID, mapping signs to hledger postings, and persisting linked-account state in config.
- Stripe's `next_refresh_available_at` throttle is surfaced instead of treated as an error so UIs can explain when a manual refresh may run again.
