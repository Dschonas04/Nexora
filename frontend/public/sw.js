// The service worker: Nexora as an app on the phone, and working on without a
// connection to the server.
//
// Rules:
//   the bundles under /assets/   carry their hash in the name, so they are
//                                taken from the cache once they are there;
//   the reading API calls        go to the network first and fall back to the
//                                last answer when the server does not answer;
//   page addresses               go to the network, without one the app shell;
//   writes on pages              (create, change, width, delete) go to the
//                                network; when the server is not reachable
//                                they are applied to the kept state and put in
//                                an outbox.
//
// "Not reachable" is not the same as "offline": a phone on mobile data is
// online, but the server in the home network does not answer. So every call
// has a time limit, and after a miss the worker answers from the cache for a
// while without trying first.
//
// Back in reach, the outbox goes out before anything else. A page changed on
// both sides is merged block by block: what changed on one side comes from
// there; a block changed on both keeps the server's version with one's own
// copy directly below it. Nothing is lost, at worst a paragraph stands twice.
//
// The mirror: when asked ("spiegeln"), the worker fetches all pages one can
// see that changed since the last time, plus every bundle of the interface,
// so the app works offline even for pages never opened on this device.
//
// Signing out empties the cache of API answers (see auth.tsx), so the next
// person at the same browser does not read the last one's pages. The outbox
// stays, bound to the account that wrote it, and is sent only with that one.
const SCHALE = "nexora-schale-v1";
const API = "nexora-api-v1";
const OFFLINE = "nexora-offline-v1";
const ZEITLIMIT = 6000;
const WEG_DAUER = 20000;

// The reading calls whose last answer is worth keeping.
const LESEN = [
  /^\/api\/auth\/me$/,
  /^\/api\/pages$/,
  /^\/api\/pages\/shared$/,
  /^\/api\/pages\/[0-9a-f-]{36}$/,
  /^\/api\/pages\/[0-9a-f-]{36}\/(backlinks|links)$/,
  /^\/api\/(spaces|tags|favorites|design|license|graph)$/,
];
const SEITE = /^\/api\/pages\/([0-9a-f-]{36})$/;
const BREITE = /^\/api\/pages\/([0-9a-f-]{36})\/width$/;
const FELDER = ["title", "content", "icon"];

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
      .then((namen) =>
        Promise.all(
          namen.filter((n) => n !== SCHALE && n !== API && n !== OFFLINE).map((n) => caches.delete(n)),
        ),
      )
      .then(() => self.clients.claim()),
  );
});

// ------------------------------------------------------------------ helpers --

const voll = (pfad) => new URL(pfad, self.location.origin).href;

// One thing at a time on the kept state: two writes arriving together must not
// both read the outbox, change it and write it back.
let kette = Promise.resolve();
function exklusiv(f) {
  const p = kette.then(f, f);
  kette = p.catch(() => {});
  return p;
}

async function jsonLesen(cacheName, pfad) {
  const r = await (await caches.open(cacheName)).match(voll(pfad), { ignoreVary: true });
  if (!r) return undefined;
  try {
    return await r.json();
  } catch {
    return undefined;
  }
}

async function jsonSchreiben(cacheName, pfad, wert) {
  await (await caches.open(cacheName)).put(
    voll(pfad),
    new Response(JSON.stringify(wert), { headers: { "Content-Type": "application/json" } }),
  );
}

const ausgangLesen = async () => (await jsonLesen(OFFLINE, "/__ausgang")) || [];
const ausgangSchreiben = (a) => jsonSchreiben(OFFLINE, "/__ausgang", a);

function antwort(status, wert) {
  return new Response(wert === undefined ? null : JSON.stringify(wert), {
    status,
    headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
  });
}
const nichtOffline = () =>
  antwort(503, { error: "Offline: this works again once the server is reachable." });

let wegSeit = 0;
const serverWeg = () => wegSeit !== 0 && Date.now() - wegSeit < WEG_DAUER;

