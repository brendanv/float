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
├── format.js          # Amount/date/timestamp formatting utilities (use components/timestamp.jsx for RFC 3339 times)
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

The sidebar groups routes (`src/components/nav-items.js` is the single source for the sidebar, section tabs and the ⌘K palette). Grouped routes share the group title as the page `h1` with section tabs under it.

| Route | Component | Nav (group › tab) | Description |
|-------|-----------|-------------------|-------------|
| `/` | `HomePage` | Overview | Net worth hero + sparkline, assets/liabilities waterline, accounts list, month spending, review notice |
| `/transactions` | `TransactionsPage` | Transactions | Day-grouped transaction list / account register, search + filter chips, bulk edit |
| `/add` | `AddTransactionPage` | Transactions | Full-page add transaction form |
| `/trends` | `TrendsPage` | Reports › Net worth | Net worth hero, chart and spending by category |
| `/monthly` | `MonthlyDashboardPage` | Reports › Income & spending | Income statement matrix (sticky account column, pinned Avg/Total) |
| `/portfolio` | `PortfolioPage` | Investments › Holdings | Portfolio value hero, value chart, returns, allocation, holdings |
| `/lots` | `LotsPage` | Investments › Lots | Open lots with acquisition date, units, and cost basis |
| `/prices` | `PricesPage` | Investments › Prices | Commodity prices and Alpha Vantage backfill |
| `/accounts` | `AccountsPage` | Organize › Accounts | Account tree (declared + used): declare, rename, delete, close/reopen |
| `/payees` | `PayeesPage` | Organize › Payees | Payee list and descriptions without a payee |
| `/rules` | `RulesPage` | Organize › Rules | Categorization rules, AI suggestions, issue detection, preview/apply |
| `/templates` | `TemplatesPage` | Organize › Templates | Saved transaction templates |
| `/connections` | `ConnectionsPage` | Bring in › Connections | Stripe Financial Connections linking, refresh, fetch, import |
| `/import` | `ImportPage` | Bring in › Import › Import | CSV import wizard and bank profiles |
| `/imports` | `ImportsHistoryPage` | Bring in › Import › History | Import batches (a row opens `/transactions?importBatchId=`) and original CSVs |
| `/settings` | `SettingsPage` | Settings › General | Timezone, export, Alpha Vantage, Stripe, AI |
| `/assertions` | `BalanceAssertionsPage` | Settings › Assertions | Balance assertion drift status |
| `/snapshots` | `SnapshotsPage` | Settings › Snapshots | Git snapshots, diffs and restores |
| `/hledger-query` | `HledgerQueryPage` | Settings › Query | Raw hledger query and AI query helpers |
| `/logs` | `LogsPage` | Settings › Logs | Live server log stream |
| `/login` | `LoginPage` | — | Passphrase login (rendered without the app shell) |

`TransactionsPage` accepts search params: `account`, `payee`, `importBatchId`, and `search`.

## Design language (Waterline)

The UI follows the Waterline language; the plan, tokens and per-step notes are in [`design/WATERLINE.md`](design/WATERLINE.md) (visual reference: `design/waterline-mockup.html`). Prefer these primitives over one-off styling:

| Primitive | Use it for |
|-----------|------------|
| `Amount` (`components/amount.jsx`) | Every money/commodity amount: sans `tabular-nums`, `size="figure\|display"` shrinks cents, `tone="inflow\|below"`, `signed`, `negate`, `whole`. Cost/lot annotations go on its mono second line. |
| `Account`, `AccountGlyph`, `Flow` (`components/account.jsx`) | Account names: `leaf`/`path` in dense views with the type glyph, `full` (mono) in editors. `accountType()` lives in `lib/accounts.js`. |
| `StatusMark` (`components/status.jsx`) | Transaction status: nothing for Cleared, accent dot for unreviewed, amber ring for Pending. |
| `Notice` (`ui/alert.jsx`, `variant="info\|warning\|critical"`) | One-line messages with an optional action. `ErrorBanner` is the critical Notice for RPC errors. |
| `PageHeader`, `Section`, `MetricHero`, `DeltaChip`, `Sparkline`, `Eyebrow` (`page-header.jsx`, `page.jsx`) | Page title + actions slot; flat sections (heading, hairline, action slot); hero figures. |
| `Segmented`, `SectionTabs`, `FilterChip`/`ChipSelect`, `TagChip` | Small option toggles, group tabs (rendered by the shell), toolbar filters, quiet `#tag` chips. |
| `ui/chart.jsx` helpers | `CHART_STROKE`, `CHART_X_AXIS`/`CHART_Y_AXIS`, `ChartAreaFill`, `chartEndLabel`, `ChartTooltipRow`; animations off. |
| `ROW_ACTION` (`lib/utils.js`) | Hover/focus-revealed row actions (rows carry `group/row`; touch always shows them). |

Do:

