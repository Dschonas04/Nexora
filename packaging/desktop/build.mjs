/* Baut die Schreibtisch-Fassungen ohne fremde Werkzeugkette.
 *
 * electron-packager verlangt auf Linux Wine, sobald ein Windows-Ziel
 * Metadaten bekommen soll -- und Metadaten kommen schon aus der
 * package.json. Wine nur fuer den Namen im Dateidialog zu installieren, waere
 * ein Gigabyte fuer nichts.
 *
 * Was der Packer sonst tut, sind drei Schritte: die fertige Electron-Fassung
 * fuer die Zielplattform holen, die eigene Anwendung nach resources/app
 * legen, und die Binaerdatei umbenennen. Genau das steht hier.
 */
import { downloadArtifact } from '@electron/get';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HIER = path.join(path.dirname(fileURLToPath(import.meta.url)), '..');
const QUELLE = path.join(HIER, 'desktop');
const ZIEL = path.join(HIER, 'fertig');
const VERSION = JSON.parse(fs.readFileSync(path.join(QUELLE, 'node_modules/electron/package.json'))).version;

const ZIELE = [
  { platform: 'win32', arch: 'x64', ordner: 'Nexora-windows-x64', binaer: 'electron.exe', neu: 'Nexora.exe' },
  { platform: 'darwin', arch: 'arm64', ordner: 'Nexora-macos-arm64', app: true },
  { platform: 'darwin', arch: 'x64', ordner: 'Nexora-macos-x64', app: true },
  { platform: 'linux', arch: 'x64', ordner: 'Nexora-linux-x64', binaer: 'electron', neu: 'nexora' },
];

function anwendungKopieren(nachResources) {
  const app = path.join(nachResources, 'app');
  fs.mkdirSync(app, { recursive: true });
  for (const datei of ['haupt.js', 'offline.js', 'zusammenfuehren.js', 'vorab.js', 'einrichten.html', 'homelab-root-ca.pem', 'package.json']) {
    fs.copyFileSync(path.join(QUELLE, datei), path.join(app, datei));
  }
  // Die Anwendung hat keine Laufzeit-Abhaengigkeiten: alles, was sie braucht,
  // steckt in Electron selbst. Deshalb wandert kein node_modules mit.
  // Die Oberflaeche fuer den Betrieb ohne Server (frontend/dist).
  const dist = path.join(HIER, '..', 'frontend', 'dist');
  if (!fs.existsSync(path.join(dist, 'index.html'))) throw new Error('frontend/dist fehlt: erst im Frontend "npx vite build"');
  fs.cpSync(dist, path.join(app, 'oberflaeche'), { recursive: true });
  const p = JSON.parse(fs.readFileSync(path.join(app, 'package.json')));
  delete p.devDependencies;
  delete p.scripts;
  fs.writeFileSync(path.join(app, 'package.json'), JSON.stringify(p, null, 2));
}

// NEXORA_ZIEL=darwin-arm64 baut nur dieses eine Ziel.
const nur = process.env.NEXORA_ZIEL;

for (const z of ZIELE.filter((z) => !nur || `${z.platform}-${z.arch}` === nur)) {
  const ordner = path.join(ZIEL, z.ordner);
  console.log(`\n== ${z.ordner}`);
  const zip = await downloadArtifact({
    version: VERSION,
    platform: z.platform,
    arch: z.arch,
    artifactName: 'electron',
  });
  fs.rmSync(ordner, { recursive: true, force: true });
  fs.mkdirSync(ordner, { recursive: true });
  execFileSync('unzip', ['-q', zip, '-d', ordner]);

  if (z.app) {
    // macOS: die Anwendung ist ein Buendel. Electron.app wird umbenannt, der
    // Inhalt liegt darin.
    const alt = path.join(ordner, 'Electron.app');
    const neu = path.join(ordner, 'Nexora.app');
    fs.renameSync(alt, neu);
    anwendungKopieren(path.join(neu, 'Contents', 'Resources'));
    const plistPfad = path.join(neu, 'Contents', 'Info.plist');
    let plist = fs.readFileSync(plistPfad, 'utf8');
    plist = plist
      .replace(/<string>Electron<\/string>/g, '<string>Nexora</string>')
      .replace(/com\.github\.Electron/g, 'de.jonasgroll.nexora');
    fs.writeFileSync(plistPfad, plist);
    fs.renameSync(
      path.join(neu, 'Contents', 'MacOS', 'Electron'),
      path.join(neu, 'Contents', 'MacOS', 'Nexora'),
    );
    // Das Zeichen von Nexora statt des Electron-Atoms im Dock.
    const icns = path.join(QUELLE, 'nexora.icns');
    if (fs.existsSync(icns)) fs.copyFileSync(icns, path.join(neu, 'Contents', 'Resources', 'electron.icns'));
    // Ein Abbild zum Weitergeben. genisoimage schreibt ein Apple-ISO, das macOS
    // wie jedes andere Abbild einhaengt.
    try {
      const dmg = path.join(ZIEL, `${z.ordner}.dmg`);
      fs.rmSync(dmg, { force: true });
      fs.symlinkSync('/Applications', path.join(ordner, 'Programme'));
      execFileSync('genisoimage', ['-quiet', '-V', 'Nexora', '-D', '-R', '-apple', '-no-pad', '-o', dmg, ordner]);
      fs.rmSync(path.join(ordner, 'Programme'));
      console.log('   Abbild:', dmg);
    } catch (f) {
      console.log('   kein Abbild:', f.message);
    }
  } else {
    anwendungKopieren(path.join(ordner, 'resources'));
    fs.renameSync(path.join(ordner, z.binaer), path.join(ordner, z.neu));
  }
  console.log('   fertig:', ordner);
}
