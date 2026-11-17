// Nexora als Anwendung auf dem Schreibtisch.
//
// Online ist es ein Fenster auf die Instanz, die schon laeuft: dieselbe
// Oberflaeche, derselbe Server, derselbe Stand fuer alle. Unterwegs, ohne
// Verbindung, arbeitet es auf einem Spiegel der eigenen Seiten (offline.js)
// und gleicht ihn ab, sobald der Server wieder erreichbar ist.
//
// Was die Anwendung gegenueber einem Lesezeichen im Browser bringt: ein
// eigenes Fenster ohne Adresszeile und fremde Reiter, ein Eintrag im
// Startmenue, und die Adresse der Instanz steht einmal fest statt in jedem
// Browserprofil neu.
const { app, BrowserWindow, Menu, Notification, dialog, ipcMain, net, protocol, session, shell } = require('electron');
const os = require('os');
const { pathToFileURL } = require('url');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');

const EINSTELLUNGEN = () => path.join(app.getPath('userData'), 'einstellungen.json');
const VORGABE = 'https://10.0.2.7:3000';
const { Offline } = require('./offline');
let offline = null;
let modus = 'start'; // 'online', 'offline', 'lokal' oder 'einrichtung'
let abgleichLaeuft = false;
let letzterAbgleich = null;
let fremdGemeldet = false;

function adresseLesen() {
  try {
    const roh = JSON.parse(fs.readFileSync(EINSTELLUNGEN(), 'utf8'));
    if (typeof roh.adresse === 'string' && roh.adresse.startsWith('http')) return roh.adresse;
  } catch {
    // Erster Start oder beschaedigte Datei: dann eben die Vorgabe.
  }
  return process.env.NEXORA_ADRESSE || VORGABE;
}

function einstellungenLesen() {
  try {
    return JSON.parse(fs.readFileSync(EINSTELLUNGEN(), 'utf8')) || {};
  } catch {
    return {};
  }
}

function einstellungenAendern(aenderung) {
  const roh = { ...einstellungenLesen(), ...aenderung };
  fs.mkdirSync(path.dirname(EINSTELLUNGEN()), { recursive: true });
  fs.writeFileSync(EINSTELLUNGEN(), JSON.stringify(roh, null, 2));
}

/** Ob ohne Server gearbeitet wird: alles liegt dann nur auf diesem Rechner. */
const ohneServer = () => einstellungenLesen().modus === 'lokal';

/** Ob schon gewaehlt wurde, Server oder ohne (sonst kommt die Einrichtung). */
function eingerichtet() {
  if (process.env.NEXORA_ADRESSE) return true;
  const roh = einstellungenLesen();
  return roh.modus === 'lokal' || (typeof roh.adresse === 'string' && roh.adresse.startsWith('http'));
}

// Ohne Server kommt die Oberflaeche aus der Anwendung selbst, unter einem
// eigenen Ursprung app://nexora. Der bleibt immer gleich, damit der Speicher
// des Fensters (IndexedDB, dort liegen die Seiten) beim naechsten Start
// wieder derselbe ist.
const LOKAL = 'app://nexora/';
const OBERFLAECHE = path.join(__dirname, 'oberflaeche');
protocol.registerSchemesAsPrivileged([
  { scheme: 'app', privileges: { standard: true, secure: true, supportFetchAPI: true, stream: true } },
]);
function lokalAusliefern() {
  protocol.handle('app', (anfrage) => {
    const u = new URL(anfrage.url);
    if (u.host !== 'nexora') return new Response('', { status: 404 });
    let datei = path.normalize(path.join(OBERFLAECHE, decodeURIComponent(u.pathname)));
    if (!datei.startsWith(OBERFLAECHE + path.sep) && datei !== OBERFLAECHE) return new Response('', { status: 400 });
    // Seitenadressen wie /page/... sind Sache der Oberflaeche: index.html.
    if (!fs.existsSync(datei) || fs.statSync(datei).isDirectory()) datei = path.join(OBERFLAECHE, 'index.html');
    return net.fetch(pathToFileURL(datei).toString());
  });
}
const istLokal = (adresse) => typeof adresse === 'string' && adresse.startsWith(LOKAL);

/** Der Spiegel gehoert zu einem Server: je Ursprung ein eigener Ordner. */
function spiegelOrdner(adresse) {
  const kennung = crypto.createHash('sha256').update(new URL(adresse).origin).digest('hex').slice(0, 16);
  return path.join(app.getPath('userData'), 'offline', kennung);
}

