// Tracks the server's txlock generation, which identifies the current cube.
//
// Every Connect response carries X-Float-Generation, so an ordinary RPC — a
// transaction list, a settings read, the mutation the user just performed —
// keeps this current with no polling and no extra round trip. React components
// subscribe through useSyncExternalStore in hooks/use-cube.js.

export const GENERATION_HEADER = "x-float-generation";

let current = null;
const listeners = new Set();

/** Returns the last observed generation, or null before the first response. */
export function getGeneration() {
  return current;
}

/**
 * Records a newly observed generation. Ignores anything not newer, so a
 * response that raced with a write cannot move the client backwards.
 */
export function setGeneration(next) {
  if (next == null || Number.isNaN(next)) return;
  if (current !== null && next <= current) return;
  current = next;
  for (const listener of listeners) listener();
}

export function subscribeGeneration(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Reads the generation header off a Connect response's header bag. */
export function observeHeaders(headers) {
  const raw = headers?.get?.(GENERATION_HEADER);
  if (raw != null) setGeneration(Number(raw));
}
