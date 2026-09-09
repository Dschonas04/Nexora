// Nexora als Anwendung auf dem Schreibtisch.
//
// Es ist bewusst kein zweites Nexora, sondern ein Fenster auf das, das schon
// laeuft: dieselbe Oberflaeche, derselbe Server, derselbe Stand fuer alle.
// Ein eigener Datenbestand in der Anwendung wuerde genau das aufgeben, wofuer
// Nexora da ist.
//
// Was die Anwendung gegenueber einem Lesezeichen im Browser bringt: ein
// eigenes Fenster ohne Adresszeile und fremde Reiter, ein Eintrag im
// Startmenue, und die Adresse der Instanz steht einmal fest statt in jedem
// Browserprofil neu.
const { app, BrowserWindow, Menu, dialog, shell } = require('electron');
const fs = require('fs');
const path = require('path');

const EINSTELLUNGEN = () => path.join(app.getPath('userData'), 'einstellungen.json');
const VORGABE = 'http://10.0.2.43:3000';

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

  fenster.loadURL(adresseLesen());

  // Verweise nach draussen gehoeren in den Browser, nicht in dieses Fenster:
  // sonst landet man in einer Anwendung ohne Zurueck-Knopf auf einer fremden
  // Seite.
  fenster.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url);
    return { action: 'deny' };
  });

  fenster.webContents.on('did-fail-load', (_e, code, beschreibung) => {
    if (code === -3) return; // abgebrochene Weiterleitung, kein Fehler
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
