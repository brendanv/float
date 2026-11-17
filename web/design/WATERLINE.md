# Waterline: web UI redesign plan

Waterline is a new visual language for the float web UI. It changes the visual layer only: tokens, type, a few shared primitives, the app shell and page layouts. **The feature set, routes, RPC usage and data behavior must not change.**

- Visual reference: [`waterline-mockup.html`](./waterline-mockup.html). It's a self-contained page, so open it in a browser or render it with Playwright. Sections, in order: diagnosis, principles, Overview mockup, Transactions mockup, Reports mockup, phone mockups, Color, Type, Shape and space, Primitives, Navigation, Rollout. All tokens live in its `:root` block at the top of the `<style>`.
- Each step below is done by one agent, in order, on branch `claude/intelligent-tesla-bdclgg`. Each step leaves the app building and every Playwright spec passing.

## Principles (apply in every step)

1. **One surface.** Page content sits directly on the page background. Separate sections with spacing, a section heading and a hairline (`border-b border-border`). Borders, fills and shadows are only for objects: dialogs, popovers, notices, the selected/active item, form fields.
2. **Numbers are typography.** Amounts use the sans font (Geist) with `tabular-nums`, never `font-mono`. At display size (≥ 24px) the cents shrink to 50%, muted and raised. Mono (Geist Mono) is only for literal hledger syntax: queries, regex patterns, raw account paths in inputs/editors, transaction codes, CSV rules, logs.
3. **Color carries a fact.** Accent blue = action/assets. `inflow` teal = income arriving. `below` brick = owed / negative. `warning` amber = needs a look. Everything else is ink, including static balances, net worth and transfers between your own accounts. Use the categorical `--chart-*` palette only for categories.
4. **Leaf first, type always.** In dense views, show the account leaf in ink with the parents muted (or omitted, with the full path in `title`), plus a 7px square glyph colored by account type.
5. **Attention is earned.** Reviewed (`Cleared`) transactions carry no status mark. Unreviewed ones get an accent dot, and Pending (`!`) gets an amber ring. Warnings are one-line `Notice`s, not tinted cards.
6. **Dense by default.** Table rows are about 34px on desktop and about 48px on phone. Don't add page subtitles that repeat the title, and don't add breadcrumbs that repeat the title.

## Tokens

Replace `:root` / `.dark` in `web/src/style.css`. Keep the shadcn variable names (components depend on them) and remap them. Keep the existing `--chart-1..8` values as they are (they are CVD-validated).

| Waterline | shadcn var(s) | Light | Dark |
|---|---|---|---|
| paper | `--background`, `--card` | `oklch(0.985 0.004 235)` | `oklch(0.175 0.012 250)` |
| raised | `--popover` | `oklch(1 0 0)` | `oklch(0.225 0.014 250)` |
| sunken | `--muted`, `--secondary`, `--sidebar` | `oklch(0.963 0.007 235)` | `oklch(0.205 0.013 250)` |
| ink | `--foreground`, `--card-foreground`, `--popover-foreground`, `--secondary-foreground`, `--sidebar-foreground` | `oklch(0.23 0.025 250)` | `oklch(0.94 0.007 240)` |
| ink-2 | `--muted-foreground` | `oklch(0.48 0.022 250)` | `oklch(0.73 0.014 245)` |
| ink-3 | new `--faint` | `oklch(0.64 0.016 250)` | `oklch(0.57 0.014 245)` |
| rule | `--border`, `--sidebar-border` | `oklch(0.915 0.008 240)` | `oklch(0.275 0.014 250)` |
| rule-strong | `--input` | `oklch(0.86 0.012 240)` | `oklch(0.35 0.016 250)` |
| accent | `--primary`, `--ring`, `--sidebar-primary`, `--sidebar-ring` | `oklch(0.50 0.14 248)` | `oklch(0.72 0.12 245)` |
| accent-ink | `--primary-foreground`, `--sidebar-primary-foreground` | `oklch(0.99 0 0)` | `oklch(0.18 0.02 250)` |
| accent-soft | `--accent`, `--sidebar-accent` (fg = ink) | `oklch(0.95 0.028 248)` | `oklch(0.28 0.05 248)` |
| inflow | `--success` (and new `--inflow`) | `oklch(0.53 0.10 170)` | `oklch(0.77 0.11 170)` |
| inflow-soft | new `--inflow-soft` | `oklch(0.95 0.03 170)` | `oklch(0.27 0.04 170)` |
| below | `--destructive` (and new `--below`) | `oklch(0.55 0.14 32)` | `oklch(0.73 0.13 32)` |
| below-soft | new `--below-soft` | `oklch(0.955 0.025 32)` | `oklch(0.27 0.045 32)` |
| warning | `--warning` | `oklch(0.58 0.13 65)` | `oklch(0.81 0.13 80)` |
| warning-soft | new `--warning-soft` | `oklch(0.96 0.045 85)` | `oklch(0.28 0.05 80)` |
| account glyphs | new `--acct-asset/-liability/-income/-expense/-equity` | accent / below / inflow / `oklch(0.66 0.025 250)` / `oklch(0.58 0.12 300)` | accent / below / inflow / `oklch(0.62 0.02 250)` / `oklch(0.73 0.11 300)` |

