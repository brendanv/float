// float's service worker. It makes the web UI installable as an app and lets
// the app shell open without a network; it never caches API traffic (RPCs,
// /api/*), so ledger data is always fetched live from floatd.
//
// - Navigations to the app (/ or /index.html): network first, falling back to
//   the last cached shell when floatd is unreachable.
// - /assets/*: Vite's content-hashed bundles, cache first.
// - Everything else passes through untouched.

const CACHE = "float-shell-v1";
const SHELL = "/";

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE).then((cache) => cache.add(SHELL)).then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

async function networkFirstShell(request) {
  const cache = await caches.open(CACHE);
  try {
    const response = await fetch(request);
    if (response.ok) {
      // A new shell references new hashed bundles: drop the old ones.
      await pruneAssets(cache, await response.clone().text());
      await cache.put(SHELL, response.clone());
    }
    return response;
  } catch (err) {
    const cached = await cache.match(SHELL);
    if (cached) return cached;
    throw err;
  }
}

// pruneAssets removes cached /assets/* entries the shell doesn't link to
// directly so the cache doesn't grow across upgrades; lazily loaded chunks
// are re-cached the next time they load.
async function pruneAssets(cache, html) {
  const keep = new Set(Array.from(html.matchAll(/\/assets\/[^"'\s)]+/g), (m) => m[0]));
  if (keep.size === 0) return;
  for (const request of await cache.keys()) {
    const path = new URL(request.url).pathname;
    if (path.startsWith("/assets/") && !keep.has(path)) await cache.delete(request);
  }
}

async function cacheFirst(request) {
  const cache = await caches.open(CACHE);
  const cached = await cache.match(request);
  if (cached) return cached;
  const response = await fetch(request);
  if (response.ok) await cache.put(request, response.clone());
  return response;
}

self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.method !== "GET") return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;

  if (request.mode === "navigate" && (url.pathname === "/" || url.pathname === "/index.html")) {
    event.respondWith(networkFirstShell(request));
  } else if (url.pathname.startsWith("/assets/")) {
    event.respondWith(cacheFirst(request));
  }
});