// Die Homelab-CA. Ihr wird nur fuer den eingestellten Server und seine
// Anmeldeanbieter vertraut, nicht fuer beliebige Seiten: wer die App mit einem
// anderen Server nutzt, soll keiner fremden CA ausgeliefert sein.
const EIGENE_CA = (() => {
  try {
    return new crypto.X509Certificate(fs.readFileSync(path.join(__dirname, 'homelab-root-ca.pem')));
  } catch {
    return null;
  }
})();

function gueltig(x) {
  const jetzt = Date.now();
  return Date.parse(x.validFrom) <= jetzt && jetzt <= Date.parse(x.validTo);
}

/**
 * Ob das Zertifikat fuer den Host gilt und ueber eine saubere Kette an der
 * eigenen CA haengt: Name passt, alle Glieder gueltig, das Blatt ist keine CA,
 * jeder Aussteller ist eine.
 */
function ketteZurEigenenCA(zertifikat, host) {
  if (!EIGENE_CA || !zertifikat) return false;
  try {
    const blatt = new crypto.X509Certificate(zertifikat.data);
    if (blatt.ca || !gueltig(blatt)) return false;
    if (!blatt.checkHost(host) && !blatt.checkIP(host)) return false;
    let x = blatt;
    let glied = zertifikat;
    for (let i = 0; i < 5; i++) {
      if (x.checkIssued(EIGENE_CA) && x.verify(EIGENE_CA.publicKey)) return gueltig(EIGENE_CA);
      const aussteller = glied.issuerCert;
      if (!aussteller || aussteller === glied) return false;
      const ax = new crypto.X509Certificate(aussteller.data);
      if (!ax.ca || !gueltig(ax) || !x.checkIssued(ax) || !x.verify(ax.publicKey)) return false;
      if (ax.fingerprint256 === EIGENE_CA.fingerprint256) return true;
      x = ax;
      glied = aussteller;
    }
  } catch {
    return false;
  }
  return false;
}

function zertifikatePruefen() {
  session.defaultSession.setCertificateVerifyProc((anfrage, antwort) => {
    if (anfrage.verificationResult === 'net::OK') return antwort(0);
    const erlaubteHosts = new Set();
    try {
      erlaubteHosts.add(new URL(adresseLesen()).hostname);
      if (process.env.NEXORA_PRUEFADRESSE) erlaubteHosts.add(new URL(process.env.NEXORA_PRUEFADRESSE).hostname);
      for (const a of ANBIETER()) erlaubteHosts.add(new URL(a).hostname);
    } catch {
      // keine gueltige Adresse: dann eben keine Ausnahme
    }
    if (
      anfrage.verificationResult === 'net::ERR_CERT_AUTHORITY_INVALID' &&
      erlaubteHosts.has(anfrage.hostname) &&
      ketteZurEigenenCA(anfrage.certificate, anfrage.hostname)
    ) {
      return antwort(0);
    }
    return antwort(-3); // Chromium entscheidet wie sonst
  });
}

function adresseSchreiben(adresse) {
  try {
    fs.mkdirSync(path.dirname(EINSTELLUNGEN()), { recursive: true });
    // Gelernte Anbieter bleiben an ihrer Instanz; eine neue Adresse erbt keine.
    let anbieterJe = {};
    try {
      anbieterJe = JSON.parse(fs.readFileSync(EINSTELLUNGEN(), 'utf8')).anbieterJe || {};
    } catch {
      anbieterJe = {};
    }
    fs.writeFileSync(EINSTELLUNGEN(), JSON.stringify({ adresse, anbieterJe, modus: 'server' }, null, 2));
  } catch (fehler) {
    dialog.showErrorBox('Nicht gespeichert', String(fehler));
  }
}

/** Nur echte Webadressen darf das Betriebssystem zu sehen bekommen. */
function nachDraussenErlaubt(adresse) {
  try {
    const u = new URL(adresse);
    return u.protocol === 'http:' || u.protocol === 'https:';
  } catch {
    return false;
  }
}

/** Ob eine Adresse noch zur eingestellten Instanz gehoert. */
function gleicherUrsprung(adresse, instanz) {
  try {
    const a = new URL(adresse).origin;
    return a !== 'null' && a === new URL(instanz).origin;
  } catch {
    return false;
  }
}

