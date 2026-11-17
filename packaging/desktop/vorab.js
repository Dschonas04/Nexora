// Bruecke fuer die Einrichtungsseite (einrichten.html). Sie haengt an jedem
// Fenster, aber haupt.js beantwortet die Aufrufe nur, wenn sie von der lokalen
// Einrichtungsseite kommen; eine Seite des Servers kommt damit an nichts.
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('nexoraEinrichtung', {
  adresse: () => ipcRenderer.invoke('einrichtung:adresse'),
  pruefen: (adresse) => ipcRenderer.invoke('einrichtung:pruefen', String(adresse)),
  speichern: (adresse) => ipcRenderer.invoke('einrichtung:speichern', String(adresse)),
});
