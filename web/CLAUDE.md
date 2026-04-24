# web/

React + Vite web UI for float. Built assets are embedded in `floatd` via `internal/webui/`.

## Tech Stack

- **React 18** with **TanStack Router** (hash-based routing via `createHashHistory`)
- **TanStack React Query** for data fetching and caching
- **TanStack React Form** for form state management
- **TanStack React Table** for the transactions table
- **ConnectRPC** (`@connectrpc/connect-web`) — talks to `floatd` at `window.location.origin`
- **TailwindCSS v4** + **shadcn/ui** components (configured via `components.json`)
- **Playwright 1.56.1** for screenshot tests — **do not upgrade** without matching system Chromium

## Commands

```bash
cd web
bun install           # install dependencies
bun run dev           # Vite dev server on :5173 (proxies /float.v1.LedgerService → :8080)
bun run build         # build → ../internal/webui/dist/ (also: mise run web-build from root)
bun run screenshots   # capture Playwright screenshots (no live floatd needed)
```

During development run `mise run floatd` concurrently with `bun run dev`.

## Structure

```
src/
├── client.js          # ConnectRPC transport + ledgerClient export
├── router.jsx         # TanStack Router: route tree with hash history
├── query-keys.js      # Centralized React Query key factory
├── format.js          # Formatting utilities (amounts, dates, etc.)
├── main.jsx           # React entry point
├── style.css          # Global styles + Tailwind base
├── components/
│   ├── ui/            # shadcn/ui components (button, dialog, table, etc.)
│   └── *.jsx          # Shared components (AppShell, TransactionTable, etc.)
├── hooks/
│   └── use-mobile.js  # Responsive breakpoint detection
├── lib/
│   └── utils.js       # cn() helper (clsx + tailwind-merge)
└── pages/             # One file per route
```

## Pages / Routes

| Route | Component | Description |
|-------|-----------|-------------|
| `/` | `HomePage` | Balance summary dashboard |
| `/transactions` | `TransactionsPage` | Searchable transaction list |
| `/add` | `AddTransactionPage` | Add transaction form |
| `/accounts` | `AccountsPage` | Account tree with register |
| `/trends` | `TrendsPage` | Net worth chart (lazy-loaded) |
| `/prices` | `PricesPage` | Commodity price management |
| `/import` | `ImportPage` | CSV import wizard |
| `/imports` | `ImportsHistoryPage` | Past import batches |
| `/rules` | `RulesPage` | Auto-categorization rules editor |
| `/snapshots` | `SnapshotsPage` | Journal snapshot management |

`TransactionsPage` accepts search params: `account`, `payee`, `importBatchId`.

## API Access

`src/client.js` exports a single `ledgerClient` — a typed ConnectRPC client for `LedgerService`. Use it directly in components via React Query:

```jsx
import { ledgerClient } from "@/client.js";
import { useQuery } from "@tanstack/react-query";
import { queryKeys } from "@/query-keys.js";

const { data } = useQuery({
  queryKey: queryKeys.balances(),
  queryFn: () => ledgerClient.getBalances({}),
});
```

## Adding a New Page

1. Create `src/pages/my-page.jsx`
2. Add a route in `src/router.jsx`
3. Add a nav link in `src/components/app-shell.jsx`

## shadcn Components

Add via: `bunx shadcn@latest add <component>` — outputs to `src/components/ui/`. See the `shadcn` skill for help composing components.

## Screenshots / Visual Testing

Playwright tests in `tests/` capture screenshots with fully mocked API responses (`tests/mock-api.js`) — no `floatd` required. Two specs: `screenshots.spec.js` (desktop) and `screenshots-mobile.spec.js` (mobile).

Use the `web-screenshots` skill or `bun run screenshots` to regenerate. Config in `playwright.config.js`.
