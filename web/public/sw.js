const CACHE_VERSION = "v1";
const CACHE_NAME = `float-${CACHE_VERSION}`;
const API_PREFIX = "/float.v1.";

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(["/", "/manifest.json"]))
  );
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(
          keys
            .filter((key) => key.startsWith("float-") && key !== CACHE_NAME)
            .map((key) => caches.delete(key))
        )
      )
      .then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const { request } = event;
  const url = new URL(request.url);

  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith(API_PREFIX)) return;

  // Content-hashed Vite assets: cache-first (immutable by URL)
  if (url.pathname.startsWith("/assets/")) {
    event.respondWith(
      caches.match(request).then((cached) => cached ?? fetchAndCache(request))
    );
    return;
  }

  // Everything else (shell, icons, manifest): network-first with offline fallback
  event.respondWith(networkFirstWithFallback(request));
});

async function fetchAndCache(request) {
  const response = await fetch(request);
  if (response.ok) {
    const cache = await caches.open(CACHE_NAME);
    cache.put(request, response.clone());
  }
  return response;
}

async function networkFirstWithFallback(request) {
  try {
    const response = await fetch(request);
    if (response.ok) {
      const cache = await caches.open(CACHE_NAME);
      cache.put(request, response.clone());
    }
    return response;
  } catch {
    const cached = await caches.match(request);
    if (cached) return cached;
    return new Response(
      `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Float — Offline</title><style>body{font-family:system-ui,sans-serif;background:#1a1a2e;color:#e2e8f0;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}div{text-align:center}h1{font-size:1.5rem;margin-bottom:.5rem}p{color:#94a3b8}</style></head><body><div><h1>Float</h1><p>You are offline. Please reconnect to continue.</p></div></body></html>`,
      { headers: { "Content-Type": "text/html" } }
    );
  }
}
