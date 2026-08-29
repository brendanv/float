// Unit tests for the cube decoder and client query engine.
//
// These use the Playwright runner as a plain test runner — no browser, no
// server, no new dependency. The fixture is a payload internal/cube actually
// encoded, so this is a cross-language contract test: if the Go writer and the
// JS reader disagree about the layout, it fails here.
//
// Regenerate the fixture after any wire-format change:
//   FLOAT_WRITE_CUBE_FIXTURE=1 go test ./internal/cube/ -run TestWriteWebFixture
//
// Expected values come from running hledger against
// internal/cube/testdata/simple.journal.

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { test, expect } from "@playwright/test";

import { decodeCube, CUBE_MAGIC, toMajorByCode } from "../src/lib/cube.js";
import {
  assertSummableOverTime,
  balanceAt,
  balanceSeries,
  dateRange,
  flowByAccount,
  flowSeries,
  flowSums,
  flowTotal,
  latestPeriod,
  periodsInRange,
  typeMatches,
} from "../src/lib/cube-query.js";

const here = dirname(fileURLToPath(import.meta.url));

function loadFixture() {
  const buf = readFileSync(join(here, "fixtures", "cube.bin"));
  // Copy into a standalone ArrayBuffer: Node pools Buffer storage, so the
  // underlying buffer is usually offset and typed-array views would misalign.
  const copy = new ArrayBuffer(buf.byteLength);
  new Uint8Array(copy).set(buf);
  return decodeCube(copy);
}

test.describe("decode", () => {
  test("reads the header and exposes typed-array columns", () => {
    const cube = loadFixture();
    expect(cube.reportingCurrency).toBe("USD");
    expect(cube.epochDate).toBe("2023-01-05");
    expect(cube.accountPaths).toContain("expenses:food:groceries");
    expect(cube.payees).toContain("Corner Store");
    expect(cube.commodities).toEqual([{ code: "USD", scale: 2 }]);

    const postings = cube.tables.postings;
    expect(postings.rows).toBeGreaterThan(0);
    expect(postings.columns.date).toBeInstanceOf(Uint16Array);
    expect(postings.columns.account).toBeInstanceOf(Uint32Array);
    expect(postings.columns.amount).toBeInstanceOf(Float64Array);
    expect(postings.columns.amount.length).toBe(postings.rows);
  });

  test("rejects a payload that is not a cube", () => {
    const junk = new ArrayBuffer(64);
    expect(() => decodeCube(junk)).toThrow(/not a cube payload/);
  });

  test("magic matches the encoder", () => {
    expect(CUBE_MAGIC).toBe("FLTCUBE1");
  });

  test("postings are sorted by date, which the range search depends on", () => {
    const { columns, rows } = loadFixture().tables.postings;
    for (let i = 1; i < rows; i++) {
      expect(columns.date[i]).toBeGreaterThanOrEqual(columns.date[i - 1]);
    }
  });
});

test.describe("flow measures", () => {
  test("totals an account subtree", () => {
    const cube = loadFixture();
    expect(flowTotal(cube, { account: "expenses" })).toBeCloseTo(3453.09, 2);
    expect(flowTotal(cube, { account: "expenses:food" })).toBeCloseTo(503.09, 2);
    expect(flowTotal(cube, { account: "expenses:home" })).toBeCloseTo(2950.0, 2);
  });

  test("account matching is a tree match, not a substring match", () => {
    const cube = loadFixture();
    // "expenses:hom" is a string prefix of "expenses:home" but not an ancestor.
    expect(flowTotal(cube, { account: "expenses:hom" })).toBe(0);
    expect(flowTotal(cube, { account: "expenses:nope" })).toBe(0);
  });

  test("the whole journal balances to zero", () => {
    const cube = loadFixture();
    for (const total of flowSums(cube, {}).values()) {
      expect(total).toBeCloseTo(0, 6);
    }
  });

  test("filters by a half-open date range", () => {
    const cube = loadFixture();
    expect(
      flowTotal(cube, { account: "expenses", from: "2023-01-01", to: "2023-02-01" }),
    ).toBeCloseTo(1532.35, 2);
  });

  test("the upper date bound is exclusive", () => {
    const cube = loadFixture();
    // A transaction lands on 2023-01-07. As an end date it must be excluded,
    // as a start date included. Getting this backwards shifts every month
    // boundary by a day and is invisible without a case like this one.
    expect(
      flowTotal(cube, { account: "expenses", from: "2023-01-01", to: "2023-01-07" }),
    ).toBe(0);
    expect(
      flowTotal(cube, { account: "expenses", from: "2023-01-07", to: "2023-01-21" }),
    ).toBeCloseTo(1532.35, 2);
  });

  test("ranges outside the journal are empty, not wrapped", () => {
    const cube = loadFixture();
    expect(flowTotal(cube, { account: "expenses", from: "2000-01-01", to: "2001-01-01" })).toBe(0);
    expect(flowTotal(cube, { account: "expenses", from: "2090-01-01", to: "2091-01-01" })).toBe(0);
    expect(flowTotal(cube, { account: "expenses", from: "2023-06-01", to: "2023-05-01" })).toBe(0);
  });

  test("filters by payee, honouring the payee | note split", () => {
    const cube = loadFixture();
    expect(flowTotal(cube, { account: "expenses", payee: "Corner Store" })).toBeCloseTo(338.84, 2);
    expect(flowTotal(cube, { account: "expenses", payee: "Nobody" })).toBe(0);
  });

  test("groups into a monthly series covering empty months", () => {
    const cube = loadFixture();
    const series = flowSeries(cube, { account: "expenses", from: "2023-01-01", to: "2023-06-01" });
    expect(series.map((p) => p.period)).toEqual([
      "2023-01",
      "2023-02",
      "2023-03",
      "2023-04",
      "2023-05",
    ]);
    expect(series[0].total).toBeCloseTo(1532.35, 2);
    // April has no activity at all and must still appear, so a chart shows the
    // gap rather than closing it.
    expect(series[3].total).toBe(0);
    expect(series[4].total).toBeCloseTo(101.25, 2);
  });

  test("groups by account subtree at a depth", () => {
    const cube = loadFixture();
    const rows = flowByAccount(cube, { account: "expenses" }, 2);
    expect(rows.map((r) => r.account)).toEqual(["expenses:home", "expenses:food"]);
    expect(rows[0].total).toBeCloseTo(2950.0, 2);
    expect(rows[1].total).toBeCloseTo(503.09, 2);
  });

  test("resolves a date range to a contiguous row span", () => {
    const cube = loadFixture();
    const [start, end] = dateRange(cube, "2023-01-01", "2023-02-01");
    expect(end).toBeGreaterThan(start);
    const [emptyStart, emptyEnd] = dateRange(cube, "2023-04-01", "2023-05-01");
    expect(emptyEnd).toBe(emptyStart);
  });
});

