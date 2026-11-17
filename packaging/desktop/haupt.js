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
const { app, BrowserWindow, Menu, Notification, dialog, shell } = require('electron');
const fs = require('fs');
const path = require('path');

const EINSTELLUNGEN = () => path.join(app.getPath('userData'), 'einstellungen.json');
const VORGABE = 'https://10.0.2.7:3000';
const { Offline } = require('./offline');
let offline = null;
let modus = 'start'; // 'online' oder 'offline'
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

function adresseSchreiben(adresse) {
  try {
    fs.mkdirSync(path.dirname(EINSTELLUNGEN()), { recursive: true });
    fs.writeFileSync(EINSTELLUNGEN(), JSON.stringify({ adresse }, null, 2));
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
    return new URL(adresse).origin === new URL(instanz).origin;
  } catch {
    return false;
  }
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
    // Erlaubt: die Instanz, der eigene Spiegel, und das Weitergehen innerhalb
    // der Seite, auf der das Fenster gerade steht. Letzteres braucht die
    // Anmeldung per SSO: der Server leitet zum Anbieter um (das ist eine
    // Weiterleitung und kein Klick), und dort schickt man ein Formular ab, das
    // auf demselben Anbieter bleibt. Ohne diese Ausnahme landete die Anmeldung
    // im Browser statt im Fenster, und die Anwendung blieb abgemeldet.
    const hier = fenster.webContents.getURL();
    const erlaubt =
      gleicherUrsprung(ziel, adresseLesen()) ||
      (offline && gleicherUrsprung(ziel, `http://127.0.0.1:${offline.port}`)) ||
      (hier && !gleicherUrsprung(hier, adresseLesen()) && gleicherUrsprung(ziel, hier) && !hier.startsWith('http://127.0.0.1'));
    if (!erlaubt) {
      ereignis.preventDefault();
      if (nachDraussenErlaubt(ziel)) shell.openExternal(ziel);
    }
  });

  fenster.webContents.on('did-fail-load', (_e, code, beschreibung, _url, hauptRahmen) => {
    if (code === -3 || !hauptRahmen) return; // abgebrochene Weiterleitung, kein Fehler
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
        buttons: ['Erneut versuchen', 'Adresse ändern', 'Beenden'],
        defaultId: 0,
        cancelId: 2,
      })
      .then(({ response }) => {
        if (response === 0) fenster.loadURL(adresseLesen());
        else if (response === 1) adresseAendern();
        else app.quit();
      });
  });
}

async function adresseAendern() {
  // Electron hat keinen Eingabedialog. Statt dafuer ein zweites Fenster mit
  // eigener Oberflaeche zu bauen, fragt eine kleine Seite im Fenster selbst.
  const eingabe = await fenster.webContents
    .executeJavaScript(
      `window.prompt(${JSON.stringify('Adresse der Nexora-Instanz')}, ${JSON.stringify(adresseLesen())})`,
    )
    .catch(() => null);
  if (!eingabe) return;
  let adresse = String(eingabe).trim();
  if (!/^https?:\/\//.test(adresse)) adresse = 'http://' + adresse;
  adresseSchreiben(adresse);
  fenster.loadURL(adresse);
}

function menueBauen() {
  Menu.setApplicationMenu(
    Menu.buildFromTemplate([
      {
        label: 'Nexora',
        submenu: [
          { label: 'Neu laden', accelerator: 'CmdOrCtrl+R', click: () => fenster.reload() },
          { label: 'Adresse ändern…', click: adresseAendern },
          { label: 'Jetzt abgleichen', accelerator: 'CmdOrCtrl+Shift+S', click: () => pruefen(true) },
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
  if (abgleichLaeuft || !offline) return;
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
  offline = offline || new Offline(path.join(app.getPath('userData'), 'offline'));
  await offline.starten();
  const server = adresseLesen();
  if (await offline.erreichbar(server)) {
    onlineZeigen();
    pruefen();
  } else if (offline.hatSpiegel()) {
    offlineZeigen();
  } else {
    fenster.loadURL(server); // zeigt die Meldung "nicht erreichbar"
  }
  // Alle 30 Sekunden schauen, ob man zuhause ist; online frischt das den
  // Spiegel auf, damit er beim Losgehen aktuell ist.
  setInterval(() => pruefen(), 30_000);
}

app.whenReady().then(() => {
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
