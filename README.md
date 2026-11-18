# float

float is a self-hostable personal finance manager built on top of [hledger](https://hledger.org/). Track your spending, net worth, and investments from a nice web UI, while all of your data lives in plain-text journal files that you own forever. No proprietary database, no lock-in, and no worrying about what happens to your financial history when some budgeting app shuts down.

![Overview](docs/screenshots/overview.png)

# Features

- Everything is stored as plain-text [hledger](https://hledger.org/) journals, organized into one file per month. Read them, grep them, edit them by hand, or point any other hledger tool at them.
  - Plus, float never does its own accounting math. hledger handles all of the parsing, validation, and reporting, so your journal is always valid hledger.
- Every change is validated with `hledger check` before it's saved, and automatically rolled back if it would break anything.
- Automatic snapshots of your data directory after every change. Browse, diff, and restore old versions right from the UI.
- Import CSVs from your bank with reusable per-bank profiles, preview everything before it hits your journal, and skip duplicates automatically. Imported something by mistake? Undo the whole batch in one click.
  - Plus, optionally connect your bank accounts via Stripe Financial Connections and have new transactions pulled in daily.
- Categorization rules automatically clean up payees and assign accounts and tags as transactions come in, or retroactively across your whole history.
- Reports for net worth over time and monthly income and spending, plus account registers and balance assertions.
- Investment tracking with holdings, lots, cost basis, returns, and allocation. Enter prices by hand or backfill them from Alpha Vantage.
- Fast, keyboard-friendly transaction entry, saved templates for recurring transactions, and bulk editing.
- Bring your own OpenRouter API key to get rule suggestions, find conflicting rules, and ask questions about your finances in plain English.
- Everything the web UI can do is also available via a gRPC/Connect API, so it's easy to script against (or hand to an agent).
- Responsive design that works well on your phone, plus light and dark mode.

<p align="center">
  <img src="docs/screenshots/overview-mobile.png" width="30%">
  &nbsp;&nbsp;
  <img src="docs/screenshots/transactions-mobile.png" width="30%">
</p>

## Screenshots

| | |
|---|---|
| ![Transactions](docs/screenshots/transactions.png) | ![Reports](docs/screenshots/reports.png) |
| ![Investments](docs/screenshots/investments.png) | ![Import](docs/screenshots/import.png) |

# Installation

The easiest way to run float is with Docker. The image bundles hledger, so there's nothing else to install.

```bash
mkdir -p data && touch data/config.toml
docker run -d --name float \
  -p 8080:8080 \
  -v "$PWD/data:/data" \
  -e FLOAT_AUTH_PASSPHRASE=change-me \
  ghcr.io/brendanv/float:latest
```

Then head to `your-server.com:8080` and log in with your passphrase.

> [!WARNING]
> If `FLOAT_AUTH_PASSPHRASE` isn't set, auth is disabled entirely and anyone who can reach the server can see and edit your finances. There's a single shared passphrase and no per-user accounts, so it's best to keep float on your home network or behind a VPN.

If you'd rather build it yourself, install [mise](https://mise.jdx.dev/) and run:

```bash
mise install
mise run build
./floatd --data-dir ./data
```

## Bringing your existing journal

Already using hledger? Just add an `include` for your existing journal in `data/main.journal` and float will pick it up. New transactions that float writes go into monthly files like `data/2026/01.journal`, and get a short code like `(a1b2c3d4)` so float can find them again later. Any metadata float needs is stored in `float-*` tags, which are hidden in the UI.

# Configuration

`config.toml` can start out empty, and most settings (bank profiles, API keys, timezone) can be managed from the Settings and Import pages. A few things are configured via environment variables instead:

- `FLOAT_AUTH_PASSPHRASE`: the passphrase for the web UI and API.
- `STRIPE_SECRET_KEY` and `STRIPE_PUBLISHABLE_KEY`: enables bank connections via Stripe.
- `OPENROUTER_API_KEY` (and optionally `OPENROUTER_MODEL`): enables the AI features.

You'll probably want to set `timezone = "America/New_York"` (or wherever you are) at the top of `config.toml` so that "today" means the same thing to float as it does to you.

# Scripting and the API

float speaks gRPC, gRPC-Web, and Connect on the same port, with server reflection turned on, so tools like `buf curl` and `grpcurl` work without any local protos:

```bash
buf curl --protocol grpc --http2-prior-knowledge \
  --header "Authorization: Bearer $FLOAT_AUTH_PASSPHRASE" \
  --reflect-header "Authorization: Bearer $FLOAT_AUTH_PASSPHRASE" \
  http://localhost:8080/float.v1.LedgerService/GetBalances --data '{}'
```

If you're writing a script (or pointing an agent at float) to turn some export into transactions, use `ImportStructuredTransactions`. It validates the whole batch up front, supports dry runs, skips anything that's already in your journal so re-running it is safe, and the result can be undone just like a CSV import.

There's also `floatctl`, an admin CLI that operates on the data directory directly, which is handy for debugging, poking at config, or restoring snapshots without the server running. Run `go run ./cmd/floatctl help` to see everything it can do.

## Contributing

Contributions are welcome but no guarantees that they'll be accepted - I mostly built float for myself so I'm somewhat opinionated on how it should evolve :)

To get a dev environment going, run `mise run floatd` and `mise run web-dev` side by side. `mise run check-all` runs lint and tests, and `mise run ci` runs everything CI does. The screenshots above are generated from mocked data via Playwright (`cd web && bash capture-screenshots.sh`), so you don't need real financial data to work on the UI. Architecture notes live in [CLAUDE.md](CLAUDE.md).