// Anmeldeanbieter (SSO), die das Fenster betreten darf. Nicht "jede Seite, auf
// der das Fenster gerade steht": eine offene Weiterleitung koennte sonst eine
// fremde Seite ins Fenster bringen, die dann wie Nexora aussieht. Gelernt wird
// ein Anbieter nur aus der Weiterleitung, die der Server selbst auf
// /api/auth/oidc/start ausgibt, und nur mit https.
const ANBIETER = () => {
  try {
    const roh = JSON.parse(fs.readFileSync(EINSTELLUNGEN(), 'utf8'));
    // Nur die Anbieter, die fuer genau diese Instanz gelernt wurden.
    const liste = roh.anbieterJe && roh.anbieterJe[new URL(adresseLesen()).origin];
    return Array.isArray(liste) ? liste : [];
  } catch {
    return [];
  }
};
function anbieterMerken(ursprung) {
  let roh = {};
  try {
    roh = JSON.parse(fs.readFileSync(EINSTELLUNGEN(), 'utf8'));
  } catch {
    roh = { adresse: adresseLesen() };
  }
  const instanz = new URL(adresseLesen()).origin;
  roh.anbieterJe = roh.anbieterJe || {};
  roh.anbieterJe[instanz] = [...new Set([...(roh.anbieterJe[instanz] || []), ursprung])];
  delete roh.anbieter;
  fs.mkdirSync(path.dirname(EINSTELLUNGEN()), { recursive: true });
  fs.writeFileSync(EINSTELLUNGEN(), JSON.stringify(roh, null, 2));
}
function istAnbieter(adresse) {
  try {
    const u = new URL(adresse);
    return u.protocol === 'https:' && ANBIETER().includes(u.origin);
  } catch {
    return false;
  }
}
function zielErlaubt(ziel) {
  if (ohneServer()) return istLokal(ziel);
  return (
    gleicherUrsprung(ziel, adresseLesen()) ||
    (offline && offline.port && gleicherUrsprung(ziel, `http://127.0.0.1:${offline.port}`)) ||
    istAnbieter(ziel)
  );
}

let fenster = null;

