---
name: web-screenshots
description: Capture and display screenshots of the float web UI using Playwright with mocked API data (no live floatd needed). TRIGGER when the user asks to see the web UI, preview web UI changes, take a screenshot of a specific page, or visually review the interface. Output images inline or upload to paste service.
---

# web-screenshots skill

Captures screenshots of the float web UI using Playwright. The test suite spins up the Vite dev server automatically and intercepts all `LedgerService` API calls with realistic mock data — no running `floatd` is required.

Screenshots are saved to `web/test-results/` and can be read directly with the `Read` tool to display them inline in the conversation, or uploaded to the paste service to share a URL.

## Prerequisites

- `web/node_modules/` must be populated. If not: `cd web && npm install`
- No other prerequisites — Playwright and Chromium are already installed.

## Running the screenshot tests

```bash
cd /home/user/float/web
npx playwright test tests/screenshots.spec.js
```

Or via the npm script:

```bash
cd /home/user/float/web && npm run screenshots
```

### Output

| File | Page |
|------|------|
| `web/test-results/home.png` | Home dashboard |
| `web/test-results/transactions.png` | Transactions list |
| `web/test-results/add-transaction.png` | Add Transaction form |

## Showing screenshots inline

After running the tests, read each file directly — Claude can display images:

```
Read tool: /home/user/float/web/test-results/home.png
Read tool: /home/user/float/web/test-results/transactions.png
Read tool: /home/user/float/web/test-results/add-transaction.png
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

Test file: `web/tests/screenshots.spec.js`
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

### Updating mock data

Edit `web/tests/mock-api.js`. The `mockLedgerApi` function intercepts Connect RPC requests by matching the method name at the end of the URL (e.g. `ListAccounts`, `GetBalances`, `ListTransactions`). Add new cases to the `switch` block for additional RPC methods.

### Testing specific UI interactions

Playwright can click, type, and wait before capturing:

```js
test("add transaction form filled", async ({ page }) => {
  await page.goto("/#/add");
  await page.waitForSelector("form");
  await page.fill('input[type="date"]', "2026-03-28");
  await page.fill('input[placeholder*="Grocery"]', "Whole Foods");
  await page.screenshot({ path: "test-results/add-transaction-filled.png", fullPage: true });
});
```

## Configuration

Playwright config: `web/playwright.config.js`

- Starts Vite dev server on port **5174** (separate from the normal dev port 5173 to avoid conflicts)
- Uses system Chromium (already installed at `/root/.cache/ms-playwright/chromium-1194`)
- `@playwright/test` is pinned to **1.56.1** to match the system browser version
- Screenshots are saved to `web/test-results/` (gitignored)
