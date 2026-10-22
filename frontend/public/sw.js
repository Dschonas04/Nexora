// The service worker: Nexora as an app on the phone, and the pages one has
// opened readable without a connection.
//
// Three rules, nothing more:
//   the bundles under /assets/   carry their hash in the name, so they are
//                                taken from the cache once they are there;
//   a few reading API calls      go to the network first and fall back to the
//                                last answer when there is none -- that is
//                                what makes an opened page readable offline;
//   page addresses               go to the network, without one the app shell.
//
// Everything else -- every write, every other call -- goes straight through.
// Nothing is saved offline: a change made without a connection would meet the
// conflict check later and the person would not know which of two states
// counts. The interface says so while offline.
//
// Signing out empties the cache of API answers (see auth.tsx), so the next
// person at the same browser does not read the last one's pages.
const SCHALE = "nexora-schale-v1";
const API = "nexora-api-v1";

// The reading calls whose last answer is worth keeping. A page by id, the
// lists the sidebar is built from, and who one is.
const LESEN = [
  /^\/api\/auth\/me$/,
  /^\/api\/pages$/,
  /^\/api\/pages\/shared$/,
  /^\/api\/pages\/[0-9a-f-]{36}$/,
  /^\/api\/pages\/[0-9a-f-]{36}\/(backlinks|links)$/,
  /^\/api\/(spaces|tags|favorites|design|license|graph)$/,
];

self.addEventListener("install", (e) => {
  e.waitUntil(
    caches
      .open(SCHALE)
      .then((c) => c.addAll(["/index.html"]))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (e) => {
  e.waitUntil(
    caches
      .keys()
      .then((namen) => Promise.all(namen.filter((n) => n !== SCHALE && n !== API).map((n) => caches.delete(n))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("message", (e) => {
  if (e.data === "abgemeldet") e.waitUntil(caches.delete(API));
});

async function netzZuerst(anfrage, cacheName) {
  const cache = await caches.open(cacheName);
  try {
    const antwort = await fetch(anfrage);
    if (antwort.ok) await cache.put(anfrage, antwort.clone());
    // Signed out on the server: what lies in the cache belongs to nobody any
    // more.
    if (antwort.status === 401) await caches.delete(API);
    return antwort;
  } catch (fehler) {
    const alt = await cache.match(anfrage);
    if (alt) return alt;
    throw fehler;
  }
}

self.addEventListener("fetch", (e) => {
  const a = e.request;
  if (a.method !== "GET") return;
  const url = new URL(a.url);
  if (url.origin !== self.location.origin) return;

  if (url.pathname.startsWith("/assets/")) {
    e.respondWith(
      caches.open(SCHALE).then(async (c) => {
        const da = await c.match(a);
        if (da) return da;
        const antwort = await fetch(a);
        if (antwort.ok) await c.put(a, antwort.clone());
        return antwort;
      }),
    );
    return;
  }

  if (url.pathname.startsWith("/api/")) {
    if (url.search === "" && LESEN.some((m) => m.test(url.pathname))) {
      e.respondWith(netzZuerst(a, API));
    }
    return;
  }

  if (a.mode === "navigate") {
    e.respondWith(
      fetch(a)
        .then(async (antwort) => {
          if (antwort.ok) (await caches.open(SCHALE)).put("/index.html", antwort.clone());
          return antwort;
        })
        .catch(async () => (await caches.match("/index.html")) || Response.error()),
    );
  }
});
