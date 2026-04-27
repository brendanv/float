---
name: web-screenshots
description: Capture and display screenshots of the float web UI using Playwright with mocked API data (no live floatd needed). TRIGGER when the user asks to see the web UI, preview web UI changes, take a screenshot of a specific page, or visually review the interface. Output images inline or upload to paste service.
---

# web-screenshots skill

Captures screenshots of the float web UI using Playwright. The test suite spins up the Vite dev server automatically and intercepts all `LedgerService` API calls with realistic mock data — no running `floatd` is required.

Screenshots are saved to `web/test-results/` and can be read directly with the `Read` tool to display them inline in the conversation, or uploaded to the paste service to share a URL.

## Prerequisites

- `web/node_modules/` must be populated. If missing: `cd /home/user/float/web && bun install`
- `web/src/gen/` must contain generated JS protobuf files. If missing: `cd /home/user/float && mise run web-gen`
- No other prerequisites — Playwright and Chromium are already installed.

## Capturing all screenshots at once

Use the dedicated script to run all four spec files and collect the PNGs in one directory:

```bash
cd /home/user/float/web && bash capture-screenshots.sh /tmp/screenshots
```

The script accepts an optional output directory (defaults to a `mktemp -d` temp dir) and prints each test result as it runs. It exits non-zero if any tests fail but still copies all PNGs from passing tests.

## Running individual spec files

```bash
cd /home/user/float/web

# Desktop screenshots (42 tests)
bun run playwright test tests/screenshots.spec.js

# Mobile screenshots (5 tests)
bun run playwright test tests/screenshots-mobile.spec.js

# Date-picker interaction screenshots (5 tests)
bun run playwright test tests/datepicker-screenshots.spec.js

# Portfolio mobile screenshot (1 test)
bun run playwright test tests/portfolio-mobile.spec.js
```

Or via the bun script alias for the desktop suite only:

```bash
cd /home/user/float/web && bun run screenshots
```

## All defined screenshots

| File | Description |
|------|-------------|
| `test-results/home.png` | Home dashboard (balances) |
| `test-results/transactions.png` | Transaction list |
| `test-results/transactions-delete.png` | Delete confirmation |
| `test-results/transactions-filter-open.png` | Filter dropdown open |
| `test-results/transactions-mobile.png` | Transaction cards (390px) |
| `test-results/transactions-payee-filter.png` | Filtered by payee |
| `test-results/transactions-account-register.png` | Account register view |
| `test-results/transactions-account-register-mobile.png` | Account register mobile |
| `test-results/transactions-mobile-bulk-edit.png` | Mobile bulk edit toolbar |
| `test-results/transactions-bulk-edit.png` | Desktop bulk edit toolbar |
| `test-results/add-transaction.png` | Add transaction form |
| `test-results/add-transaction-modal.png` | Add transaction modal |
| `test-results/trends.png` | Net worth chart |
| `test-results/monthly-dashboard.png` | Monthly dashboard |
| `test-results/prices.png` | Commodity prices |
| `test-results/accounts.png` | Account tree |
| `test-results/import.png` | CSV import page |
| `test-results/import-profile-selected.png` | Profile selected (clipped) |
| `test-results/import-edit-profile-modal.png` | Edit profile modal |
| `test-results/import-delete-profile-dialog.png` | Delete profile dialog |
| `test-results/import-create-profile-modal.png` | Create profile modal |
| `test-results/import-create-profile-modal-wizard.png` | Create profile with CSV wizard |
| `test-results/import-preview.png` | Import preview with candidates |
| `test-results/import-history.png` | Import history list |
| `test-results/import-detail.png` | Import batch detail |
| `test-results/rules.png` | Categorization rules table |
| `test-results/rules-account-typeahead.png` | Account typeahead open (clipped) |
| `test-results/rules-account-typeahead-filtered.png` | Typeahead filtered (clipped) |
| `test-results/rules-apply-preview.png` | Apply rules preview |
| `test-results/rules-apply-preview-zoomed.png` | Apply rules preview zoomed |
| `test-results/rules-mobile-form.png` | Rules form mobile |
| `test-results/portfolio.png` | Portfolio holdings |
| `test-results/payees.png` | Payees list |
| `test-results/payees-set-payee.png` | Set payee inline form |
| `test-results/payees-mobile.png` | Payees mobile |
| `test-results/hamburger-closed.png` | Hamburger nav closed (clipped) |
| `test-results/hamburger-open.png` | Hamburger nav open (clipped) |
| `test-results/mobile-home.png` | Home mobile |
| `test-results/mobile-transactions.png` | Transactions mobile |
| `test-results/mobile-add-transaction.png` | Add transaction mobile |
| `test-results/mobile-trends.png` | Trends mobile |
| `test-results/mobile-prices.png` | Prices mobile |
| `test-results/home-datepicker-open.png` | Home date picker open (clipped) |
| `test-results/transactions-datepicker-open.png` | Transactions date picker (clipped) |
| `test-results/home-datepicker-mobile.png` | Home date picker mobile closed |
| `test-results/home-datepicker-mobile-open.png` | Home date picker mobile open |
| `test-results/transactions-datepicker-mobile-open.png` | Transactions date picker mobile |
| `test-results/portfolio-mobile.png` | Portfolio mobile |