Expose every new var through `@theme inline` (`--color-faint`, `--color-inflow`, …) so Tailwind classes like `text-faint` and `bg-inflow-soft` work. Foreground pairs: `--success-foreground`, `--destructive-foreground` and `--warning-foreground` should read on their solid fills in both themes.

Shape: `--radius: 0.375rem` (6px controls). Chips and tags use 4px or a pill. Notices and popovers use 8px. Dialogs and sheets use 12px. Elevation is none on page content, a 1px ring for selected/active items, and one soft shadow for popovers and dialogs only.

Type: one family. Geist Variable for UI. **Geist Mono replaces JetBrains Mono** (`@fontsource-variable/geist-mono`). `body` gets `font-variant-numeric: tabular-nums`. Scale:

| Role | Size / weight / tracking |
|---|---|
| Display figure | 44px / 500 / −0.035em, cents at 0.5em ink-3 raised |
| Figure | 26px / 500 / −0.03em |
| Page title | 22px / 600 / −0.02em |
| Section heading | 14px / 600 |
| Body / row | 13.5–14px / 400 (payee 500) |
| Label | 11px / 500 / +0.08em uppercase, ink-3 |
| Code | Geist Mono 12.5px |

## Primitives (built in step 2, used everywhere after)

- `components/amount.jsx`: `<Amount value commodity? cost? size="default|figure|display" tone="auto|ink|inflow|below" signed? />`. It wraps the existing `format.js` helpers (`formatCurrency`, `formatCostBasis`, …) and never reimplements formatting. `size` sets the type scale (cents shrink at `figure`/`display`). `tone="auto"` leaves amounts ink; callers pass `inflow` for income arriving and `below` for negative balances. Add a secondary line for commodity + cost (`10 AAPL @ $175.00`, mono, ink-3).
- `components/account.jsx`: `<Account name variant="leaf|full|path" glyph? />`. The `leaf` variant shows the last segment. `path` shows the parents muted with the leaf in ink. `full` is mono for editors. Always set `title` to the full path. Add `accountType(name)` to `lib/accounts.js`, mapping the top-level segment: `assets|asset` → asset, `liabilities|liability` → liability, `income|revenue|revenues` → income, `expenses|expense` → expense, `equity` → equity, anything else → other (ink-3 glyph). Also add `<AccountGlyph type />` and `<Flow from to />` (`from → to`, where `to` can be a count string like "3 accounts").
- `components/status.jsx`: `<StatusMark status />`. Cleared/reviewed renders nothing, `""` (unmarked) renders an accent dot, Pending renders an amber ring. It keeps the existing click-to-cycle behavior from `StatusButton` in `transaction-table.jsx`, including the `aria-label` and title semantics that tests rely on.
- `Notice`: an `Alert` variant in `components/ui/alert.jsx` (`variant="info|warning|critical"`). It's a single line with a 8px dot, the soft background and an optional trailing action link or button.
- `Card`: add `variant` to `ui/card.jsx`. `section` (no ring, no fill, no outer padding, header = section heading + hairline) becomes the default for `PageCard`. `raised` keeps a ring and is for genuine objects. Keep `size`.
- `Section`/`MetricHero` helpers in `components/page.jsx` as needed (hero figure + delta chip + optional sparkline).

