# float TUI — Bubbletea Implementation Plan

## Context

float's PLAN.md Step 6 calls for a simple CLI client (`float balances`, `float transactions`, `float accounts`). Instead, we're building a full Bubbletea-based TUI inspired by [Bagels](https://github.com/EnhancedJax/Bagels) — a terminal expense tracker with multi-panel layout, tab navigation, and keyboard-driven UX.

The TUI lives in `cmd/float/` and connects to `floatd` via ConnectRPC using the generated `LedgerServiceClient` at `gen/float/v1/floatv1connect/`. It consumes the existing 6 RPCs (ListTransactions, GetBalances, ListAccounts, DeleteTransaction, ModifyTags, UpdateTransactionDate) with no proto changes needed.

**Why a TUI instead of a CLI:** float is a personal finance tool where users browse, filter, and drill into data interactively. A TUI provides this naturally, while a CLI would require many separate commands with flags to achieve the same workflows.

---

## Architectural Decisions

1. **Layout:** lipgloss `JoinHorizontal`/`JoinVertical` with a custom proportional `CalcLayout` function — no third-party layout library needed (~100 lines of layout math)
2. **Components:** Each panel is its own `tea.Model`. Root model orchestrates, forwarding messages based on focus
3. **Data fetching:** `tea.Cmd` functions that call gRPC synchronously (bubbletea runs Cmds in goroutines), returning typed `tea.Msg` results
4. **Proto types used directly:** No intermediate domain types — `*floatv1.Transaction`, `*floatv1.BalanceReport`, `*floatv1.Account` flow through the TUI as-is
5. **Testing:** Mock the generated `LedgerServiceClient` interface (it's already an interface from ConnectRPC codegen)

### Dependencies to Add
```
github.com/charmbracelet/bubbletea
github.com/charmbracelet/lipgloss
github.com/charmbracelet/bubbles
```

---

## Phase 1: Foundation & Layout Shell

**Goal:** Running Bubbletea app with responsive two-column layout, tab bar, help bar, and correct resize handling. No data yet.

### Files to Create

| File | Purpose |
|------|---------|
| `cmd/float/main.go` | Entry point: `--server` flag, ConnectRPC client construction, `tea.NewProgram` with `tea.WithAltScreen()` |
| `cmd/float/ui/app.go` | Root model: holds width/height, activeTab, child models, client. Routes messages |
| `cmd/float/ui/layout.go` | `CalcLayout(w, h) Layout` — pure function computing panel dimensions. Left column 30% (min 25, max 45). Right column = remainder |
| `cmd/float/ui/layout_test.go` | Table-driven tests for CalcLayout at various terminal sizes (80x24, 120x40, 40x15) |
| `cmd/float/ui/tabbar.go` | Tab bar model: "[ Home ] Manager" with lipgloss styling. 1 line tall |
| `cmd/float/ui/helpbar.go` | Bottom shortcuts bar: context-sensitive keys. 1 line tall |
| `cmd/float/ui/hometab.go` | Home tab model: bordered left + right empty panels via JoinHorizontal |
| `cmd/float/ui/managertab.go` | Manager tab: placeholder "coming soon" text |
| `cmd/float/ui/style.go` | Shared lipgloss styles (borders, colors, amounts, headers) |

### Resize Flow
1. `tea.WindowSizeMsg` → root stores width/height
2. Root calls `CalcLayout(width, height)` → `Layout{LeftWidth, RightWidth, ContentHeight}`
3. Root calls `SetSize(w, h)` on each tab model
4. Each tab propagates sub-dimensions to its child panels
5. `View()` renders using stored dimensions

### Key Implementation Detail — Border Accounting
```go
func innerSize(outerW, outerH int, style lipgloss.Style) (int, int) {
    return outerW - style.GetHorizontalFrameSize(), outerH - style.GetVerticalFrameSize()
}
```
Every panel subtracts its own border/padding internally. The `Layout` struct provides gross dimensions.

### Minimum Terminal Size
If terminal < 60x15, show centered message: "Terminal too small. Need at least 60x15."

### What the User Sees
```
[ Home ]  Manager
┌─ Accounts ─────────┐  ┌─ Transactions ──────────────┐
│                     │  │                              │
│   (loading...)      │  │   (loading...)               │
│                     │  │                              │
└─────────────────────┘  └──────────────────────────────┘
 q quit  tab switch  j/k navigate
```

### Verification
- `go build ./cmd/float/` succeeds
- `go run ./cmd/float/` launches alt-screen TUI with empty panels
- Resizing terminal reflows panels proportionally
- Tab/Shift-Tab switches between Home/Manager
- `q` or Ctrl-C exits cleanly

---

## Phase 2: Data Layer & Accounts Panel

**Goal:** Fetch real data from floatd, display accounts with balances, handle loading/error states.

### Files to Create/Modify

| File | Purpose |
|------|---------|
| `cmd/float/ui/fetch.go` | `FetchAccounts`, `FetchBalances`, `FetchTransactions` — tea.Cmd functions returning typed Msgs |
| `cmd/float/ui/fetch_test.go` | Tests with mock `LedgerServiceClient` |
| `cmd/float/ui/accounts.go` | Accounts panel: groups by type (A/L/E/R/X), shows balance beside each, scrollable |
| `cmd/float/ui/accounts_test.go` | Rendering tests at various widths, account-type grouping logic |
| `cmd/float/ui/spinner.go` | Thin wrapper around `bubbles/spinner` for consistent loading indicators |

### Data Flow
- On init, hometab dispatches `tea.Batch(FetchAccounts(client), FetchBalances(client, 0, nil))` — concurrent fetches
- `AccountsMsg` and `BalancesMsg` arrive independently → accounts panel merges them
- Account balances looked up by matching `BalanceRow.FullName` to `Account.FullName`

### Mock Client Pattern
```go
type mockLedgerClient struct {
    floatv1connect.LedgerServiceClient // embed for unimplemented methods
    listAccountsFn     func(context.Context, *connect.Request[floatv1.ListAccountsRequest]) (*connect.Response[floatv1.ListAccountsResponse], error)
    // ... other func fields
}
```

### Error State
Connection failures show inline: `"! Connection failed: localhost:8080"` with `r` to retry.

### Verification
- Start `floatd` with test data, run `go run ./cmd/float/` — accounts with balances appear
- Kill `floatd`, restart TUI — error state shown, press `r` to retry

---

## Phase 3: Transactions Table

**Goal:** Scrollable, navigable transactions table in the right panel with filtering.

### Files to Create/Modify

| File | Purpose |
|------|---------|
| `cmd/float/ui/transactions.go` | Table using `bubbles/table`: Date (10ch fixed), Description (40% flex), Amount (12ch fixed, right-aligned), Account (remainder) |
| `cmd/float/ui/transactions_test.go` | Column width calculation, primary posting selection, rendering |
| `cmd/float/ui/filter.go` | `bubbles/textinput` activated with `/`, sends query on Enter, clears on Esc |
| `cmd/float/ui/filter_test.go` | Filter → re-fetch command tests |

### Focus Model
Home tab tracks `focusLeft` / `focusRight`. Focused panel gets highlighted border. `Tab` or `h`/`l` switches focus. Key events forwarded only to focused panel.

### Multi-Posting Display
Show primary posting (expense/income side). `s` key toggles split view showing all postings.

### Column Resize
On `SetSize`, recalculate column widths. Description is the flex column absorbing width changes. `bubbles/table` handles row scrolling given a height.

### Verification
- Transactions appear in table with correct formatting
- j/k scrolls rows, Tab switches focus between panels
- `/` opens filter, typing and Enter re-fetches with query
- `s` toggles split view for multi-posting transactions

---

## Phase 4: Period Navigation & Insights

**Goal:** Month-based period selector and expense breakdown bar chart in the left column.

### Files to Create/Modify

| File | Purpose |
|------|---------|
| `cmd/float/ui/period.go` | Period selector: `<<< March 2026 >>>`, `[`/`]` to navigate months. Exposes `Query() string` returning `"date:2026-03"` |
| `cmd/float/ui/period_test.go` | Month navigation, year rollover, query format |
| `cmd/float/ui/insights.go` | Horizontal bar chart of expense categories using lipgloss block chars. Data from `GetBalances(depth=2, query=["expenses", period])` |
| `cmd/float/ui/insights_test.go` | Bar rendering at various widths, empty/single/many categories |

### Left Column Sub-Layout
Vertically stacked:
- Accounts panel: 55% of content height (min 5 rows)
- Period selector: 1 line (fixed)
- Insights: remaining height (min 3 rows, hidden if height < 15)

### Period Change Triggers
When period changes, hometab dispatches new fetches for balances and transactions with the period query appended.

### Verification
- `[`/`]` changes month, data refreshes
- Bar chart shows expense categories proportionally
- Resizing shrinks/hides insights panel gracefully

---

## Phase 5: Manager Tab

**Goal:** Account tree view and balance summaries by account type.

### Files to Create/Modify

| File | Purpose |
|------|---------|
| `cmd/float/ui/accounttree.go` | Hierarchical tree built from flat `ListAccounts` response (parse `full_name` on `:`). Collapsible nodes with Enter |
| `cmd/float/ui/accounttree_test.go` | Tree building, collapse/expand, narrow-width truncation |
| `cmd/float/ui/summary.go` | Net worth (Assets - Liabilities) and net income (Income - Expenses) from `GetBalances(depth=1)` |
| `cmd/float/ui/summary_test.go` | Balance grouping, net worth/income calculation |

### Manager Layout
Two columns: Left 35% (summary top, chart placeholder bottom), Right 65% (account tree full height).

### Verification
- Switch to Manager tab, see account tree with type annotations
- Collapse/expand nodes with Enter
- Summary shows net worth and net income

---

## Phase 6: Interactive Features

**Goal:** Transaction detail overlay, delete confirmation, date/tag editing using existing write RPCs.

### Files to Create/Modify

| File | Purpose |
|------|---------|
| `cmd/float/ui/detail.go` | Modal overlay on Enter: shows all postings, tags, FID. Centered, sized `min(60, w-4)` x `min(20, h-4)` |
| `cmd/float/ui/confirm.go` | Delete confirmation dialog: "Delete transaction X? [y/n]" |
| `cmd/float/ui/commands.go` | `DeleteTransaction`, `ModifyTags`, `UpdateDate` — tea.Cmd functions for write RPCs |
| `cmd/float/ui/commands_test.go` | Write commands with mock client |
| `cmd/float/ui/dateinput.go` | Date editor: `bubbles/textinput` with YYYY-MM-DD validation |
| `cmd/float/ui/taginput.go` | Tag editor: key=value input for ModifyTags |

### Refresh After Writes
After successful write, dispatch `tea.Batch(FetchTransactions(...), FetchBalances(...))` for full refresh — consistent with server's cache invalidation strategy.

### Verification
- Enter on a transaction opens detail overlay
- `d` in detail → confirmation → delete → table refreshes
- `e` in detail → date input → update → table refreshes
- Esc dismisses overlays at any point

---

## Complete File Tree

```
cmd/float/
├── main.go
├── plan.md
└── ui/
    ├── app.go            # root model
    ├── layout.go         # CalcLayout
    ├── layout_test.go
    ├── tabbar.go
    ├── helpbar.go
    ├── style.go          # shared lipgloss styles
    ├── hometab.go        # home tab orchestrator
    ├── managertab.go     # manager tab orchestrator
    ├── fetch.go          # async gRPC fetch commands
    ├── fetch_test.go
    ├── accounts.go       # accounts panel
    ├── accounts_test.go
    ├── spinner.go
    ├── transactions.go   # transactions table
    ├── transactions_test.go
    ├── filter.go
    ├── filter_test.go
    ├── period.go         # period selector
    ├── period_test.go
    ├── insights.go       # expense bar chart
    ├── insights_test.go
    ├── accounttree.go    # hierarchical tree
    ├── accounttree_test.go
    ├── summary.go        # net worth / net income
    ├── summary_test.go
    ├── detail.go         # transaction detail overlay
    ├── detail_test.go
    ├── confirm.go        # delete confirmation
    ├── confirm_test.go
    ├── commands.go       # write command functions
    ├── commands_test.go
    ├── dateinput.go
    └── taginput.go
```

## Critical Reference Files
- `gen/float/v1/floatv1connect/ledger.connect.go` — `LedgerServiceClient` interface
- `gen/float/v1/ledger.pb.go` — Proto types flowing through the TUI
- `internal/server/ledger/handler_test.go` — Mock client pattern reference
- `cmd/floatd/main.go` — Server wiring (TUI client mirrors this)
- `internal/hledger/types.go` — Domain types behind the proto types
- `proto/float/v1/ledger.proto` — Service contract (6 RPCs)

## mise.toml Addition
```toml
[tasks.float]
description = "Run the float TUI"
run = "go run ./cmd/float/ --server ${FLOAT_ADDR:-localhost:8080}"
```