## Showing screenshots inline

After running the tests, read each PNG directly — Claude can display images:

```
Read tool: /home/user/float/web/test-results/home.png
Read tool: /home/user/float/web/test-results/transactions.png
```

## Uploading a screenshot to the paste service

Use the [creating-pastes skill](.claude/skills/creating-pastes.md) to upload and share a URL:

```bash
RESPONSE=$(curl -s -X POST "$PASTE_URL/api/upload" \
  -H "Origin: $PASTE_URL" \
  -H "X-PASTE-USERID: $PASTE_USER_ID" \
  -H "X-PASTE-API-KEY: $PASTE_API_KEY" \
  -F "file=@/home/user/float/web/test-results/home.png" \
  -F "visibility=logged_in" \
  -F "expiration=1day")

SLUG=$(echo "$RESPONSE" | jq -r '.slug')
echo "Screenshot: ${PASTE_URL}/p/${SLUG}"
```

## Adding or updating screenshot tests

Spec files:
- `web/tests/screenshots.spec.js` — desktop (42 tests)
- `web/tests/screenshots-mobile.spec.js` — mobile (5 tests)
- `web/tests/datepicker-screenshots.spec.js` — date picker (5 tests)
- `web/tests/portfolio-mobile.spec.js` — portfolio mobile (1 test)

Mock data and API interception: `web/tests/mock-api.js`

### Adding a new page screenshot

```js
test("my new page", async ({ page }) => {
  await page.goto("/#/my-route");
  await page.waitForSelector("main-element-selector", { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(400);
  await page.screenshot({ path: "test-results/my-page.png", fullPage: true });
});
```

### Selector notes

- Checkboxes use `@base-ui/react` — select `[data-slot="checkbox"]`, not `input[type=checkbox]`
- Dropdowns use shadcn `Select` — use `[role="combobox"]` to open and `[role="option"]` to pick
- `AccountInput` renders as `button[role="combobox"]`; the search input inside the popover has `placeholder="Search <account-placeholder>..."`

### Updating mock data

Edit `web/tests/mock-api.js`. The `mockLedgerApi` function intercepts Connect RPC requests by matching the method name at the end of the URL (e.g. `ListAccounts`, `GetBalances`, `ListTransactions`). Add new cases to the `switch` block for additional RPC methods.

## Configuration

Playwright config: `web/playwright.config.js`

- Starts Vite dev server on port **5174** (separate from the normal dev port 5173 to avoid conflicts)
- Uses system Chromium (already installed at `/root/.cache/ms-playwright/chromium-1194`)
- `@playwright/test` is pinned to **1.56.1** to match the system browser version
- Screenshots are saved to `web/test-results/` (gitignored)
