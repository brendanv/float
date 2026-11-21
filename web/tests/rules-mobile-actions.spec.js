import { test, expect } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

// The Rules section's three actions share one row on a small phone with
// touch sizing, rather than wrapping onto two.
test.use({ viewport: { width: 375, height: 740 }, hasTouch: true, isMobile: true });

test("rules actions sit on a single row on mobile", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/rules");
  const names = ["Suggest Rules", "Find Issues", "Preview Changes"];
  const boxes = [];
  for (const name of names) {
    const button = page.getByRole("button", { name, exact: true });
    await expect(button).toBeVisible();
    boxes.push(await button.boundingBox());
  }
  for (const box of boxes.slice(1)) expect(Math.abs(box.y - boxes[0].y)).toBeLessThan(1);
  const right = Math.max(...boxes.map((b) => b.x + b.width));
  expect(right).toBeLessThanOrEqual(375);
  await page.getByRole("button", { name: "Find Issues" }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: "test-results/mobile-rules-actions.png" });
});
