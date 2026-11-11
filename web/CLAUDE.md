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
bun run test          # run every Playwright spec (also: mise run web-test from root)
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
│   └── use-mobile.js  # responsive breakpoint detection
├── lib/
│   └── utils.js       # cn() helper (clsx + tailwind-merge)
└── pages/             # one file per route
```

## Pages / Routes

| Route | Component | Description |
|-------|-----------|-------------|
| `/` | `HomePage` | Dashboard summary |
| `/trends` | `TrendsPage` | Net worth chart |
| `/monthly` | `MonthlyDashboardPage` | Income statement / revenue and expense dashboard |
| `/transactions` | `TransactionsPage` | Searchable, pageable transaction list |
| `/portfolio` | `PortfolioPage` | Holdings, allocation, market value, cost basis/gain |
| `/accounts` | `AccountsPage` | Account tree (declared + used accounts): declare, rename, delete, close/reopen |
| `/payees` | `PayeesPage` | Payee list and filtering entry point |
| `/rules` | `RulesPage` | Categorization rules editor, AI rule suggestions, and AI duplicate/contradiction detection |
| `/connections` | `ConnectionsPage` | Stripe Financial Connections linking, refresh, fetch, import |
| `/import` | `ImportPage` | CSV import wizard and bank profile rules content |
| `/lots` | `LotsPage` | Open lots with acquisition date, units, and cost basis |
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

## Adding a New Page

1. Create `src/pages/my-page.jsx`.
2. Add a lazy route and route entry in `src/router.jsx`.
3. Add a navigation item in `src/components/app-shell.jsx` if the page should be globally reachable.
4. Add or extend mocked data in `tests/mock-api.js` and screenshot specs if the page is visual/user-facing.

## shadcn Components

Add components with `bunx shadcn@latest add <component>`, which writes to `src/components/ui/`. Use the `shadcn` skill when composing or repairing shadcn components.

## hledger Charts

Prefer hledger's own SVG charts over hand-rolled ones where hledger has a fitting chart. `src/components/hledger-chart.jsx` exports `useHledgerChart(params)` (calls `GetChart`) and `HledgerChartSvg`, which inlines the server-sanitized SVG. `style.css` (`.hledger-chart-host`) maps the chart's background/text/grid onto theme tokens and gives it a minimum width with horizontal scroll on narrow screens; series colours are hledger's. Playwright mocks serve real chart SVGs from `tests/fixtures/chart-*.svg`, generated from `tests/fixtures/charts.journal` by `go run ./scripts/gen-chart-fixtures` (rerun after hledger upgrades).

## Screenshots / Visual Testing

Playwright specs in `tests/` mock ConnectRPC responses using `tests/mock-api.js`; no live server is required. Desktop, mobile, and feature-specific screenshot specs cover major pages. Use the `web-screenshots` skill or `bun run screenshots` to regenerate. Config lives in `playwright.config.js`.

Specs must import `test`/`expect` from `./fixtures.js`, not `@playwright/test`. The fixture fails a test on any uncaught page error, `console.error` (including React dev warnings), a visible Vite error overlay, or a `LedgerService` RPC that falls through to the `default` case in `mock-api.js`. Fix the cause instead of suppressing it; a test that deliberately triggers an error can use `test.use({ allowedErrors: [/status of 400/] })`.

React is 18: components that receive a ref (anything used as a base-ui `render` element, or as a Radix/vaul child) must use `forwardRef`. base-ui primitives (`Dialog*`, `Tooltip*`, …) take `render={<Button />}`, not `asChild`; vaul's `Drawer*` primitives still use `asChild`.

If the preinstalled Chromium doesn't match the pinned Playwright, set `PW_CHROMIUM_PATH` (e.g. `/opt/pw-browsers/chromium` in Claude Code on the web).
