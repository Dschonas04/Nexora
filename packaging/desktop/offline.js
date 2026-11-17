// Nexora unterwegs: ein Spiegel der eigenen Seiten auf dem Rechner, eine
// kleine Ersatz-API, wenn der Server nicht erreichbar ist, und der Abgleich,
// sobald er es wieder ist.
//
// Online zeigt das Fenster weiter den Server selbst, mit allem was dazu gehoert
// (gemeinsames Schreiben, Verwaltung, Suche). Nur ohne Verbindung laedt es die
// Oberflaeche aus dem Spiegel, von 127.0.0.1. Dort gibt es lesen, schreiben,
// Seiten anlegen und loeschen; jede Aenderung landet im Ausgang und geht beim
// naechsten Abgleich an den Server.
//
// Der Abgleich geht ueber die Sitzung des Fensters (session.fetch): die
// Anwendung kennt kein Passwort, sie nutzt die Anmeldung, die im Fenster schon
// besteht. Ist sie abgelaufen, wartet der Ausgang, bis man sich wieder
// angemeldet hat.
'use strict';

const { session } = require('electron');
const crypto = require('crypto');
const fs = require('fs');
const http = require('http');
const path = require('path');
const { inhaltZusammenfuehren, feldZusammenfuehren } = require('./zusammenfuehren');

const LOKAL_PORT = 47331;
const TYPEN = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.webp': 'image/webp',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.json': 'application/json',
  '.ico': 'image/x-icon',
};

class Offline {
  constructor(verzeichnis) {
    this.ordnerSetzen(verzeichnis);
    this.schluessel = crypto.randomBytes(24).toString('hex');
    this.port = null;
    this.laeuft = false;
  }

  /** Ein anderer Server, ein anderer Spiegel; der Dienst auf 127.0.0.1 bleibt. */
  ordnerWechseln(verzeichnis) {
    this.ordnerSetzen(verzeichnis);
  }

  ordnerSetzen(verzeichnis) {
    this.dir = verzeichnis;
    this.schale = path.join(verzeichnis, 'schale');
    this.standDatei = path.join(verzeichnis, 'stand.json');
    this.ausgangDatei = path.join(verzeichnis, 'ausgang.json');
    fs.mkdirSync(this.schale, { recursive: true });
    this.stand = this.lesen(this.standDatei, null);
    this.ausgang = this.lesen(this.ausgangDatei, []);
  }

  lesen(datei, vorgabe) {
    try {
      return JSON.parse(fs.readFileSync(datei, 'utf8'));
    } catch {
      return vorgabe;
    }
  }

  // Erst in eine zweite Datei, dann umbenennen: ein Absturz mitten im Schreiben
  // darf weder den Spiegel noch den Ausgang halb zuruecklassen.
  schreiben(datei, wert) {
    const tmp = datei + '.neu';
    fs.writeFileSync(tmp, JSON.stringify(wert));
    fs.renameSync(tmp, datei);
  }

  standSichern() {
    this.schreiben(this.standDatei, this.stand);
  }

  ausgangSichern() {
    this.schreiben(this.ausgangDatei, this.ausgang);
  }

  hatSpiegel() {
    return !!(this.stand && fs.existsSync(path.join(this.schale, 'index.html')));
  }

  offeneAenderungen() {
    return this.ausgang.length;
  }

  // ---------------------------------------------------------------- Server --

  async holen(server, pfad, init = {}) {
    const res = await session.defaultSession.fetch(server + pfad, {
      ...init,
      headers: { 'Content-Type': 'application/json', ...(init.headers || {}) },
    });
    const text = await res.text();
    let daten;
    try {
      daten = text ? JSON.parse(text) : undefined;
    } catch {
      daten = text;
    }
    if (!res.ok) {
      const f = new Error((daten && daten.error) || res.statusText || String(res.status));
      f.status = res.status;
      throw f;
    }
    return daten;
  }

  async erreichbar(server) {
    try {
      const res = await session.defaultSession.fetch(server + '/healthz', {
        signal: AbortSignal.timeout(4000),
      });
      return res.ok;
    } catch {
      return false;
    }
  }

  async angemeldet(server) {
    try {
      await this.holen(server, '/api/auth/me');
      return true;
    } catch {
      return false;
    }
  }

  // --------------------------------------------------------------- Spiegeln --