test.describe("account types", () => {
  test("filters by hledger account type", () => {
    const cube = loadFixture();
    // Same totals as the account-name filters, reached through type: instead.
    expect(flowTotal(cube, { type: "X" })).toBeCloseTo(3453.09, 2);
    expect(flowTotal(cube, { type: "R" })).toBeCloseTo(-15200.0, 2);
    expect(flowTotal(cube, { type: "L" })).toBeCloseTo(-503.09, 2);
  });

  test("type:A includes cash accounts", () => {
    const cube = loadFixture();
    // hledger's own inference types a plainly-named "assets:checking" as C, so
    // exact matching would report zero assets here.
    expect(cube.accounts.find((a) => a.path === "assets:checking").type).toBe("C");
    expect(flowTotal(cube, { type: "A" })).toBeCloseTo(12250.0, 2);
    expect(flowTotal(cube, { type: "A" })).toBeCloseTo(flowTotal(cube, { account: "assets" }), 6);
  });

  test("the subtype relation is directional", () => {
    expect(typeMatches("C", "A")).toBe(true);
    expect(typeMatches("A", "C")).toBe(false);
    expect(typeMatches("V", "E")).toBe(true);
    expect(typeMatches("X", "X")).toBe(true);
    expect(typeMatches("X", "R")).toBe(false);
  });

  test("combines a type with an account subtree", () => {
    const cube = loadFixture();
    expect(flowTotal(cube, { type: "X", account: "expenses:food" })).toBeCloseTo(503.09, 2);
    expect(flowTotal(cube, { type: "R", account: "expenses" })).toBe(0);
  });
});

test.describe("stock measures", () => {
  test("rolls up the account tree at one period", () => {
    const cube = loadFixture();
    // assets 7050.00 + 500.00, liabilities -308.44
    expect(balanceAt(cube, "valued", "2023-03", "assets")).toBeCloseTo(7550.0, 2);
    expect(balanceAt(cube, "valued", "2023-03", "liabilities")).toBeCloseTo(-308.44, 2);
    expect(balanceAt(cube, "valued", "2023-03", "")).toBeCloseTo(7241.56, 2);
  });

  test("returns a series of independent instants, not a running sum", () => {
    const cube = loadFixture();
    const series = balanceSeries(cube, "valued", "assets");
    const byPeriod = Object.fromEntries(series.map((p) => [p.period, p.total]));
    expect(byPeriod["2023-03"]).toBeCloseTo(7550.0, 2);
    // A historical balance carries forward through a month with no activity,
    // which a sum of deltas would report as zero.
    expect(byPeriod["2023-04"]).toBeCloseTo(7550.0, 2);
  });

  test("an unknown period yields zero rather than throwing", () => {
    const cube = loadFixture();
    expect(balanceAt(cube, "valued", "1999-01", "assets")).toBe(0);
  });

  test("refuses to be summed across periods", () => {
    const cube = loadFixture();
    // This is the guard that stops a caller quietly producing a wrong net
    // worth by treating market value as a flow.
    expect(() => assertSummableOverTime(cube, "valued", "amount")).toThrow(/cannot be summed/);
    expect(() => assertSummableOverTime(cube, "cost", "amount")).toThrow(/cannot be summed/);
    expect(() => assertSummableOverTime(cube, "postings", "amount")).not.toThrow();
  });
});

test.describe("periods", () => {
  test("filters period labels by a half-open range", () => {
    const cube = loadFixture();
    expect(periodsInRange(cube, "2023-02-01", "2023-04-01")).toEqual(["2023-02", "2023-03"]);
    // An end date mid-month still includes that month.
    expect(periodsInRange(cube, "2023-02-01", "2023-04-15")).toEqual([
      "2023-02",
      "2023-03",
      "2023-04",
    ]);
  });

  test("reports the latest period", () => {
    expect(latestPeriod(loadFixture())).toBe("2024-02");
  });
});

test.describe("amount scaling", () => {
  test("converts minor units to major units by commodity scale", () => {
    const cube = loadFixture();
    expect(toMajorByCode(cube, "USD", 8235)).toBeCloseTo(82.35, 2);
    expect(toMajorByCode(cube, "USD", -145000)).toBeCloseTo(-1450.0, 2);
  });
});
