# web/

React + Vite web UI for float. Built assets are emitted to `../internal/webui/dist/` and embedded in `floatd`.

## Tech Stack

- React 18.
- TanStack Router with hash-based routing (`createHashHistory`).
- TanStack React Query for data fetching/caching.
- TanStack React Form for forms.
- TanStack React Table for transaction tables.
- ConnectRPC (`@connectrpc/connect-web`) for `LedgerService` calls to `window.location.origin`.
- TailwindCSS v4 plus shadcn/ui components configured by `components.json`.
- Playwright 1.56.1 for mocked screenshot tests — **do not upgrade** without matching system Chromium.

## Commands

```bash
cd web
bun install           # install dependencies
bun run dev           # Vite dev server on :5173 (proxies /float.v1.LedgerService → :8080)
bun run build         # build → ../internal/webui/dist/ (also: mise run web-build from root)
bun run screenshots   # capture Playwright screenshots with mocked API data
```

During development run `mise run floatd` concurrently with `bun run dev` unless using screenshot tests, which mock the API and do not need `floatd`.

## Structure

```text
src/
├── client.js          # ConnectRPC transport + ledgerClient export
├── router.jsx         # TanStack Router route tree with hash history
├── query-keys.js      # Centralized React Query key factory
├── format.js          # Amount/date/number formatting utilities
├── main.jsx           # React entry point
├── style.css          # Global styles + Tailwind base/theme
├── components/
│   ├── ui/            # shadcn/ui components
│   └── *.jsx          # shared app components and tables/forms
├── hooks/
│   ├── use-mobile.js  # responsive breakpoint detection
│   └── use-cube.js    # loads the cube for the current generation
├── lib/
│   ├── utils.js       # cn() helper (clsx + tailwind-merge)
│   ├── cube.js        # fetch + decode the dashboard cube (typed-array views)
│   ├── cube-query.js  # client-side aggregation over the cube
│   └── generation.js  # tracks the server's txlock generation
└── pages/             # one file per route
```

## Pages / Routes

| Route | Component | Description |
|-------|-----------|-------------|
| `/` | `HomePage` | Dashboard summary |
| `/trends` | `TrendsPage` | Net worth chart (reads the cube, no RPCs) |
| `/monthly` | `MonthlyDashboardPage` | Income statement / revenue and expense dashboard |
| `/transactions` | `TransactionsPage` | Searchable, pageable transaction list |
| `/portfolio` | `PortfolioPage` | Holdings, allocation, market value, cost basis/gain |
| `/accounts` | `AccountsPage` | Account declarations/tree and register |
| `/payees` | `PayeesPage` | Payee list and filtering entry point |
| `/rules` | `RulesPage` | Categorization rules editor, AI rule suggestions, and AI duplicate/contradiction detection |
| `/connections` | `ConnectionsPage` | Stripe Financial Connections linking, refresh, fetch, import |
| `/import` | `ImportPage` | CSV import wizard and bank profile rules content |
| `/prices` | `PricesPage` | Commodity prices and Alpha Vantage backfill |
| `/assertions` | `BalanceAssertionsPage` | Balance assertion drift status |
| `/imports` | `ImportsHistoryPage` | Import batches and original CSV retrieval |
| `/snapshots` | `SnapshotsPage` | Git snapshots and diffs/restores |
| `/settings` | `SettingsPage` | General timezone, AI, Alpha Vantage, Stripe settings |
| `/hledger-query` | `HledgerQueryPage` | Raw hledger query/debug UI and AI query helpers |
| `/logs` | `LogsPage` | Live server log stream |
| `/login` | `LoginPage` | Passphrase login (rendered without the app shell) |

`TransactionsPage` accepts search params: `account`, `payee`, `importBatchId`, and `search`.

## API Access

`src/client.js` exports `ledgerClient`, a typed ConnectRPC client for `LedgerService`. The transport includes an interceptor that redirects to `#/login` when the server returns `unauthenticated` (auth is the `float_session` cookie set by `POST /api/login`; the Vite dev proxy forwards `/api` to floatd). Use the client from React Query with keys from `src/query-keys.js`:

```jsx
import { ledgerClient } from "@/client.js";
import { useQuery } from "@tanstack/react-query";
import { queryKeys } from "@/query-keys.js";

const { data } = useQuery({
  queryKey: queryKeys.balances(),
  queryFn: () => ledgerClient.getBalances({}),
});
```

Invalidate the relevant query keys after mutations. Keep query-key construction centralized in `query-keys.js`.

## The dashboard cube

Dashboard pages can read a precomputed snapshot of the ledger instead of calling
RPCs. Every hledger invocation costs the same multi-second journal parse
regardless of query size, so a page built on RPCs pays it again on every filter
change; the cube is fetched once per generation and sliced in the browser in
well under a millisecond.

```jsx
import { useCube } from "@/hooks/use-cube.js";
import { flowByAccount, balanceAt, latestPeriod } from "@/lib/cube-query.js";

const { data: cube, isLoading, error } = useCube();
const rows = useMemo(
  () => (cube ? flowByAccount(cube, { type: "X", from: begin }, 2) : []),
  [cube, begin],
);
```

Everything derived from the cube belongs in `useMemo` keyed on the cube and the
filters — never in a query.

**Flows sum over time; stocks do not.** `flowSums` / `flowSeries` /
`flowByAccount` aggregate posting amounts and are safe over any date range and
account subtree. `balanceAt` / `balanceSeries` read market value and cost basis,
which are materialized per period end: they may be rolled up the account tree at
a fixed period but never summed across periods, and `assertSummableOverTime`
throws rather than let that happen quietly.

Two details that bite:

- Date ranges are **half-open**, `[from, to)`, matching hledger's `date:A..B`.
- `account` filters match a subtree (the account and its descendants), not a
  substring, and `type` filters use hledger's letters with Cash counting as an
  Asset.

The cube is served from `/api/cube/{generation}.bin` and is not part of
`LedgerService`, so **screenshot tests mock it separately** — `mock-api.js`
serves `tests/fixtures/cube.bin`, a payload `internal/cube` really encoded.
`tests/cube-query.unit.spec.js` decodes that same fixture, which makes it a
cross-language contract test; run it with `bun run test:unit` (no browser, no
dev server).

## Adding a New Page

1. Create `src/pages/my-page.jsx`.
2. Add a lazy route and route entry in `src/router.jsx`.
3. Add a navigation item in `src/components/app-shell.jsx` if the page should be globally reachable.
4. Add or extend mocked data in `tests/mock-api.js` and screenshot specs if the page is visual/user-facing.

## shadcn Components

Add components with `bunx shadcn@latest add <component>`, which writes to `src/components/ui/`. Use the `shadcn` skill when composing or repairing shadcn components.

## Screenshots / Visual Testing

Playwright specs in `tests/` mock ConnectRPC responses using `tests/mock-api.js`; no live server is required. Desktop, mobile, and feature-specific screenshot specs cover major pages. Use the `web-screenshots` skill or `bun run screenshots` to regenerate. Config lives in `playwright.config.js`.