function fensterBauen() {
  fenster = new BrowserWindow({
    width: 1280,
    height: 860,
    minWidth: 900,
    minHeight: 600,
    title: 'Nexora',
    backgroundColor: '#f7f7f6',
    autoHideMenuBar: true,
    webPreferences: {
      // Die Anwendung fuehrt nur fremde Seiten vor, sie braucht keinen
      // Zugriff auf Node. Ohne diese beiden Zeilen haette jede Seite, die im
      // Fenster laedt, das Dateisystem des Rechners.
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: true,
      preload: path.join(__dirname, 'vorab.js'),
      spellcheck: true,
    },
  });

  starten();

  // Verweise nach draussen gehoeren in den Browser, nicht in dieses Fenster:
  // sonst landet man in einer Anwendung ohne Zurueck-Knopf auf einer fremden
  // Seite.
  //
  // Weitergereicht wird nur http und https. shell.openExternal gibt die
  // Adresse an das Betriebssystem, und das oeffnet auch file:, smb: oder
  // ms-msdt: -- eine Seite koennte damit ueber ein window.open Programme auf
  // dem Rechner anstossen. Die Anwendung zeigt fremde Seiten, also wird hier
  // gefiltert und nicht gehofft.
  fenster.webContents.setWindowOpenHandler(({ url }) => {
    if (nachDraussenErlaubt(url)) shell.openExternal(url);
    return { action: 'deny' };
  });

  // Zweiter Riegel: das Fenster selbst bleibt bei seiner Instanz (oder beim
  // eigenen Spiegel auf 127.0.0.1). Ohne das
  // koennte ein Verweis im Inhalt das Fenster auf eine fremde Seite umlenken,
  // und die saehe dann aus wie Nexora -- samt Eingabefeld fuer das Passwort.
  fenster.webContents.on('will-navigate', (ereignis, ziel) => {
    const erlaubt = zielErlaubt(ziel);
    if (!erlaubt) {
      ereignis.preventDefault();
      if (nachDraussenErlaubt(ziel)) shell.openExternal(ziel);
    }
  });

  // Weiterleitungen pruefen wie Klicks. Die eine Ausnahme: der Server schickt
  // beim Start der SSO-Anmeldung zum Anbieter. Dessen Ursprung wird gemerkt,
  // damit das Formular dort abgeschickt werden kann.
  // Die letzte Station der laufenden Navigation. Gelernt wird ein Anbieter
  // nur aus dem einen Sprung, der unmittelbar von /api/auth/oidc/start der
  // eigenen Instanz ausgeht; jede weitere Weiterleitung wird normal geprueft.
  let letzteStation = '';
  fenster.webContents.on('did-start-navigation', (d) => {
    if (d.isMainFrame && !d.isSameDocument) letzteStation = d.url;
  });
  fenster.webContents.on('will-redirect', (ereignis) => {
    const ziel = ereignis.url;
    const von = letzteStation;
    letzteStation = ziel;
    if (zielErlaubt(ziel)) return;
    let vomStart = false;
    let https = false;
    try {
      vomStart = gleicherUrsprung(von, adresseLesen()) && new URL(von).pathname === '/api/auth/oidc/start';
      https = new URL(ziel).protocol === 'https:';
    } catch {
      vomStart = false;
    }
    if (vomStart && https) {
      anbieterMerken(new URL(ziel).origin);
      return;
    }
    ereignis.preventDefault();
    if (nachDraussenErlaubt(ziel)) shell.openExternal(ziel);
  });

  fenster.webContents.on('did-fail-load', (_e, code, beschreibung, _url, hauptRahmen) => {
    if (code === -3 || !hauptRahmen) return; // abgebrochene Weiterleitung, kein Fehler
    if (modus === 'lokal' || modus === 'einrichtung') return;
    // Mit Spiegel: still in den Offline-Modus, statt eine Fehlermeldung zu zeigen.
    if (offline && offline.hatSpiegel()) {
      offlineZeigen();
      return;
    }
    const ziel = adresseLesen();
    dialog
      .showMessageBox(fenster, {
        type: 'warning',
        title: 'Nexora nicht erreichbar',
        message: `${ziel} antwortet nicht.`,
        detail: `${beschreibung}\n\nLäuft die Instanz, und bist du im selben Netz?`,
        buttons: ['Erneut versuchen', 'Server wechseln', 'Beenden'],
        defaultId: 0,
        cancelId: 2,
      })
      .then(({ response }) => {
        if (response === 0) fenster.loadURL(adresseLesen());
        else if (response === 1) einrichtungZeigen();
        else app.quit();
      });
  });
}

const EINRICHTUNG = () => path.join(__dirname, 'einrichten.html');

function einrichtungZeigen() {
  modus = 'einrichtung';
  fenster.loadFile(EINRICHTUNG());
  titelSetzen();
}

/** Nur die lokale Einrichtungsseite darf Adresse lesen, pruefen, speichern. */
function vonEinrichtung(ereignis) {
  try {
    const u = ereignis.senderFrame && ereignis.senderFrame.url;
    return !!u && u.startsWith('file:') && decodeURIComponent(new URL(u).pathname).endsWith('/einrichten.html');
  } catch {
    return false;
  }
}

