import { test, expect } from "./fixtures.js";
import { mockLedgerApi } from "./mock-api.js";

// On touch devices every interactive control must offer at least a 40px hit
// area (WCAG 2.5.8 recommends 24px minimum; 40px+ is the platform norm). The
// shadcn primitives grow via `pointer-coarse:min-h-10`; small inline controls
// use the `touch-target` utility from style.css, which extends the hit area
// with an absolutely-positioned ::after, so that is counted here too.
//
// Note: measure before any fullPage screenshot — in Chromium a fullPage
// screenshot resets touch emulation, so (pointer: coarse) stops matching.
test.use({ viewport: { width: 375, height: 740 }, hasTouch: true, isMobile: true });

test.beforeEach(async ({ page }) => {
  await mockLedgerApi(page);
});

const MIN = 40;

const routes = [
  "/", "/transactions", "/add", "/trends", "/monthly", "/prices", "/accounts",
  "/assertions", "/transfers", "/snapshots", "/import", "/rules", "/imports", "/payees",
  "/portfolio", "/lots", "/settings", "/hledger-query", "/connections", "/templates",
];

for (const route of routes) {
  test(`tap targets are at least ${MIN}px on ${route}`, async ({ page }) => {
    await page.goto(`/#${route}`);
    await page.waitForLoadState("networkidle");
    await page.locator("[data-slot=sidebar-inset] main, [data-slot=sidebar-inset]").first().waitFor();
    await page.waitForTimeout(300);

    expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);

    const tooSmall = await page.evaluate((min) => {
      const selector = [
        "button", "a[href]", "input:not([type=hidden])", "select", "textarea",
        "[role=button]", "[role=checkbox]", "[role=combobox]", "[role=tab]",
      ].join(",");
      const out = [];
      for (const el of document.querySelectorAll(selector)) {
        const box = el.getBoundingClientRect();
        if (box.width <= 1 || box.height <= 1) continue; // hidden / sr-only
        // Links inside running text are exempt (WCAG 2.5.8 "inline" exception):
        // an <a> whose parent also holds non-whitespace text nodes.
        if (
          el.tagName === "A" &&
          [...(el.parentElement?.childNodes ?? [])].some(
            (n) => n.nodeType === Node.TEXT_NODE && n.textContent.trim(),
          )
        ) continue;
        let { width, height } = box;
        const after = getComputedStyle(el, "::after");
        if (after.content !== "none" && after.position === "absolute") {
          width = Math.max(width, parseFloat(after.width) || 0);
          height = Math.max(height, parseFloat(after.height) || 0);
        }
        if (width < min - 0.5 || height < min - 0.5) {
          const label = (el.getAttribute("aria-label") || el.textContent || el.getAttribute("placeholder") || "").trim().slice(0, 40);
          out.push(`${el.tagName.toLowerCase()} ${Math.round(width)}x${Math.round(height)} "${label}"`);
        }
      }
      return out;
    }, MIN);

    expect(tooSmall, `controls smaller than ${MIN}px on ${route}`).toEqual([]);
  });
}
