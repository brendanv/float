// Shared Playwright test fixture. Every spec should import `test`/`expect`
// from here instead of "@playwright/test" so that a page which crashes or
// logs errors fails the test rather than producing a broken screenshot.
//
// After each test this fails if the page:
//   - threw an uncaught exception (`pageerror`)
//   - logged via console.error (React render errors, failed queries, etc.)
//   - is showing the Vite error overlay
//   - called a LedgerService RPC that tests/mock-api.js does not mock
//   - received a mocked LedgerService response that doesn't decode as the
//     method's proto response message (unknown field, wrong type, a oneof
//     written as {case, value}, ...), so mocks can't drift from the protos
//
// Tests that deliberately trigger errors can allow them by pattern:
//   test.use({ allowedErrors: [/status of 400/] });
import { test as base, expect } from "@playwright/test";
import { fromJson } from "@bufbuild/protobuf";
import { LedgerService } from "../src/gen/float/v1/ledger_pb.js";
import { unmockedCalls } from "./mock-api.js";

const rpcMethods = new Map(LedgerService.methods.map((m) => [m.name, m]));

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
      page.on("response", (res) => {
        if (res.url().includes("/float.v1.LedgerService/")) pending.push(checkRpcResponse(res).then((e) => e && errors.push(e)));
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

// checkRpcResponse decodes a successful LedgerService response with the
// method's generated schema (strict: unknown fields are errors) and returns an
// error string if it doesn't match, else null.
async function checkRpcResponse(res) {
  if (res.status() !== 200) return null;
  const name = new URL(res.url()).pathname.split("/").pop();
  const method = rpcMethods.get(name);
  if (!method) return `RPC ${name} is not a LedgerService method in the generated code`;
  let body;
  try {
    body = await res.body();
  } catch {
    return null; // page closed before the body was read
  }
  try {
    const contentType = res.headers()["content-type"] ?? "";
    const messages = contentType.startsWith("application/connect+json") ? decodeEnvelopes(body) : [JSON.parse(body.toString("utf8") || "{}")];
    messages.forEach((m) => fromJson(method.output, m));
    return null;
  } catch (err) {
    return `mock ${name} response doesn't match ${method.output.typeName}: ${err.message}`;
  }
}

// decodeEnvelopes returns the JSON messages of a Connect streaming body,
// skipping the end-of-stream frame.
function decodeEnvelopes(buf) {
  const out = [];
  for (let i = 0; i < buf.length; ) {
    const flags = buf[i];
    const len = buf.readUInt32BE(i + 1);
    const data = buf.subarray(i + 5, i + 5 + len).toString("utf8");
    if ((flags & 0x02) === 0) out.push(JSON.parse(data));
    i += 5 + len;
  }
  return out;
}

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
