// Fetch and decode the dashboard cube — a precomputed, column-oriented
// snapshot of the ledger built once per txlock generation.
//
// The payload is laid out so every column can be exposed as a typed-array view
// over the same ArrayBuffer, with no per-value decode pass:
//
//   "FLTCUBE1" | uint32 headerLen | JSON header | pad | columns
//
// Column offsets in the header are relative to the start of the data section,
// which begins at the first 8-byte boundary at or after the header. Every
// column is 8-byte aligned because Float64Array construction throws on a
// misaligned byte offset.

export const CUBE_MAGIC = "FLTCUBE1";

const MAGIC_LEN = CUBE_MAGIC.length;
const HEADER_PREFIX_LEN = MAGIC_LEN + 4;
const COLUMN_ALIGN = 8;

const VIEWS = {
  u16: Uint16Array,
  u32: Uint32Array,
  f64: Float64Array,
};

const align = (n) => (n % COLUMN_ALIGN === 0 ? n : n + COLUMN_ALIGN - (n % COLUMN_ALIGN));

/** Thrown when the server is on a different generation than the one requested. */
export class StaleGenerationError extends Error {
  constructor(generation) {
    super(`cube generation is stale; server is on ${generation}`);
    this.name = "StaleGenerationError";
    this.generation = generation;
  }
}

/** Reads the server's current txlock generation. */
export async function fetchGeneration() {
  const res = await fetch("/api/generation", { headers: { Accept: "application/json" } });
  if (!res.ok) throw new Error(`could not read generation: HTTP ${res.status}`);
  const body = await res.json();
  return body.generation;
}

/**
 * Fetches and decodes the cube for one generation.
 *
 * The URL is content-addressed by generation and served `immutable`, so a
 * repeat load for the same generation is a browser cache hit and never touches
 * the server. A 409 means a write landed first; the error carries the current
 * generation so the caller can move to it.
 */
export async function loadCube(generation) {
  const res = await fetch(`/api/cube/${generation}.bin`);
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    throw new StaleGenerationError(body.generation);
  }
  if (!res.ok) throw new Error(`could not load cube: HTTP ${res.status}`);
  return decodeCube(await res.arrayBuffer());
}

/** Decodes a cube payload into header metadata plus typed-array column views. */
export function decodeCube(buffer) {
  const bytes = new Uint8Array(buffer);
  const magic = new TextDecoder().decode(bytes.subarray(0, MAGIC_LEN));
  if (magic !== CUBE_MAGIC) {
    throw new Error(`not a cube payload (magic ${JSON.stringify(magic)})`);
  }

  const headerLen = new DataView(buffer).getUint32(MAGIC_LEN, true);
  const header = JSON.parse(
    new TextDecoder().decode(bytes.subarray(HEADER_PREFIX_LEN, HEADER_PREFIX_LEN + headerLen)),
  );
  const dataStart = align(HEADER_PREFIX_LEN + headerLen);

  const tables = {};
  for (const [name, table] of Object.entries(header.tables)) {
    const columns = {};
    const summable = {};
    for (const [colName, col] of Object.entries(table.columns)) {
      const View = VIEWS[col.type];
      if (!View) throw new Error(`unknown column type ${col.type} in ${name}.${colName}`);
      columns[colName] = new View(buffer, dataStart + col.offset, table.rows);
      if (col.summable) summable[colName] = col.summable;
    }
    tables[name] = { rows: table.rows, sortedBy: table.sortedBy, columns, summable };
  }

  return {
    generation: header.generation,
    builtAt: header.builtAt,
    configHash: header.configHash,
    reportingCurrency: header.reportingCurrency,
    epochDate: header.epochDate,
    accounts: header.accounts,
    accountPaths: header.accounts.map((a) => a.path),
    payees: header.payees,
    commodities: header.commodities,
    periods: header.periods,
    tables,
    // Lazily-built indexes, populated by cube-query.js on first use.
    _cache: new Map(),
  };
}

/**
 * Converts a minor-unit amount to major units for display.
 *
 * Aggregation happens in minor units, where a Float64Array holds integers
 * exactly below 2^53; only the final display value is scaled.
 */
export function toMajor(cube, commodityIndex, minor) {
  const scale = cube.commodities[commodityIndex]?.scale ?? 0;
  return minor / 10 ** scale;
}

/** Converts a minor-unit amount to major units by commodity code. */
export function toMajorByCode(cube, code, minor) {
  const i = cube.commodities.findIndex((c) => c.code === code);
  return toMajor(cube, i < 0 ? 0 : i, minor);
}