  /** Holt alle eigenen und geteilten Seiten, die sich seit dem letzten Mal geaendert haben. */
  async spiegeln(server) {
    const alt = this.stand || { seiten: {} };
    const [ich, design, lizenz, spaces, tags, favoriten, seitenListe, geteilt] = await Promise.all([
      this.holen(server, '/api/auth/me'),
      this.holen(server, '/api/design').catch(() => ({})),
      this.holen(server, '/api/license').catch(() => ({})),
      this.holen(server, '/api/spaces').catch(() => []),
      this.holen(server, '/api/tags').catch(() => []),
      this.holen(server, '/api/favorites').catch(() => []),
      this.holen(server, '/api/pages'),
      this.holen(server, '/api/pages/shared').catch(() => []),
    ]);
    // Solange Aenderungen eines anderen Kontos warten, bleibt dessen Spiegel
    // stehen: die neu angelegten Seiten darin gibt es nur hier.
    const besitzer = this.ausgangKonto();
    if (besitzer && besitzer !== ich.id) return 0;
    // Ein anderes Konto am selben Rechner bekommt keinen fremden Spiegel.
    const seiten = alt.ich && alt.ich.id === ich.id ? { ...alt.seiten } : {};
    const liste = (x) => (Array.isArray(x) ? x : []);
    const alle = [...liste(seitenListe), ...liste(geteilt)];
    const noetig = alle.filter((m) => !seiten[m.id] || seiten[m.id].updatedAt !== m.updatedAt);
    // Vier gleichzeitig: genug, dass es zuegig geht, und wenig genug, dass der
    // Server nebenher noch fuer die anderen da ist.
    for (let i = 0; i < noetig.length; i += 4) {
      const stueck = await Promise.all(
        noetig.slice(i, i + 4).map((m) => this.holen(server, `/api/pages/${m.id}`).catch(() => null)),
      );
      for (const s of stueck) if (s) seiten[s.id] = s;
    }
    const kennungen = new Set(alle.map((m) => m.id));
    for (const id of Object.keys(seiten)) if (!kennungen.has(id)) delete seiten[id];

    this.stand = {
      ich,
      design,
      lizenz,
      spaces,
      tags,
      favoriten,
      seitenListe,
      geteilt,
      seiten,
      zuletzt: new Date().toISOString(),
    };
    this.standSichern();
    await this.schaleHolen(server);
    return noetig.length;
  }