- Put content straight on the page: `Section` heading + hairline, spacing between sections. Borders, fills and shadows only for objects (dialogs, popovers, notices, form fields, the selected item).
- Use color only for a fact: accent = action/assets, `inflow` = income arriving, `below` = owed/negative, `warning` = needs a look. Static balances, net worth and transfers stay ink. `--chart-*` only for categories.
- Keep tables dense (~34px rows desktop, ~48px phone), sentence-case labels and headings, `text-label` uppercase table heads.
- Check new UI in light and dark at 1280px and 390px (no horizontal page overflow; the phone tab bar covers the bottom).

Don't:

- Don't use `font-mono` for amounts; mono is for hledger syntax only (queries, regexes, raw account paths in inputs, codes, CSV rules, logs, commodity symbols).
- Don't add up postings or balances in JS. Totals, balancing, cost conversion and valuation come from hledger reports; the UI may pick values out of a report, flip a sign for display, or derive a label from a report's own totals.
- Don't wrap sections in `Card`s or nest cards; use `Card variant="raised"` only for genuine objects.
- Don't use `text-success`/`text-destructive` (or red/green) as decoration, show destructive row actions permanently, or add subtitles/breadcrumbs that repeat the title.

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

After any ledger mutation call `invalidateLedgerQueries(queryClient)` from `query-keys.js`: floatd bumps its cache generation on every write, so any ledger-derived query may be stale, and a hand-picked key list misses pages (with `staleTime: 30s` they would show old data). Keep query-key construction centralized in `query-keys.js`.

Build hledger query tokens from UI values with `src/lib/hledger-query.js`: `payee:`/`desc:`/`tag:` values are regexes, so literal names must go through `escapeRegex`/`exactRegex` (e.g. `SQ *COFFEE`), and `dateRangeQuery` handles open-ended ranges.

`UpdateTransaction` replaces all postings: send every posting's `comment` back (`fieldsFromPosting` → `toPostingInput` carry it) or it is deleted. An empty `comment`/`tags` keeps the existing value; set `replaceComment`/`replaceTags` to clear.

## Adding a New Page

1. Create `src/pages/my-page.jsx`.
2. Add a lazy route and route entry in `src/router.jsx`.
3. Make it reachable: add a sidebar item to `NAV_GROUPS` or a tab to `SECTION_TABS` in `src/components/nav-items.js` (the command palette lists both).
4. Add or extend mocked data in `tests/mock-api.js` and screenshot specs if the page is visual/user-facing.

## shadcn Components

Add components with `bunx shadcn@latest add <component>`, which writes to `src/components/ui/`. Use the `shadcn` skill when composing or repairing shadcn components.

## Touch Targets

On touch devices (`pointer: coarse`) every control must offer a ≥40px hit area; desktop sizing is unchanged. The `components/ui` primitives (Button, Input, NativeSelect, Select, Combobox, Tabs, Checkbox, menu items, Calendar, sidebar) already grow via `pointer-coarse:min-h-10` / `min-w-10`, so prefer them over raw `<button>`s. For a small inline control whose visual size should stay (sort header, chip ✕, tree chevron) add the `touch-target` utility from `style.css`, which extends the hit area with an invisible `::after` (don't combine it with components that already use `::after`, e.g. tabs or checkbox). `tests/touch-targets.spec.js` enforces this on every route; links inside running text are exempt.

The Waterline primitives follow the same rule without changing their look: `Segmented` options and `FilterChip`/`ChipSelect` buttons (and their ✕) use `touch-target`, and the chip row pads itself vertically on coarse pointers so its sideways scroll doesn't clip the extended area. Note that `overflow-x-auto` clips an `::after` vertically too, so inside a scrolling row either make room with padding or grow the control (as `SectionTabs` does with `pointer-coarse:pt-2.5`; its underline is an inset shadow, not `::after`).

## Screenshots / Visual Testing

Playwright specs in `tests/` mock ConnectRPC responses using `tests/mock-api.js`; no live server is required. Desktop, mobile, and feature-specific screenshot specs cover major pages. Use the `web-screenshots` skill or `bun run screenshots` to regenerate. Config lives in `playwright.config.js`.

Specs must import `test`/`expect` from `./fixtures.js`, not `@playwright/test`. The fixture fails a test on any uncaught page error, `console.error` (including React dev warnings), a visible Vite error overlay, or a `LedgerService` RPC that falls through to the `default` case in `mock-api.js`. Fix the cause instead of suppressing it; a test that deliberately triggers an error can use `test.use({ allowedErrors: [/status of 400/] })`.

React is 18: components that receive a ref (anything used as a base-ui `render` element, or as a Radix/vaul child) must use `forwardRef`. base-ui primitives (`Dialog*`, `Tooltip*`, …) take `render={<Button />}`, not `asChild`; vaul's `Drawer*` primitives still use `asChild`.

To render mobile with touch styles use `hasTouch: true, isMobile: true` and take viewport (not `fullPage`) screenshots, or measure before the screenshot: in Chromium a `fullPage` screenshot resets touch emulation, so `(pointer: coarse)` stops matching for the rest of the test.

If the preinstalled Chromium doesn't match the pinned Playwright, set `PW_CHROMIUM_PATH` (e.g. `/opt/pw-browsers/chromium` in Claude Code on the web).
