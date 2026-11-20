// Fixtures for the real-backend e2e suite. Import `test`/`expect` from here.
//
// Every test gets its own floatd (`floatd` fixture): a fresh copy of e2e/data
// in a temp dir, auth enabled with PASSPHRASE, listening on a free port. The
// browser's baseURL points at it and `page` starts logged in (opt out with
// `test.use({ loggedIn: false })`), so `page.goto("/#/transactions")` works.
//
// `floatd` also exposes the backend directly so specs can check what a UI
// action actually did:
//   await floatd.rpc("GetBalances", { query: ["assets:checking"] })
//   floatd.readFile("2026/02.journal")
//   floatd.journalText()   // every *.journal under the data dir, concatenated
//
// Like tests/fixtures.js, a test fails on any uncaught page error,
// console.error, or a LedgerService RPC that returns an error status. Tests
// that trigger errors on purpose can allow them by pattern:
//   test.use({ allowedErrors: [[/unauthenticated/, /status of 401/]] });
import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { cpSync, mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defaultBin } from "./global-setup.js";

export const PASSPHRASE = "e2e-passphrase";

// The fixture ledger covers Jan–Feb 2026, so the browser's clock is pinned to
// its last day: "this month" views (transactions, overview spending) show
// fixture data on any real date. Timers still run normally.
export const FIXTURE_NOW = new Date("2026-02-27T12:00:00");

const fixtureDir = path.join(path.dirname(fileURLToPath(import.meta.url)), "data");
export const filesDir = path.join(path.dirname(fileURLToPath(import.meta.url)), "files");

async function startFloatd() {
  const dataDir = mkdtempSync(path.join(os.tmpdir(), "float-e2e-"));
  cpSync(fixtureDir, dataDir, { recursive: true });
  const bin = process.env.FLOAT_E2E_BIN || defaultBin;
  const proc = spawn(bin, ["--data-dir", dataDir, "--addr", "127.0.0.1:0"], {
    env: { ...process.env, FLOAT_AUTH_PASSPHRASE: PASSPHRASE },
    stdio: ["ignore", "ignore", "pipe"],
  });
  const state = { logs: "", exited: null };
  proc.stderr.setEncoding("utf8");
  proc.stderr.on("data", (chunk) => (state.logs += chunk));
  const exited = new Promise((resolve) =>
    proc.on("exit", (code, signal) => {
      state.exited = { code, signal };
      resolve();
    }),
  );

  const url = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`floatd did not start within 30s\n${state.logs}`)), 30_000);
    const check = () => {
      const m = state.logs.match(/"msg":"floatd listening","addr":"([^"]+)"/);
      if (m) {
        clearTimeout(timer);
        resolve(`http://${m[1]}`);
      }
    };
    proc.stderr.on("data", check);
    proc.on("error", (err) => {
      clearTimeout(timer);
      reject(new Error(`could not start ${bin}: ${err.message} (did global setup build it?)`));
    });
    exited.then(() => {
      clearTimeout(timer);
      reject(new Error(`floatd exited (${JSON.stringify(state.exited)}) before listening\n${state.logs}`));
    });
  });

  async function rpc(method, body = {}) {
    const res = await fetch(`${url}/float.v1.LedgerService/${method}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${PASSPHRASE}` },
      body: JSON.stringify(body),
    });
    const text = await res.text();
    if (!res.ok) throw new Error(`${method} failed: ${res.status} ${text}`);
    return text ? JSON.parse(text) : {};
  }

  function readFile(rel) {
    return readFileSync(path.join(dataDir, rel), "utf8");
  }

  function journalText() {
    const out = [];
    const walk = (dir) => {
      for (const e of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
        if (e.name === ".git") continue;
        const p = path.join(dir, e.name);
        if (e.isDirectory()) walk(p);
        else if (e.name.endsWith(".journal")) out.push(`; ==> ${path.relative(dataDir, p)}\n${readFileSync(p, "utf8")}`);
      }
    };
    walk(dataDir);
    return out.join("\n");
  }

  async function stop() {
    if (!state.exited) {
      proc.kill("SIGTERM");
      const killer = setTimeout(() => proc.kill("SIGKILL"), 10_000);
      await exited;
      clearTimeout(killer);
    }
  }

  return { url, dataDir, rpc, readFile, journalText, stop, get logs() { return state.logs; } };
}