function einrichtungVerdrahten() {
  ipcMain.handle('einrichtung:adresse', (e) => (vonEinrichtung(e) && eingerichtet() ? adresseLesen() : ''));
  ipcMain.handle('einrichtung:pruefen', async (e, adresse) => {
    if (!vonEinrichtung(e)) return { ok: false, fehler: 'nicht erlaubt' };
    let u;
    try {
      u = new URL(adresse);
      if (u.protocol !== 'https:' && u.protocol !== 'http:') throw new Error('');
    } catch {
      return { ok: false, fehler: 'Das ist keine gültige Adresse.' };
    }
    // Vorab eintragen, damit die Pruefung der CA fuer diesen Host gilt.
    const vorher = process.env.NEXORA_PRUEFADRESSE;
    process.env.NEXORA_PRUEFADRESSE = u.origin;
    try {
      const r = await session.defaultSession.fetch(u.origin + '/healthz', { signal: AbortSignal.timeout(6000) });
      if (!r.ok) return { ok: false, fehler: `Der Server antwortet mit ${r.status}. Ist das ein Nexora?` };
      return { ok: true };
    } catch (f) {
      const text = String((f && f.message) || f);
      if (/CERT|certificate/i.test(text)) return { ok: false, fehler: 'Dem Zertifikat des Servers wird nicht vertraut.' };
      return { ok: false, fehler: 'Der Server ist nicht erreichbar. Stimmt die Adresse, und bist du im selben Netz?' };
    } finally {
      if (vorher === undefined) delete process.env.NEXORA_PRUEFADRESSE;
      else process.env.NEXORA_PRUEFADRESSE = vorher;
    }
  });
  ipcMain.handle('einrichtung:lokal', async (e) => {
    if (!vonEinrichtung(e)) return false;
    einstellungenAendern({ modus: 'lokal' });
    lokalZeigen();
    return true;
  });
  // Aus der Oberflaeche ohne Server: der Name des Benutzers auf diesem
  // Rechner, und der Weg zurueck zur Einrichtung (das "Abmelden").
  const vonLokal = (e) => istLokal(e.senderFrame && e.senderFrame.url);
  ipcMain.handle('lokal:benutzer', (e) => {
    if (!vonLokal(e)) return '';
    try {
      return os.userInfo().username;
    } catch {
      return '';
    }
  });
  ipcMain.handle('lokal:zurueck', (e) => {
    if (!vonLokal(e)) return false;
    einstellungenAendern({ modus: einstellungenLesen().adresse ? 'server' : '' });
    einrichtungZeigen();
    return true;
  });
  ipcMain.handle('einrichtung:speichern', async (e, adresse) => {
    if (!vonEinrichtung(e)) return false;
    const u = new URL(adresse);
    adresseSchreiben(u.origin);
    await offline.ordnerWechseln(spiegelOrdner(u.origin));
    letzterAbgleich = null;
    modus = 'start';
    onlineZeigen();
    pruefen();
    return true;
  });
}

function menueBauen() {
  Menu.setApplicationMenu(
    Menu.buildFromTemplate([
      {
        label: 'Nexora',
        submenu: [
          { label: 'Neu laden', accelerator: 'CmdOrCtrl+R', click: () => fenster.reload() },
          { label: 'Server wechseln…', click: einrichtungZeigen },
          { label: 'Jetzt abgleichen', accelerator: 'CmdOrCtrl+Shift+S', click: () => (ohneServer() ? melden('Nexora', 'Ohne Server gibt es nichts abzugleichen.') : pruefen(true)) },
          { label: 'Abgleich-Stand anzeigen', click: standZeigen },
          { type: 'separator' },
          { label: 'Vollbild', role: 'togglefullscreen' },
          { label: 'Entwicklerwerkzeuge', accelerator: 'F12', role: 'toggleDevTools' },
          { type: 'separator' },
          { label: 'Beenden', role: 'quit' },
        ],
      },
      {
        label: 'Bearbeiten',
        submenu: [
          { label: 'Rückgängig', role: 'undo' },
          { label: 'Wiederholen', role: 'redo' },
          { type: 'separator' },
          { label: 'Ausschneiden', role: 'cut' },
          { label: 'Kopieren', role: 'copy' },
          { label: 'Einfügen', role: 'paste' },
          { label: 'Alles auswählen', role: 'selectAll' },
        ],
      },
    ]),
  );
}

// ------------------------------------------------------------ Unterwegs --

function melden(titel, text) {
  if (Notification.isSupported()) new Notification({ title: titel, body: text }).show();
}

function titelSetzen() {
  if (!fenster) return;
  const offen = offline ? offline.offeneAenderungen() : 0;
  fenster.setTitle(
    modus === 'offline'
      ? `Nexora (offline${offen ? `, ${offen} Änderung${offen === 1 ? '' : 'en'} warten` : ''})`
      : 'Nexora',
  );
}

function offlineZeigen() {
  if (modus === 'offline') return;
  modus = 'offline';
  fenster.loadURL(offline.startAdresse());
  titelSetzen();
}

function lokalZeigen() {
  modus = 'lokal';
  fenster.loadURL(LOKAL);
  titelSetzen();
}

function onlineZeigen() {
  const wechsel = modus !== 'online';
  modus = 'online';
  if (wechsel) fenster.loadURL(adresseLesen());
  titelSetzen();
}

/**
 * Ist der Server da? Dann erst den Ausgang senden, dann den Spiegel
 * auffrischen, dann das Fenster auf den Server stellen. Ohne Server: Spiegel.
 */
