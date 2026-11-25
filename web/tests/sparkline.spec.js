import { expect, test } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

test("net worth sparkline shows past values on hover and arrow keys", async ({ page }) => {
  await mockLedgerApi(page);
  await page.goto("/#/");
  const hero = page.locator('[data-slot="metric-hero"]');
  const chart = hero.getByRole("img", { name: /^Net worth,/ });
  await expect(chart).toBeVisible();
  const readout = hero.locator('[data-slot="sparkline-readout"]');
  await expect(readout).toHaveCount(0);

  const box = await chart.boundingBox();
  await page.mouse.move(box.x + 2, box.y + box.height / 2);
  await expect(readout).toBeVisible();
  const first = await readout.textContent();
  expect(first).toMatch(/’\d\d/);
  expect(first).toMatch(/\$/);

  await page.mouse.move(box.x + box.width - 2, box.y + box.height / 2);
  await expect(readout).not.toHaveText(first);

  await page.mouse.move(box.x, box.y - 200);
  await expect(readout).toHaveCount(0);

  await chart.locator("..").focus();
  await expect(readout).toBeVisible();
  const last = await readout.textContent();
  await page.keyboard.press("ArrowLeft");
  await expect(readout).not.toHaveText(last);
});
