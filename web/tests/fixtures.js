// Shared Playwright test fixture. Every spec should import `test`/`expect`
// from here instead of "@playwright/test" so that a page which crashes or
// logs errors fails the test rather than producing a broken screenshot.
//
// After each test this fails if the page:
//   - threw an uncaught exception (`pageerror`)
//   - logged via console.error (React render errors, failed queries, etc.)
//   - is showing the Vite error overlay
//   - called a LedgerService RPC that tests/mock-api.js does not mock
//
// Tests that deliberately trigger errors can allow them by pattern:
//   test.use({ allowedErrors: [/status of 400/] });
import { test as base, expect } from "@playwright/test";
import { unmockedCalls } from "./mock-api.js";

export const test = base.extend({
  allowedErrors: [[], { option: true }],

  browserErrors: [
    async ({ page, allowedErrors }, use) => {
      const errors = [];
      page.on("pageerror", (err) => errors.push(`pageerror: ${err.message}`));
      const pending = [];
      page.on("console", (msg) => {
        if (msg.type() === "error") pending.push(formatConsoleMessage(msg).then((t) => errors.push(`console.error: ${t}`)));
      });

      await use(errors);
      await Promise.all(pending);

      if (!page.isClosed() && (await page.locator("vite-error-overlay").count()) > 0) {
        errors.push("vite-error-overlay is visible");
      }
      for (const method of unmockedCalls.get(page) ?? []) {
        errors.push(`unmocked RPC: ${method} (add a case to tests/mock-api.js)`);
      }

      const unexpected = errors.filter((e) => !allowedErrors.some((re) => re.test(e)));
      expect(unexpected, `unexpected browser errors:\n\n${unexpected.join("\n\n")}\n`).toEqual([]);
    },
    { auto: true },
  ],
});

// React logs warnings as console.error("Warning: ... %s", arg, ...), and
// msg.text() leaves the placeholders unfilled. Substitute the args so the
// failure names the offending component.
async function formatConsoleMessage(msg) {
  const [first, ...rest] = await Promise.all(
    msg.args().map((a) => a.jsonValue().catch(() => "<unserializable>")),
  );
  if (typeof first !== "string" || !first.includes("%")) return msg.text();
  let i = 0;
  const out = first.replace(/%[sdoifO]/g, () => (i < rest.length ? String(rest[i++]) : ""));
  return [out, ...rest.slice(i).map(String)].join(" ").trim();
}

export { expect };