## Steps

Each step is one subagent. "Verify" always includes `cd web && bun run build` and the Playwright specs listed (`PW_CHROMIUM_PATH=/opt/pw-browsers/chromium bun run playwright test <specs> --trace off`), plus a look at the regenerated screenshots in `web/test-results/` for the touched pages, in light and dark where relevant.

1. **Tokens and type.** `style.css` tokens per the table, `@theme` exposure, font swap in `package.json`/`style.css` (`bun install` to update `bun.lock`), `tabular-nums` on body, and a pass over `components/ui/*` base styles that fight the language: `ui/card.jsx` ring/fill (leave the variant work to step 2), `rounded-none` overrides, button/badge/input/tabs radius and heights. No page edits. Verify: build plus `tests/screenshots.spec.js`.
2. **Primitives.** Build everything in "Primitives" above, add `accountType`, and adopt it in the shared components that aren't page-specific: `page.jsx` (`PageCard` → section variant, `MetricCard`), `balance-summary.jsx`, `account-list.jsx`, `insights-chart.jsx`, `empty-state.jsx`, `error-banner.jsx`, `loading.jsx`. Don't touch `transaction-table.jsx` or pages yet. Verify: build plus all specs.
3. **Shell and navigation.** `app-shell.jsx`: wordmark plus a "New transaction" primary button (opens the existing `AddTransactionModal`, shortcut `N` when no input is focused) and a "Jump to… ⌘K" trigger that opens a command palette (existing `ui/command.jsx`) listing pages, accounts (`ListAccounts`) and payees, with selection navigating as the existing links do. Nav groups: (none) Overview `/`, Transactions `/transactions` (with a count of unreviewed transactions from an existing RPC; add a mock in `tests/mock-api.js` if needed), Reports `/trends`, Investments `/portfolio`; Organize: Accounts, Payees, Rules, Templates; Bring in: Connections, Import `/import`; footer: Settings `/settings`, plus the theme toggle moved into the footer/settings area. Add `components/section-tabs.jsx` and render it at the top of grouped pages so every route stays reachable: Reports = Net worth `/trends`, Income & spending `/monthly`; Investments = Holdings `/portfolio`, Lots `/lots`, Prices `/prices`; Import = Import `/import`, History `/imports`; Settings = General `/settings`, Assertions `/assertions`, Snapshots `/snapshots`, Query `/hledger-query`, Logs `/logs`. Nav items count as active for every route in their group. Remove the header breadcrumb that repeats the page title, and give page headers a right-aligned actions slot (title 22px). On mobile (`use-mobile`), add a bottom tab bar (Overview, Activity, + New, Reports, More, where More opens the existing sidebar sheet). Update spec selectors that depended on the old nav (hamburger tests and so on). Verify: build plus all specs.
4. **Overview and Trends.** `pages/home.jsx`: page title "Overview" (route unchanged) and a review `Notice` linking to `/transactions` filtered to unreviewed when the count > 0. Hero: "Net worth" label, display `Amount`, delta chip vs previous month (inflow-soft background) and the year change in ink-3, sparkline of net worth over 12 months (data the Trends page already fetches; reuse its query/RPC). Waterline bar: Assets bar (accent) and Liabilities bar (below) at true scale, separated by a dashed rule labeled "Waterline". Two columns: Accounts list (grouped Assets/Liabilities, glyph + leaf, `Amount`), and "<Month> spending" with an In / Out / Kept line and single-hue accent bars with an amount and a share %. Keep the period selector in the header. `pages/trends.jsx`: collapse the stat cards into a hero, restyle charts per step 6 rules (shared chart helpers can be added here). Verify: build, `screenshots.spec.js`, `screenshots-mobile.spec.js`, `datepicker-screenshots.spec.js`.
5. **Transactions.** `components/transaction-table.jsx`, `pages/transactions.jsx`, `components/search-controls.jsx`, `tag-editor.jsx`, `table-pagination.jsx`. Group rows by day with sticky-ish day headers ("25 Mar Wed" plus the day's net of income/expense postings, ignoring transfers between asset/liability accounts; show "transfer" when 0). Single-line rows: status mark, payee (500) + note (ink-3) + tags as quiet `#tag` chips (editing still available on hover/click), `Flow` with leaf accounts and glyphs, right-aligned `Amount` (inflow teal for income, ink otherwise, commodity/cost second line). Checkbox and row actions appear on hover/focus or when any row is selected; the bulk-edit toolbar is unchanged in behavior. Toolbar: one wide search field (the placeholder mentions hledger query syntax) and filter chips (period, account, status "Needs review", tags), with the active filter in accent-soft. A summary line shows the count, in and out. Mobile: stacked rows (payee + amount, then flow in ink-3). The account register view gets the same treatment. Keep all existing behaviors and test selectors working (update specs only where a selector was purely visual). Verify: build, `screenshots.spec.js`, `screenshots-mobile.spec.js`, `transaction-interactions.spec.js`, `datepicker-screenshots.spec.js`, `screenshots-unknown-account.spec.js`.
6. **Reports and charts.** `pages/monthly-dashboard.jsx`: matrix with a sticky account column, glyph + leaf, whole-dollar display with a `$1,234 | $1,234.56` segmented toggle (default whole), an Avg column, cell heat shading by value within its row (`color-mix` of accent for expenses and inflow for income, 4–24%), Total rows in weight 600, and a "Kept" (net) row with a strong top rule, inflow/below tone. `components/ui/chart.jsx` and chart usages (`insights-chart.jsx`, `trends.jsx`, portfolio charts): 1.8px lines, an area fill on the primary series, an endpoint dot, direct labels instead of a legend when there are 3 or fewer series, axis/grid in ink-3/rule, and tooltips using `Amount`. Verify: build plus all specs.
7. **Investments.** `pages/portfolio.jsx`, `pages/lots.jsx`, `pages/prices.jsx`. Hero (total value + gain delta) instead of four stat cards. Missing/stale price warnings become `Notice`s (critical/warning) with their existing actions. The Allocation half-donut becomes a single stacked horizontal bar with a legend list. Holdings table uses `Amount`/`Account`, with mono only for symbols. Lots and Prices adopt the primitives and section cards. Verify: build, `screenshots.spec.js`, `portfolio-mobile.spec.js`, `price-ui-screenshots.spec.js`.
8. **Organize pages.** `pages/accounts.jsx`, `payees.jsx`, `rules.jsx`, `templates.jsx`, `components/rule-issues-dialog.jsx`, `suggest-rules-wizard.jsx`, `inline-edit.jsx`. Section cards, primitives, mono only for regex patterns/raw paths in inputs, consistent table density, and dialogs at 12px radius. Verify: build, `screenshots.spec.js`, `rule-issues-screenshots.spec.js`, `extra-mobile.spec.js`, `new-features-screenshots.spec.js`.
9. **Bring-in pages.** `pages/connections.jsx`, `import.jsx`, `imports-history.jsx`. Same treatment, with status as Notices/StatusMark-style dots, mono only for CSV rules/file content. Verify: build, `screenshots.spec.js`, `connections-screenshots.spec.js`.
10. **Settings and system pages, entry forms.** `pages/settings.jsx`, `balance-assertions.jsx`, `snapshots.jsx`, `hledger-query.jsx`, `logs.jsx`, `login.jsx`, `add-transaction.jsx`, `components/add-transaction-modal.jsx`, `posting-fields.jsx`, `period-selector.jsx`, `filter-input.jsx`, `stats-15.jsx` (delete it if unused). Login gets a centered wordmark plus a single field on paper. Verify: build plus all specs.
11. **Final consistency pass.** Grep for leftovers (`font-mono` on amounts, `text-success`/`text-destructive` used as decoration, `rounded-none`, nested cards, raw `<Card>` wrapping sections), regenerate all screenshots and review every page in light and dark at desktop and phone widths, fix inconsistencies, and update `web/CLAUDE.md` with a short "Design language" section (link this file, list the primitives and the do/don't rules). Verify: `mise run web-build`, `mise run web-test`, `mise run check-all`.

## Guardrails for every step

- Don't change RPC calls, query keys, route paths, URL search params or data semantics. Purely visual reorganization only (the step 3 command palette and review count reuse existing RPCs).
- Follow `web/CLAUDE.md`: React 18 (`forwardRef` for anything used as a base-ui `render` element), base-ui `render={...}` rather than `asChild`, specs import from `./fixtures.js`, no unmocked RPCs, no console errors. Don't upgrade Playwright (pinned 1.56.1).
- Keep accessibility: visible focus ring, labels on icon buttons, sufficient contrast in both themes.
- Prefer editing shared primitives over per-page one-offs. Don't add new dependencies beyond the font swap unless the step requires it.
- Stay inside your step's files. If something outside your scope is broken or ugly, note it in your final report rather than fixing it.
- Commit your step as one or more focused commits on `claude/intelligent-tesla-bdclgg` with a message starting `web(waterline): step N – <title>`. Don't push.

## As built (kept current by each step)

- **Step 1 (tokens and type):** tokens in `style.css` with Tailwind classes (`text-faint`, `bg-inflow-soft`, `text-below`, `bg-warning-soft`, `--acct-*` incl. `--acct-other`). Radius scale: sm 4, md 6, lg 8, xl 12. Geist Mono replaces JetBrains Mono.
- **Step 2 (primitives):**
  - `components/amount.jsx`: `<Amount value commodity? cost? costBasis? size="default|figure|display" tone="auto|ink|inflow|below" signed? negate? />`. `value` can be a number/string, an amount object, or an array (one line per commodity). With a cost, the main line is the value at cost, with a mono lot line under it.
  - `components/account.jsx`: `<Account name variant="leaf|path|full" glyph? type? />`, `<AccountGlyph type />`, `<Flow from to variant? />`. `lib/accounts.js`: `accountType`, `accountTypeFromCode`, `splitAccount`.
  - `components/status.jsx`: `<StatusMark status fid? onStatusChange? />`. Static without `fid`; with `fid` it has `StatusButton`'s click-to-cycle behavior.
  - `ui/alert.jsx`: variants `info|warning|critical`, plus `<Notice variant action>`.
  - `ui/card.jsx`: `variant="raised|section"` (+ `size`). `PageCard` defaults to `section`.
  - `components/page.jsx`: `Section`, `Eyebrow`, `DeltaChip`, `Sparkline`, `MetricHero`. `MetricCard` is flat and takes `amount`/`tone`.
  - Type classes in `style.css`: `text-label`, `text-control`, `text-body`, `text-figure`, `text-display`, … (registered in `lib/utils.js` so `cn` keeps them). Body 13.5px, controls 13px, table heads 11px uppercase.
- **Step 3 (shell and navigation):**
  - `components/nav-items.js`: `NAV_GROUPS`, `SETTINGS_ITEM`, `SECTION_TABS` (single source for sidebar, tabs and palette), `navItemFor(path)` (an item is active for every route in its `routes`/`tabs`), `PAGE_ENTRIES`.
  - `components/section-tabs.jsx`: `<SectionTabs items={[{to,label}]} currentPath label? />` (a labelled `<nav>`, `aria-current` on the current tab). The shell renders it above the page for grouped routes, so pages don't.
  - `components/command-palette.jsx`: `<CommandPalette open onOpenChange onNewTransaction />` (pages, accounts via `ListAccounts`, payees via `ListPayees`; loads only while open).
  - `hooks/use-review-count.js`: `useReviewCount()` (`ListTransactions` with `not:status:*`, `limit: 1`, reads `total`; key `queryKeys.unreviewedCount()`), plus `UNREVIEWED_QUERY` and `UNREVIEWED_SEARCH` (`{ search: "not:status:*" }` for `/transactions`). Note: the Transactions page still applies its default "This month" range, so the filtered list can show fewer than the all-time count.
  - `app-shell.jsx`: sunken sidebar (13.5rem) with a text wordmark, "New transaction" (`N`), "Jump to…" (`Ctrl/⌘K`), review-count pill linking to the unreviewed list, Settings + theme toggle in the footer; content on paper with no header/breadcrumb. Phone: sticky bottom tab bar (`data-slot="mobile-tab-bar"`: Overview, Activity, + New, Reports, More → sidebar sheet).
  - `PageHeader`: `<PageHeader title description? actions? />` (children also work as actions), `h1` at `text-title`, actions right-aligned.
  - `ui/sidebar.jsx`: menu buttons are ink-2; active = raised fill, 1px ring, accent icon.
  - `tests/mock-api.js`: `ListTransactions` handles `status:` tokens and `limit`/`offset` with `total`. New `tests/shell.spec.js`.
- **Step 4 (overview and trends):**
  - Grouped routes: the shell renders `GroupHeader` (in `page-header.jsx`): the group label from `nav-items.js` as the `h1`, an actions slot on the right, `SectionTabs` under it. Inside a group, `GroupHeaderProvider` makes `<PageHeader>` drop its `title`, portal its actions into that slot and render only its `description` (as `data-slot="page-description"`). Pages need no changes; ungrouped routes are unchanged.
  - `lib/net-worth.js`: `timeseriesBegin(months)`, `netWorthTimeseriesQuery(begin)` (shared by Overview and Trends, same cache entry for 12 months), `netWorthChanges(snapshots, current)` (`monthChange`, `prevMonth`, `ytdChange`), `firstQuantity`, `shortMonthLabel`, `deltaTone`.
  - `components/segmented.jsx`: `<Segmented options={[{value,label}]} value onChange label />` (aria-pressed buttons; Trends range, reusable for step 6's precision toggle).
  - `pages/home.jsx`: Overview (review `Notice`, `MetricHero` with sparkline, local `Waterline`, Accounts + spending `Section`s). `insights-chart.jsx` is now the spending block (In / Out / Kept from hledger's `report.total`, accent bars with share %); the revenue-by-account bars are gone (In covers it). `mock-api.js` returns `report.total` for expense/revenue balances.
  - `pages/trends.jsx`: `MetricHero` + `DeltaChip`s; net worth chart is a `ComposedChart` (ink area for net worth, asset/liability-colored lines, end labels with dots, compact y-axis). `ui/chart.jsx` axis ticks are `fill-faint` and grid lines full `border`. `BalanceSummary` is now unused.
- **Step 5 (transactions):**
  - Split `transaction-table.jsx` into `lib/transactions.js` (flow/amount pickers, `flowTotals`, `dayNet`, `registerTotals`, `registerDayNet`, `isIncome`, `dayLabel`), `transaction-editors.jsx` (inline cells, `EditableDetailRow`, `StripeIndicator`), `transaction-columns.jsx` (column defs, `HOVER_REVEAL`), `transaction-summary.jsx` (`<TransactionSummary>`: count, In, Out) and the table itself (`TransactionRow`, day headers, `MobileList`/`MobileRow`).
  - Rows: `tr[data-slot=transaction-row]` (`data-fid`), `StatusMark`, payee 500 + note ink-3, quiet `#tag` chips (`TagChip`, `tagLabel`, `<TagEditor quiet>` from `tag-editor.jsx`), hover "Edit" button (expands), From / → To leaf accounts with glyphs, `Amount` (inflow + signed when every "from" posting is income). Checkboxes show on hover/focus or once anything is selected. Sorted by date: `tr[data-slot=day-header]` rows (sticky from `lg`) with the day's income − expenses or "transfer"; the Date sort toggle sits in the Description header. Any other sort: flat list with a Date column. Register: same, with signed Change (inflow when up) and Balance (below when negative).
  - Phone: `[data-slot=transaction-card]` stacked rows under day headers (tap opens the editor), and a "Select"/"Done" toggle for checkboxes.
  - Toolbar (`search-controls.jsx`): one search field (mono when it holds a query), chips (`FilterChip`, `ChipSelect`): period (`DateRangePicker embedded`), account, "Needs review · N" (`aria-pressed`, count from `useReviewCount`), Status (Reviewed / No payee set), Tags, payee; summary on the right. `/transactions?search=not:status:*` (the review links) now opens the Needs review chip over All time and drops the param, so the list matches the count.