async function pruefen(vonHand = false) {
  if (abgleichLaeuft || !offline || modus === 'lokal' || ohneServer()) return;
  abgleichLaeuft = true;
  const server = adresseLesen();
  try {
    if (!(await offline.erreichbar(server))) {
      if (offline.hatSpiegel()) offlineZeigen();
      else if (vonHand) melden('Nexora', 'Der Server ist nicht erreichbar.');
      return;
    }
    if (!(await offline.angemeldet(server))) {
      // Erst anmelden; der Ausgang wartet so lange.
      const offen = offline.offeneAenderungen();
      if (modus !== 'online' && offen) {
        melden('Nexora', `Bitte anmelden, dann werden ${offen} Änderung${offen === 1 ? '' : 'en'} übertragen.`);
      }
      onlineZeigen();
      return;
    }
    const warteten = offline.offeneAenderungen();
    if (warteten) {
      const e = await offline.ausgangSenden(server);
      const teile = [`${e.gesendet} Änderung${e.gesendet === 1 ? '' : 'en'} übertragen`];
      if (e.zusammengefuehrt) teile.push(`${e.zusammengefuehrt} Seite${e.zusammengefuehrt === 1 ? '' : 'n'} zusammengeführt`);
      if (e.konflikte) teile.push(`${e.konflikte} Stelle${e.konflikte === 1 ? '' : 'n'} doppelt, bitte ansehen`);
      if (e.fehlgeschlagen.length) teile.push(`${e.fehlgeschlagen.length} nicht möglich`);
      melden('Nexora abgeglichen', teile.join(', ') + '.');
    }
    // Den Spiegel nicht bei jedem Blick auf den Server neu ziehen: alle zwei
    // Minuten reicht, damit er beim Losgehen aktuell ist.
    if (vonHand || warteten || !letzterAbgleich || Date.now() - letzterAbgleich > 120_000) {
      await offline.spiegeln(server);
      letzterAbgleich = new Date();
    }
    if (modus !== 'online') {
      onlineZeigen();
    } else if (warteten) {
      fenster.reload();
    }
    if (vonHand && !warteten) melden('Nexora', 'Alles abgeglichen.');
  } catch (f) {
    if (f && f.fremdesKonto) {
      if (!fremdGemeldet) melden('Nexora', 'Es warten Änderungen eines anderen Kontos. Melde dich mit diesem Konto an, dann werden sie übertragen.');
    } else if (vonHand) melden('Abgleich nicht möglich', String(f.message || f));
  } finally {
    abgleichLaeuft = false;
    titelSetzen();
  }
}

function standZeigen() {
  const st = offline && offline.stand;
  const seiten = st ? Object.keys(st.seiten).length : 0;
  dialog.showMessageBox(fenster, {
    type: 'info',
    title: 'Abgleich',
    message: modus === 'offline' ? 'Offline: du arbeitest auf dem Spiegel.' : 'Online.',
    detail: [
      `Seiten im Spiegel: ${seiten}`,
      `Wartende Änderungen: ${offline ? offline.offeneAenderungen() : 0}`,
      `Letzter Abgleich: ${letzterAbgleich ? letzterAbgleich.toLocaleString('de-DE') : (st && st.zuletzt ? new Date(st.zuletzt).toLocaleString('de-DE') : 'noch keiner')}`,
      `Server: ${adresseLesen()}`,
    ].join('\n'),
  });
}

async function starten() {
  offline = offline || new Offline(spiegelOrdner(adresseLesen()));
  await offline.starten();
  // Alle 30 Sekunden schauen, ob man zuhause ist; online frischt das den
  // Spiegel auf, damit er beim Losgehen aktuell ist.
  setInterval(() => {
    if (modus !== 'einrichtung') pruefen();
  }, 30_000);
  if (!eingerichtet()) {
    einrichtungZeigen();
    return;
  }
  if (ohneServer()) {
    lokalZeigen();
    return;
  }
  const server = adresseLesen();
  if (await offline.erreichbar(server)) {
    onlineZeigen();
    pruefen();
  } else if (offline.hatSpiegel()) {
    offlineZeigen();
  } else {
    fenster.loadURL(server); // zeigt die Meldung "nicht erreichbar"
  }
}

app.whenReady().then(() => {
  zertifikatePruefen();
  lokalAusliefern();
  einrichtungVerdrahten();
  menueBauen();
  fensterBauen();
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) fensterBauen();
  });
});

app.on('window-all-closed', () => {
  // Auf macOS bleibt eine Anwendung ueblicherweise laufen, wenn das letzte
  // Fenster zugeht; auf den anderen beiden nicht.
  if (process.platform !== 'darwin') app.quit();
});
