import { expect, test } from "./fixtures.js";
import { mockLedgerApi, mockTransactions } from "./mock-api.js";

const oldestFirst = [...mockTransactions].sort((a, b) => a.date.localeCompare(b.date));
const pending = oldestFirst.filter((tx) => tx.status === "Pending");

function newPending(date) {
  return { ...pending[0], fid: "feed0001", date, description: "New coffee", payee: undefined, note: undefined };
}

function markReviewed(txs, target) {
  return txs.map((tx) => (tx === target ? { ...tx, status: "Cleared" } : tx));
}

async function dismissNotice(page) {
  await mockLedgerApi(page, { transactions: oldestFirst });
  await page.goto("/#/");
  const notice = page.getByRole("status").filter({ hasText: "review" });
  await expect(notice).toBeVisible();
  await notice.getByRole("button", { name: "Dismiss" }).click();
  await expect(notice).toHaveCount(0);
  return notice;
}

async function reloadWith(page, transactions) {
  await mockLedgerApi(page, { transactions });
  await page.reload();
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
}

test("dismissed review notice stays hidden across reloads", async ({ page }) => {
  const notice = await dismissNotice(page);
  await reloadWith(page, oldestFirst);
  await expect(notice).toHaveCount(0);
});

test("dismissed review notice stays hidden after reviewing some transactions", async ({ page }) => {
  const notice = await dismissNotice(page);
  await reloadWith(page, markReviewed(oldestFirst, pending[0]));
  await expect(notice).toHaveCount(0);
});

test("review notice returns for a newer unreviewed transaction", async ({ page }) => {
  const notice = await dismissNotice(page);
  await reloadWith(page, [...markReviewed(oldestFirst, pending[0]), newPending("2026-03-30")]);
  await expect(notice).toBeVisible();
});

test("review notice returns for a backdated unreviewed transaction", async ({ page }) => {
  const notice = await dismissNotice(page);
  await reloadWith(page, [newPending("2026-01-02"), ...oldestFirst]);
  await expect(notice).toBeVisible();
});

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  test("review notice keeps its text and actions on one line", async ({ page }) => {
    await mockLedgerApi(page, { transactions: oldestFirst });
    await page.goto("/#/");
    const notice = page.getByRole("status").filter({ hasText: "review" });
    const text = notice.locator("[data-slot=alert-description]");
    const action = notice.locator("[data-slot=alert-action]");
    await expect(action).toBeVisible();
    const t = await text.boundingBox();
    const a = await action.boundingBox();
    expect(a.y).toBeLessThan(t.y + t.height);
    expect(a.x).toBeGreaterThanOrEqual(t.x + t.width);
  });
});