  /** Die Oberflaeche: index.html und alles, was darin und in den Bundles verwiesen wird. */
  async schaleHolen(server) {
    const res = await session.defaultSession.fetch(server + '/');
    if (!res.ok) return;
    const index = await res.text();
    // Gleiche index.html heisst gleiche Bundles: die Namen tragen ihren Hash.
    const alt = path.join(this.schale, 'index.html');
    if (fs.existsSync(alt) && fs.readFileSync(alt, 'utf8') === index) return;
    const neu = path.join(this.dir, 'schale.neu');
    fs.rmSync(neu, { recursive: true, force: true });
    fs.mkdirSync(path.join(neu, 'assets'), { recursive: true });
    fs.writeFileSync(path.join(neu, 'index.html'), index);

    const offen = new Set();
    const muster = /["'(\/]((?:assets\/)[A-Za-z0-9_.\-]+\.(?:js|css|woff2?|ttf|svg|png|webp))/g;
    const wurzel = /(?:href|src)="\/([A-Za-z0-9_.\-]+\.(?:svg|png|ico|webmanifest|json))"/g;
    const merken = (text) => {
      for (const m of text.matchAll(muster)) offen.add(m[1]);
    };
    merken(index);
    for (const m of index.matchAll(wurzel)) offen.add(m[1]);
    const fertig = new Set();
    while (offen.size) {
      const datei = offen.values().next().value;
      offen.delete(datei);
      if (fertig.has(datei)) continue;
      fertig.add(datei);
      try {
        const r = await session.defaultSession.fetch(`${server}/${datei}`);
        if (!r.ok) continue;
        const puffer = Buffer.from(await r.arrayBuffer());
        fs.mkdirSync(path.dirname(path.join(neu, datei)), { recursive: true });
        fs.writeFileSync(path.join(neu, datei), puffer);
        if (/\.(js|css)$/.test(datei)) merken(puffer.toString('utf8'));
      } catch {
        // Eine fehlende Schrift soll den Spiegel nicht verhindern.
      }
    }
    fs.rmSync(this.schale, { recursive: true, force: true });
    fs.renameSync(neu, this.schale);
  }

  // -------------------------------------------------------------- Abgleich --

  /**
   * Spielt den Ausgang auf den Server. Gibt zurueck, wie viele Aenderungen
   * durchgingen und wie oft zusammengefuehrt werden musste.
   */
  /** Das Konto, dem die wartenden Aenderungen gehoeren, oder null. */
  ausgangKonto() {
    const k = this.ausgang.find((x) => x.konto);
    return k ? k.konto : this.ausgang.length && this.stand ? this.stand.ich.id : null;
  }

  async ausgangSenden(server) {
    // Gesendet wird nur mit der Sitzung des Kontos, das die Aenderungen
    // gemacht hat. Hat sich inzwischen jemand anderes angemeldet, bleibt der
    // Ausgang liegen: sonst liefen fremde Aenderungen und Loeschungen unter
    // dessen Namen.
    const ich = await this.holen(server, '/api/auth/me');
    const besitzer = this.ausgangKonto();
    if (besitzer && besitzer !== ich.id) {
      const f = new Error('Die wartenden Änderungen gehören zu einem anderen Konto.');
      f.fremdesKonto = true;
      throw f;
    }
    let gesendet = 0;
    let zusammengefuehrt = 0;
    let konflikte = 0;
    const fehlgeschlagen = [];
    const neueKennung = new Map(); // offline vergebene Kennung -> die des Servers

    const echt = (id) => (id && neueKennung.has(id) ? neueKennung.get(id) : id);

    while (this.ausgang.length) {
      const op = this.ausgang[0];
      try {
        if (op.art === 'anlegen') {
          const s = await this.holen(server, '/api/pages', {
            method: 'POST',
            body: JSON.stringify({ parentId: echt(op.parentId), spaceId: op.spaceId ?? null }),
          });
          neueKennung.set(op.id, s.id);
          const seite = this.stand.seiten[op.id];
          if (seite) {
            await this.holen(server, `/api/pages/${s.id}`, {
              method: 'PUT',
              body: JSON.stringify({ title: seite.title, content: seite.content, icon: seite.icon }),
            });
          }
          // Spaetere Eintraege fuer dieselbe Seite zielen jetzt auf die echte.
          for (const spaeter of this.ausgang.slice(1)) {
            if (spaeter.id === op.id) spaeter.id = s.id;
            if (spaeter.parentId === op.id) spaeter.parentId = s.id;
          }
          // Das Anlegen hat Titel und Inhalt schon mitgenommen.
          this.ausgang = this.ausgang.filter((x, i) => i === 0 || !(x.art === 'aendern' && x.id === s.id));
        } else if (op.art === 'aendern') {
          const id = echt(op.id);
          const aufServer = await this.holen(server, `/api/pages/${id}`);
          const patch = {};
          if (aufServer.updatedAt === op.basis.updatedAt) {
            for (const f of ['title', 'content', 'icon']) if (f in op.neu) patch[f] = op.neu[f];
          } else {
            zusammengefuehrt++;
            const ih = inhaltZusammenfuehren(op.basis.content, op.neu.content ?? op.basis.content, aufServer.content);
            patch.content = ih.inhalt;
            konflikte += ih.konflikte;
            for (const f of ['title', 'icon']) {
              const z = feldZusammenfuehren(op.basis[f], op.neu[f] ?? op.basis[f], aufServer[f]);
              patch[f] = z.wert;
              if (z.konflikt) konflikte++;
            }
          }
          patch.baseVersion = aufServer.updatedAt;
          await this.holen(server, `/api/pages/${id}`, { method: 'PUT', body: JSON.stringify(patch) });
        } else if (op.art === 'loeschen') {
          await this.holen(server, `/api/pages/${echt(op.id)}`, { method: 'DELETE' });
        } else if (op.art === 'breite') {
          await this.holen(server, `/api/pages/${echt(op.id)}/width`, {
            method: 'PUT',
            body: JSON.stringify({ width: op.breite }),
          });
        }
        gesendet++;
      } catch (f) {
        // Abgemeldet oder Server weg: alles bleibt, wie es ist, und kommt beim
        // naechsten Mal dran. Andere Fehler (keine Rechte, Seite geloescht)
        // haelt der Ausgang nicht auf; sie werden genannt.
        if (f.status === 401 || f.status === undefined || f.status >= 500) throw f;
        fehlgeschlagen.push({ op, fehler: f.message });
      }
      this.ausgang.shift();
      this.ausgangSichern();
    }
    return { gesendet, zusammengefuehrt, konflikte, fehlgeschlagen };
  }

  // ----------------------------------------------------------- Ersatz-API --

  aendern(id, neu) {
    const seite = this.stand.seiten[id];
    if (!seite) return null;
    // Die Basis ist der Stand vor der ersten Aenderung, nicht vor der letzten:
    // nur gegen sie laesst sich spaeter sagen, was der Server inzwischen
    // anders hat.
    let op = this.ausgang.find((x) => (x.art === 'aendern' || x.art === 'anlegen') && x.id === id);
    if (!op) {
      op = {
        art: 'aendern',
        konto: this.stand.ich.id,
        id,
        basis: { title: seite.title, content: seite.content, icon: seite.icon, updatedAt: seite.updatedAt },
        neu: {},
      };
      this.ausgang.push(op);
    }
    for (const f of ['title', 'content', 'icon']) {
      if (f in neu) {
        seite[f] = neu[f];
        if (op.neu) op.neu[f] = neu[f];
      }
    }
    seite.updatedAt = new Date().toISOString();
    const meta = this.stand.seitenListe.find((m) => m.id === id);
    if (meta) {
      meta.title = seite.title;
      meta.icon = seite.icon;
      meta.updatedAt = seite.updatedAt;
    }
    this.standSichern();
    this.ausgangSichern();
    return seite;
  }

  anlegen(parentId, spaceId) {
    const id = crypto.randomUUID();
    const jetzt = new Date().toISOString();
    const seite = {
      id,
      ownerId: this.stand.ich.id,
      parentId: parentId ?? null,
      spaceId: spaceId ?? null,
      title: '',
      content: [],
      icon: '',
      isPublic: false,
      publicToken: null,
      tags: [],
      isFavorite: false,
      watching: false,
      canEdit: true,
      isOwner: true,
      collaborative: false,
      width: '',
      createdAt: jetzt,
      updatedAt: jetzt,
    };
    this.stand.seiten[id] = seite;
    this.stand.seitenListe.push({
      id,
      parentId: seite.parentId,
      spaceId: seite.spaceId,
      title: '',
      icon: '',
      shared: false,
      updatedAt: jetzt,
    });
    this.ausgang.push({ art: 'anlegen', konto: this.stand.ich.id, id, parentId: seite.parentId, spaceId: seite.spaceId });
    this.standSichern();
    this.ausgangSichern();
    return seite;
  }

  loeschen(id) {
    const neuAngelegt = this.ausgang.some((x) => x.art === 'anlegen' && x.id === id);
    delete this.stand.seiten[id];
    this.stand.seitenListe = this.stand.seitenListe.filter((m) => m.id !== id);
    if (neuAngelegt) {
      // Nie beim Server gewesen: dann muss er davon auch nichts erfahren.
      this.ausgang = this.ausgang.filter((x) => x.id !== id);
    } else {
      this.ausgang = this.ausgang.filter((x) => !(x.art === 'aendern' && x.id === id));
      this.ausgang.push({ art: 'loeschen', konto: this.stand.ich.id, id });
    }
    this.standSichern();
    this.ausgangSichern();
  }

  // ------------------------------------------------------- Lokaler Dienst --

  startAdresse() {
    return `http://127.0.0.1:${this.port}/__start?k=${this.schluessel}`;
  }

  starten() {
    if (this.laeuft) return Promise.resolve();
    const dienst = http.createServer((req, res) => this.anfrage(req, res).catch((f) => {
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: String(f && f.message) }));
    }));
    return new Promise((fertig) => {
      dienst.once('error', () => dienst.listen(0, '127.0.0.1'));
      dienst.listen(LOKAL_PORT, '127.0.0.1');
      dienst.on('listening', () => {
        this.port = dienst.address().port;
        this.laeuft = true;
        fertig();
      });
    });
  }

  async anfrage(req, res) {
    const url = new URL(req.url, 'http://127.0.0.1');
    const json = (status, wert) => {
      res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' });
      res.end(wert === undefined ? '' : JSON.stringify(wert));
    };

    // Nur das eigene Fenster kommt an den Spiegel: es bekommt beim Start einen
    // Keks mit einem Schluessel, den nur diese Anwendung kennt. Andere
    // Programme auf dem Rechner stehen ohne ihn vor der Tuer.
    if (url.pathname === '/__start') {
      if (url.searchParams.get('k') !== this.schluessel) return json(403, { error: 'forbidden' });
      res.writeHead(302, {
        'Set-Cookie': `nexora_lokal=${this.schluessel}; HttpOnly; SameSite=Strict; Path=/`,
        Location: '/',
      });
      return res.end();
    }
    const kekse = Object.fromEntries(
      String(req.headers.cookie || '')
        .split(';')
        .map((k) => k.trim().split('='))
        .filter((k) => k.length === 2),
    );
    if (kekse.nexora_lokal !== this.schluessel) return json(403, { error: 'forbidden' });

    if (url.pathname.startsWith('/api/')) return this.api(req, url.pathname.slice(4), json);
    // Kein Service Worker im Spiegel: er wuerde sich zwischen Spiegel und
    // Fenster schieben und alte Staende festhalten.
    if (url.pathname === '/sw.js') return json(404, { error: 'not here' });
    return this.datei(url.pathname, res);
  }

  datei(pfad, res) {
    let datei = path.normalize(path.join(this.schale, decodeURIComponent(pfad)));
    if (!datei.startsWith(this.schale)) {
      res.writeHead(400);
      return res.end();
    }
    if (!fs.existsSync(datei) || fs.statSync(datei).isDirectory()) datei = path.join(this.schale, 'index.html');
    let inhalt = fs.readFileSync(datei);
    if (datei.endsWith('index.html')) {
      inhalt = Buffer.from(String(inhalt).replace('</body>', `${HINWEIS(this)}</body>`));
    }
    res.writeHead(200, {
      'Content-Type': TYPEN[path.extname(datei)] || 'application/octet-stream',
      'Cache-Control': datei.includes(`${path.sep}assets${path.sep}`) ? 'max-age=31536000' : 'no-store',
    });
    res.end(inhalt);
  }

  async koerper(req) {
    const teile = [];
    for await (const t of req) teile.push(t);
    const text = Buffer.concat(teile).toString('utf8');
    return text ? JSON.parse(text) : {};
  }

  async api(req, p, json) {
    const st = this.stand;
    if (!st) return json(503, { error: 'Offline: noch kein Spiegel vorhanden.' });
    const m = req.method;
    const seiteId = (p.match(/^\/pages\/([0-9a-f-]{36})(?:\/|$)/) || [])[1];

    if (m === 'GET') {
      if (p === '/auth/me') return json(200, st.ich);
      if (p === '/design') return json(200, st.design);
      if (p === '/license') return json(200, st.lizenz);
      if (p === '/spaces') return json(200, st.spaces);
      if (p === '/tags') return json(200, st.tags);
      if (p === '/favorites') return json(200, st.favoriten);
      if (p === '/pages') return json(200, st.seitenListe);
      if (p === '/pages/shared') return json(200, st.geteilt);
      if (seiteId && p === `/pages/${seiteId}`) {
        const s = st.seiten[seiteId];
        if (!s) return json(404, { error: 'Diese Seite ist offline nicht da.' });
        // Ohne Verbindung keine Leitung fuer gemeinsames Schreiben.
        return json(200, { ...s, collaborative: false });
      }
      if (seiteId && /\/(backlinks|links|comments|attachments|versions)$/.test(p)) return json(200, []);
      if (/^\/tags\/[^/]+\/pages$/.test(p)) {
        const tagId = p.split('/')[2];
        return json(
          200,
          st.seitenListe.filter((x) => (st.seiten[x.id]?.tags || []).some((t) => t.id === tagId)),
        );
      }
      if (p === '/postfach' || p.startsWith('/postfach')) return json(200, []);
      return json(503, { error: 'Offline: das geht erst wieder zuhause.' });
    }

    if (m === 'POST' && p === '/pages') {
      const k = await this.koerper(req);
      return json(201, this.anlegen(k.parentId, k.spaceId));
    }
    if (m === 'PUT' && seiteId && p === `/pages/${seiteId}`) {
      const k = await this.koerper(req);
      const s = this.aendern(seiteId, k);
      return s ? json(200, { ...s, collaborative: false }) : json(404, { error: 'not found' });
    }
    if (m === 'PUT' && seiteId && p === `/pages/${seiteId}/width`) {
      const k = await this.koerper(req);
      if (st.seiten[seiteId]) st.seiten[seiteId].width = k.width;
      this.ausgang.push({ art: 'breite', konto: st.ich.id, id: seiteId, breite: k.width });
      this.standSichern();
      this.ausgangSichern();
      return json(200, { width: k.width });
    }
    if (m === 'DELETE' && seiteId && p === `/pages/${seiteId}`) {
      this.loeschen(seiteId);
      return json(200, { ok: true });
    }
    return json(503, { error: 'Offline: das geht erst wieder zuhause.' });
  }
}

/** Ein schmaler Streifen unten, damit klar ist, dass man im Spiegel arbeitet. */
function HINWEIS(o) {
  const zeit = o.stand && o.stand.zuletzt ? new Date(o.stand.zuletzt).toLocaleString('de-DE') : '';
  return `<div id="nexora-offline" style="position:fixed;left:50%;bottom:10px;transform:translateX(-50%);z-index:9999;background:#37352f;color:#fff;font:12px/1.4 -apple-system,sans-serif;padding:6px 12px;border-radius:999px;opacity:.9;pointer-events:none">Offline · Änderungen werden zuhause abgeglichen${zeit ? ' · Stand ' + zeit : ''}</div>`;
}

module.exports = { Offline };