export const test = base.extend({
  allowedErrors: [[], { option: true }],
  loggedIn: [true, { option: true }],

  floatd: async ({}, use, testInfo) => {
    const floatd = await startFloatd();
    await use(floatd);
    await floatd.stop();
    if (testInfo.status !== testInfo.expectedStatus) {
      await testInfo.attach("floatd.log", { body: floatd.logs, contentType: "text/plain" });
      await testInfo.attach("journal", { body: floatd.journalText(), contentType: "text/plain" });
    }
    rmSync(floatd.dataDir, { recursive: true, force: true });
  },

  baseURL: async ({ floatd }, use) => {
    await use(floatd.url);
  },

  page: async ({ page, loggedIn }, use) => {
    trackRpcs(page);
    await page.clock.setFixedTime(FIXTURE_NOW);
    if (loggedIn) {
      const res = await page.request.post("/api/login", { data: { passphrase: PASSPHRASE } });
      expect(res.ok(), "log in via /api/login").toBe(true);
    }
    await use(page);
  },

  browserErrors: [
    async ({ page, allowedErrors }, use) => {
      const errors = [];
      page.on("pageerror", (err) => errors.push(`pageerror: ${err.message}`));
      page.on("console", (msg) => {
        if (msg.type() === "error") errors.push(`console.error: ${msg.text()}`);
      });
      page.on("response", async (res) => {
        if (!res.url().includes("/float.v1.LedgerService/") || res.status() < 400) return;
        const body = await res.text().catch(() => "");
        errors.push(`RPC ${new URL(res.url()).pathname} → ${res.status()} ${body}`);
      });

      await use(errors);

      // test.use reads a two-element array as [value, options], so accept a
      // bare RegExp or a nested array too.
      const allowed = [allowedErrors].flat(2);
      const unexpected = errors.filter((e) => !allowed.some((re) => re.test(e)));
      expect(unexpected, `unexpected browser errors:\n\n${unexpected.join("\n\n")}\n`).toEqual([]);
    },
    { auto: true },
  ],
});

export { expect };

// balanceOf returns an account's amounts from a GetBalances report as
// ["123.45 USD", ...], so a test can assert both the value and that there is
// exactly one commodity.
export async function balanceOf(floatd, account) {
  const res = await floatd.rpc("GetBalances", { query: [`acct:^${account}$`] });
  const row = (res.report?.rows ?? []).find((r) => r.fullName === account);
  return (row?.amounts ?? []).map((a) => `${a.quantity} ${a.commodity ?? ""}`.trim());
}

// In-flight LedgerService requests per page (StreamLogs is a long-lived
// stream, so it is not counted).
const inflight = new WeakMap();

function trackRpcs(page) {
  const pending = new Set();
  inflight.set(page, pending);
  const isRpc = (req) => req.url().includes("/float.v1.LedgerService/") && !req.url().endsWith("/StreamLogs");
  page.on("request", (req) => isRpc(req) && pending.add(req));
  page.on("requestfinished", (req) => pending.delete(req));
  page.on("requestfailed", (req) => pending.delete(req));
}

// settle waits until the page has no loading spinner and no LedgerService
// request has been in flight for `quietMs`. Use it after navigation or an
// action before asserting that something is absent.
export async function settle(page, { quietMs = 300, timeout = 15_000 } = {}) {
  const pending = inflight.get(page);
  const deadline = Date.now() + timeout;
  let quietSince = null;
  while (Date.now() < deadline) {
    const loading = await page.locator('[role="status"][aria-label="Loading"]').count();
    if (pending.size === 0 && loading === 0) {
      quietSince ??= Date.now();
      if (Date.now() - quietSince >= quietMs) return;
    } else {
      quietSince = null;
    }
    await page.waitForTimeout(50);
  }
  throw new Error(`page did not settle within ${timeout}ms (${pending.size} RPCs in flight)`);
}

// pickAccount chooses `account` in the account combobox labelled `label`
// (e.g. "Posting 1 account"), typing to filter like a user would.
export async function pickAccount(page, label, account) {
  await page.getByRole("combobox", { name: label }).click();
  const search = page.locator('[data-slot="popover-content"]:visible [data-slot="command-input"]');
  await search.fill(account);
  await page.locator('[data-slot="popover-content"]:visible').getByRole("option", { name: account, exact: true }).click();
  await expect(page.getByRole("combobox", { name: label })).toContainText(account);
  // Let the popover finish closing so the next pick finds only its own.
  await expect(page.locator('[data-slot="popover-content"]:visible')).toHaveCount(0);
}
