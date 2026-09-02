// Der Einstieg, den die Probe bündelt.
//
// Yjs darf nur EINMAL im Speicher liegen: es prüft Objekte über Konstruktoren,
// und zwei Kopien führen zu Fehlern, die nach einem Fehler im Programm
// aussehen. Deshalb reicht dieses Modul dieselbe Kopie heraus, die auch die
// Leitung benutzt, statt die Probe eine zweite ziehen zu lassen.
export { Leitung } from "../src/mitschrift";
export * as Y from "yjs";