async function melden(nachricht) {
  for (const c of await self.clients.matchAll({ includeUncontrolled: true })) c.postMessage(nachricht);
}
async function zustandMelden() {
  const a = await ausgangLesen();
  await melden({ art: "nexora-offline", offline: serverWeg(), wartend: a.length });
}
function weg() {
  const neu = !serverWeg();
  wegSeit = Date.now();
  if (neu) zustandMelden();
}
function erreicht() {
  if (wegSeit) {
    wegSeit = 0;
    zustandMelden();
  }
}

const mitZeitlimit = (anfrage, init = {}) => fetch(anfrage, { ...init, signal: AbortSignal.timeout(ZEITLIMIT) });

async function api(pfad, init = {}) {
  const r = await fetch(voll("/api" + pfad), {
    credentials: "include",
    ...init,
    headers: { "Content-Type": "application/json" },
    signal: AbortSignal.timeout(15000),
  });
  const text = await r.text();
  let daten;
  try {
    daten = text ? JSON.parse(text) : undefined;
  } catch {
    daten = text;
  }
  if (!r.ok) {
    const f = new Error((daten && daten.error) || r.statusText);
    f.status = r.status;
    throw f;
  }
  return daten;
}

// ------------------------------------------------------------------ merging --

const gleich = (a, b) => JSON.stringify(a) === JSON.stringify(b);

function kopie(block) {
  const k = JSON.parse(JSON.stringify(block));
  const umbenennen = (b) => {
    if (b && typeof b === "object") {
      if (typeof b.id === "string") b.id = crypto.randomUUID();
      if (Array.isArray(b.children)) b.children.forEach(umbenennen);
    }
  };
  umbenennen(k);
  return k;
}

const bloecke = (x) => (Array.isArray(x) ? x : []);

function inhaltZusammenfuehren(basis, lokal, server) {
  if (gleich(lokal, basis)) return { inhalt: server, konflikte: 0 };
  if (gleich(server, basis) || gleich(server, lokal)) return { inhalt: lokal, konflikte: 0 };
  if (!Array.isArray(lokal) || !Array.isArray(server)) return { inhalt: lokal, konflikte: 1 };
  const b = new Map(bloecke(basis).map((x) => [x.id, x]));
  const l = new Map(bloecke(lokal).map((x) => [x.id, x]));
  const s = new Map(bloecke(server).map((x) => [x.id, x]));
  let konflikte = 0;
  const ergebnis = [];
  for (const sb of server) {
    const bb = b.get(sb.id);
    const lb = l.get(sb.id);
    if (!lb) {
      // Deleted here. If the server changed it meanwhile, it stays.
      if (bb && gleich(sb, bb)) continue;
      ergebnis.push(sb);
    } else if (!bb) {
      ergebnis.push(sb);
      if (!gleich(lb, sb)) {
        ergebnis.push(kopie(lb));
        konflikte++;
      }
    } else {
      const hier = !gleich(lb, bb);
      const dort = !gleich(sb, bb);
      if (hier && dort && !gleich(lb, sb)) {
        ergebnis.push(sb, kopie(lb));
        konflikte++;
      } else ergebnis.push(hier ? lb : sb);
    }
  }
  // Blocks new here go behind their predecessor from here.
  lokal.forEach((lb, i) => {
    if (s.has(lb.id)) return;
    if (b.has(lb.id) && gleich(lb, b.get(lb.id))) return; // deleted there, unchanged here
    let pos = 0;
    for (let j = i - 1; j >= 0; j--) {
      const k = ergebnis.findIndex((x) => x.id === lokal[j].id);
      if (k >= 0) {
        pos = k + 1;
        break;
      }
    }
    ergebnis.splice(pos, 0, lb);
  });
  return { inhalt: ergebnis, konflikte };
}

function feldZusammenfuehren(basis, lokal, server) {
  if (gleich(lokal, basis)) return { wert: server, konflikt: false };
  if (gleich(server, basis) || gleich(server, lokal)) return { wert: lokal, konflikt: false };
  return { wert: lokal, konflikt: true };
}

// --------------------------------------------------------------- the mirror --

async function schaleSpiegeln() {
  const r = await fetch(voll("/"), { signal: AbortSignal.timeout(15000) });
  if (!r.ok) return;
  const index = await r.text();
  const schale = await caches.open(SCHALE);
  await schale.put(voll("/index.html"), new Response(index, { headers: { "Content-Type": "text/html" } }));
  const muster = /["'(/]((?:assets\/)[A-Za-z0-9_.-]+\.(?:js|css|woff2?|ttf|svg|png|webp))/g;
  const offen = new Set();
  const merken = (text) => {
    for (const m of text.matchAll(muster)) offen.add(m[1]);
  };
  merken(index);
  const fertig = new Set();
  while (offen.size) {
    const datei = offen.values().next().value;
    offen.delete(datei);
    if (fertig.has(datei)) continue;
    fertig.add(datei);
    try {
      let da = await schale.match(voll("/" + datei));
      if (!da) {
        const neu = await fetch(voll("/" + datei), { signal: AbortSignal.timeout(15000) });
        if (!neu.ok) continue;
        await schale.put(voll("/" + datei), neu.clone());
        da = neu;
      }
      if (/\.(js|css)$/.test(datei)) merken(await da.text());
    } catch {
      // A missing font must not stop the mirror.
    }
  }
}

async function spiegelnRoh() {
  const ich = await api("/auth/me");
  const ausgang = await ausgangLesen();
  const besitzer = ausgang.find((x) => x.konto);
  // Changes of another account are waiting: their pages stay as they are.
  if (besitzer && besitzer.konto !== ich.id) return;
  const altIch = await jsonLesen(API, "/api/auth/me");
  if (altIch && altIch.id !== ich.id) await caches.delete(API);
  await jsonSchreiben(API, "/api/auth/me", ich);
  for (const p of ["/spaces", "/tags", "/favorites", "/design", "/license", "/graph", "/pages/shared"]) {
    try {
      await jsonSchreiben(API, "/api" + p, await api(p));
    } catch {
      // Not every instance has every list; what is missing stays missing.
    }
  }
  const liste = await api("/pages");
  const geteilt = bloecke(await jsonLesen(API, "/api/pages/shared"));
  const alle = [...bloecke(liste), ...geteilt];
  const noetig = [];
  for (const m of alle) {
    const alt = await jsonLesen(API, `/api/pages/${m.id}`);
    if (!alt || alt.updatedAt !== m.updatedAt) noetig.push(m);
  }
  for (let i = 0; i < noetig.length; i += 4) {
    await Promise.all(
      noetig.slice(i, i + 4).map(async (m) => {
        try {
          await jsonSchreiben(API, `/api/pages/${m.id}`, await api(`/pages/${m.id}`));
        } catch {
          // Gone in between, or no longer shared: then not in the mirror.
        }
      }),
    );
  }
  await jsonSchreiben(API, "/api/pages", liste);
  const kennungen = new Set(alle.map((m) => m.id));
  const c = await caches.open(API);
  for (const k of await c.keys()) {
    const m = new URL(k.url).pathname.match(SEITE);
    if (m && !kennungen.has(m[1])) await c.delete(k);
  }
  await schaleSpiegeln();
}

const spiegeln = () =>
  exklusiv(async () => {
    if (serverWeg()) return;
    try {
      await spiegelnRoh();
      erreicht();
    } catch (f) {
      if (f.status === undefined) weg();
    }
  });

// ---------------------------------------------------------------- the outbox --

async function abgleichenRoh() {
  let ausgang = await ausgangLesen();
  if (!ausgang.length) return;
  let ich;
  try {
    ich = await api("/auth/me");
  } catch (f) {
    if (f.status === undefined) weg();
    return; // signed out: the outbox waits until one has signed in again
  }
  const besitzer = ausgang.find((x) => x.konto);
  if (besitzer && besitzer.konto !== ich.id) {
    await melden({ art: "nexora-fremd" });
    return;
  }
  const neueKennungen = {};
  let gesendet = 0;
  let zusammengefuehrt = 0;
  let konflikte = 0;
  let fehlgeschlagen = 0;
  while (ausgang.length) {
    const op = ausgang[0];
    // Every entry for itself: only what this account wrote goes out with it.
    if (op.konto && op.konto !== ich.id) {
      await melden({ art: "nexora-fremd" });
      break;
    }
    try {
      if (op.art === "anlegen") {
        const s = await api("/pages", {
          method: "POST",
          body: JSON.stringify({ parentId: op.parentId ?? null, spaceId: op.spaceId ?? null }),
        });
        await api(`/pages/${s.id}`, { method: "PUT", body: JSON.stringify(op.seite) });
        neueKennungen[op.id] = s.id;
        for (const spaeter of ausgang.slice(1)) {
          if (spaeter.id === op.id) spaeter.id = s.id;
          if (spaeter.parentId === op.id) spaeter.parentId = s.id;
        }
      } else if (op.art === "aendern") {
        const dort = await api(`/pages/${op.id}`);
        const patch = {};
        if (dort.updatedAt === op.basis.updatedAt) {
          for (const f of FELDER) if (f in op.neu) patch[f] = op.neu[f];
        } else {
          zusammengefuehrt++;
          const ih = inhaltZusammenfuehren(
            op.basis.content,
            "content" in op.neu ? op.neu.content : op.basis.content,
            dort.content,
          );
          patch.content = ih.inhalt;
          konflikte += ih.konflikte;
          for (const f of ["title", "icon"]) {
            const z = feldZusammenfuehren(op.basis[f], f in op.neu ? op.neu[f] : op.basis[f], dort[f]);
            patch[f] = z.wert;
            if (z.konflikt) konflikte++;
          }
        }
        patch.baseVersion = dort.updatedAt;
        await api(`/pages/${op.id}`, { method: "PUT", body: JSON.stringify(patch) });
      } else if (op.art === "breite") {
        await api(`/pages/${op.id}/width`, { method: "PUT", body: JSON.stringify({ width: op.breite }) });
      } else if (op.art === "loeschen") {
        await api(`/pages/${op.id}`, { method: "DELETE" });
      }
      gesendet++;
    } catch (f) {
      // Server gone again or signed out: everything stays for the next time.
      // Other refusals (no right, page deleted) do not hold up the rest.
      if (f.status === undefined || f.status === 401 || f.status >= 500) {
        if (f.status === undefined) weg();
        await ausgangSchreiben(ausgang);
        break;
      }
      fehlgeschlagen++;
    }
    ausgang.shift();
    await ausgangSchreiben(ausgang);
  }
  if (gesendet || fehlgeschlagen) {
    try {
      await spiegelnRoh();
    } catch {
      // The next mirror catches up.
    }
    await melden({ art: "nexora-abgeglichen", gesendet, zusammengefuehrt, konflikte, fehlgeschlagen, neueKennungen });
  }
  await zustandMelden();
}

const abgleichen = () => exklusiv(abgleichenRoh);

async function pruefen() {
  try {
    const r = await mitZeitlimit(voll("/healthz"));
    if (!r.ok) throw new Error(String(r.status));
    erreicht();
    await abgleichen();
  } catch {
    weg();
  }
  await zustandMelden();
}

/** A write while the server is away: applied to the kept state, queued. */
async function offlineSchreiben(methode, pfad, k) {
  const ich = await jsonLesen(API, "/api/auth/me");
  if (!ich) return nichtOffline();
  const ausgang = await ausgangLesen();
  // Changes of another account wait here: no mixing of two accounts' writes.
  if (ausgang.some((x) => x.konto && x.konto !== ich.id)) {
    return antwort(409, { error: "Changes of another account are waiting on this device. Sign in with that account to transfer them." });
  }
  const jetzt = new Date().toISOString();
  const liste = bloecke(await jsonLesen(API, "/api/pages"));
  let ergebnis;

  if (methode === "POST" && pfad === "/api/pages") {
    const id = crypto.randomUUID();
    const seite = {
      id,
      ownerId: ich.id,
      parentId: k.parentId ?? null,
      spaceId: k.spaceId ?? null,
      title: "",
      content: [],
      icon: "",
      isPublic: false,
      publicToken: null,
      tags: [],
      isFavorite: false,
      watching: false,
      canEdit: true,
      isOwner: true,
      collaborative: false,
      width: "normal",
      createdAt: jetzt,
      updatedAt: jetzt,
    };
    ausgang.push({
      art: "anlegen",
      konto: ich.id,
      id,
      parentId: seite.parentId,
      spaceId: seite.spaceId,
      seite: { title: "", content: [], icon: "" },
    });
    liste.push({ id, parentId: seite.parentId, spaceId: seite.spaceId, title: "", icon: "", shared: false, updatedAt: jetzt });
    await jsonSchreiben(API, `/api/pages/${id}`, seite);
    ergebnis = antwort(201, seite);
  } else if (SEITE.test(pfad) && methode === "PUT") {
    const id = pfad.match(SEITE)[1];
    const seite = await jsonLesen(API, pfad);
    if (!seite) return antwort(503, { error: "Offline: this page is not kept on this device." });
    if (seite.canEdit === false) return antwort(403, { error: "forbidden" });
    const neu = ausgang.find((x) => x.art === "anlegen" && x.id === id);
    if (neu) {
      for (const f of FELDER) if (f in k) neu.seite[f] = k[f];
    } else {
      // The basis is the state before the first change, not before the last:
      // only against it can one tell later what the server has differently.
      let op = ausgang.find((x) => x.art === "aendern" && x.id === id);
      if (!op) {
        op = {
          art: "aendern",
          konto: ich.id,
          id,
          basis: { title: seite.title, content: seite.content, icon: seite.icon, updatedAt: seite.updatedAt },
          neu: {},
        };
        ausgang.push(op);
      }
      for (const f of FELDER) if (f in k) op.neu[f] = k[f];
    }
    for (const f of FELDER) if (f in k) seite[f] = k[f];
    seite.updatedAt = jetzt;
    seite.collaborative = false;
    await jsonSchreiben(API, pfad, seite);
    const meta = liste.find((m) => m.id === id);
    if (meta) Object.assign(meta, { title: seite.title, icon: seite.icon, updatedAt: jetzt });
    ergebnis = antwort(200, seite);
  } else if (BREITE.test(pfad) && methode === "PUT") {
    const id = pfad.match(BREITE)[1];
    const seite = await jsonLesen(API, `/api/pages/${id}`);
    if (seite) {
      seite.width = k.width;
      await jsonSchreiben(API, `/api/pages/${id}`, seite);
    }
    ausgang.push({ art: "breite", konto: ich.id, id, breite: k.width });
    ergebnis = antwort(200, { width: k.width });
  } else if (SEITE.test(pfad) && methode === "DELETE") {
    const id = pfad.match(SEITE)[1];
    const nieDort = ausgang.some((x) => x.art === "anlegen" && x.id === id);
    const rest = ausgang.filter((x) => x.id !== id || (!nieDort && x.art !== "aendern"));
    // Never at the server: then it need not hear of it.
    if (!nieDort) rest.push({ art: "loeschen", konto: ich.id, id });
    ausgang.length = 0;
    ausgang.push(...rest);
    const i = liste.findIndex((m) => m.id === id);
    if (i >= 0) liste.splice(i, 1);
    await (await caches.open(API)).delete(voll(pfad));
    ergebnis = antwort(204);
  } else {
    return nichtOffline();
  }

  await jsonSchreiben(API, "/api/pages", liste);
  await ausgangSchreiben(ausgang);
  await zustandMelden();
  return ergebnis;
}

// ------------------------------------------------------------------- events --

self.addEventListener("message", (e) => {
  if (e.data === "abgemeldet") e.waitUntil(caches.delete(API));
  else if (e.data === "spiegeln") e.waitUntil(spiegeln());
  else if (e.data === "pruefen") e.waitUntil(pruefen());
  else if (e.data === "zustand") e.waitUntil(zustandMelden());
});

async function lesen(anfrage, pfad) {
  if (!serverWeg()) {
    if ((await ausgangLesen()).length) await abgleichen();
    if (!serverWeg()) {
      try {
        const a = await mitZeitlimit(anfrage);
        erreicht();
        if (a.ok) await (await caches.open(API)).put(anfrage, a.clone());
        // Signed out on the server: what lies in the cache belongs to nobody
        // any more.
        if (a.status === 401) await caches.delete(API);
        return a;
      } catch {
        weg();
      }
    }
  }
  const alt = await (await caches.open(API)).match(anfrage, { ignoreVary: true });
  if (!alt) return nichtOffline();
  if (SEITE.test(pfad)) {
    // Without the server no wire for writing together.
    const seite = await alt.json();
    return antwort(200, { ...seite, collaborative: false });
  }
  return alt;
}

async function schreiben(anfrage, pfad) {
  const text = anfrage.method === "DELETE" ? "" : await anfrage.clone().text();
  if (!serverWeg()) {
    if ((await ausgangLesen()).length) await abgleichen();
    if (!serverWeg()) {
      try {
        const a = await mitZeitlimit(anfrage);
        erreicht();
        return a;
      } catch {
        weg();
      }
    }
  }
  let koerper = {};
  try {
    koerper = text ? JSON.parse(text) : {};
  } catch {
    return nichtOffline();
  }
  return exklusiv(() => offlineSchreiben(anfrage.method, pfad, koerper));
}

self.addEventListener("fetch", (e) => {
  const a = e.request;
  const url = new URL(a.url);
  if (url.origin !== self.location.origin) return;
  const pfad = url.pathname;

  if (pfad.startsWith("/assets/")) {
    e.respondWith(
      caches.open(SCHALE).then(async (c) => {
        const da = await c.match(a);
        if (da) return da;
        const neu = await fetch(a);
        if (neu.ok) await c.put(a, neu.clone());
        return neu;
      }),
    );
    return;
  }

  if (pfad.startsWith("/api/")) {
    if (a.method === "GET") {
      if (url.search === "" && LESEN.some((m) => m.test(pfad))) e.respondWith(lesen(a, pfad));
      // Everything else needs the server; known to be away, say so at once
      // instead of letting the call run into its time limit.
      else if (serverWeg()) e.respondWith(nichtOffline());
      return;
    }
    const seitenSchreiben =
      (a.method === "POST" && pfad === "/api/pages") ||
      ((a.method === "PUT" || a.method === "DELETE") && SEITE.test(pfad)) ||
      (a.method === "PUT" && BREITE.test(pfad));
    if (seitenSchreiben) e.respondWith(schreiben(a, pfad));
    else if (serverWeg()) e.respondWith(nichtOffline());
    return;
  }

  if (a.mode === "navigate") {
    e.respondWith(
      (async () => {
        if (!serverWeg()) {
          try {
            const r = await mitZeitlimit(a);
            erreicht();
            if (r.ok) (await caches.open(SCHALE)).put("/index.html", r.clone());
            return r;
          } catch {
            weg();
          }
        }
        return (await caches.match("/index.html")) || Response.error();
      })(),
    );
  }
});
