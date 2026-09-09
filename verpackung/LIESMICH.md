# Nexora zum Mitnehmen

*This guide is also available [in English](README.md).*

Nexora ist eine Webanwendung. Was hier entsteht, ist kein zweites Nexora,
sondern ein eigenes Fenster auf das, das schon läuft: dieselbe Oberfläche,
derselbe Server, derselbe Stand für alle. Ein eigener Datenbestand in der App
gäbe genau das auf, wofür Nexora da ist.

Was die App gegenüber einem Lesezeichen bringt: ein Fenster ohne Adresszeile
und fremde Reiter, ein Eintrag im Startmenü oder auf dem Startbildschirm, und
die Adresse der Instanz steht einmal fest statt in jedem Browserprofil neu.

## Was gebaut wird

| Ziel | Ergebnis | Baut auf |
|---|---|---|
| Windows | `Nexora-windows-x64` mit `Nexora.exe` | Linux |
| macOS | `Nexora-macos-arm64.dmg`, `Nexora-macos-x64.dmg` | Linux |
| Linux | `Nexora-linux-x64` mit `nexora` | Linux |
| Android | `Nexora-android.apk` | Linux, Android-SDK im Container |
| iOS | Xcode-Projekt unter `mobil/ios` | **nur macOS mit Xcode** |

## Schreibtisch (Windows, macOS, Linux)

```
cd schreibtisch && npm install
node bauen.mjs
```

`bauen.mjs` holt die fertige Electron-Fassung je Plattform, legt die Anwendung
nach `resources/app` und benennt die Binärdatei um. Genau das tut ein Packer
auch — nur verlangt `electron-packager` auf Linux Wine, sobald ein
Windows-Ziel Metadaten bekommen soll, und Wine allein für den Namen im
Dateidialog ist ein Gigabyte für nichts.

Das DMG entsteht mit `genisoimage` als Apple-ISO. macOS hängt es ein wie ein
normales Abbild.

## Android

```
cd mobil && npm install
npx cap sync android
```

Gebaut wird im Container mit dem Android-SDK, auf dem Docker-Host:

```
docker run --rm -v <pfad>/mobil:/build -v nexora-gradle-cache:/root/.gradle \
  -w /build/android hecht-android:2 ./gradlew --no-daemon assembleDebug
```

Das Ergebnis liegt unter `android/app/build/outputs/apk/debug/app-debug.apk`.

Zu beachten: das ist ein *Debug*-Build. Nur der bringt `usesCleartextTraffic`
von selbst mit, und ohne das verweigert Android ab Version 9 unverschlüsseltes
HTTP wortlos — die App zeigte eine leere Fläche. Ein Release-Build braucht die
Angabe ausdrücklich, oder die Instanz braucht TLS.

## iOS

Das Projekt liegt unter `mobil/ios` und ist vollständig, aber ein `.ipa`
entsteht nur auf einem Mac mit Xcode, und installieren lässt es sich nur mit
einem Apple-Entwicklerkonto. Ohne Konto bleibt der Weg über den Browser:
Nexora im Safari öffnen, Teilen, "Zum Home-Bildschirm". Das gibt ein Symbol
ohne Adresszeile — praktisch dasselbe wie die App.

## Adresse der Instanz

Vorgabe ist `http://10.0.2.43:3000`.

* Schreibtisch: im Menü unter *Nexora → Adresse ändern…*, gemerkt wird sie in
  den Anwendungsdaten. Alternativ die Umgebungsvariable `NEXORA_ADRESSE`.
* Android und iOS: in `capacitor.config.json`, danach `npx cap sync`.

## Was das Fenster darf

Die Hülle reicht nur `http` und `https` an das Betriebssystem weiter.
`shell.openExternal` gibt eine Adresse an das System, und das öffnet `file:`,
`smb:` oder `ms-msdt:` genauso bereitwillig — eine Seite könnte darüber sonst
Programme auf dem Rechner anstoßen.

Das Fenster selbst bleibt außerdem beim Ursprung seiner Instanz. Ohne das
könnte ein Verweis im Inhalt das Fenster auf eine fremde Seite umlenken, und
die sähe aus wie Nexora, samt Feld für das Passwort. Verweise nach außen gehen
stattdessen an den Browser.

## Signaturen

Nichts davon ist signiert.

* Windows meldet beim ersten Start SmartScreen: *Weitere Informationen →
  Trotzdem ausführen*.
* macOS verweigert einen Doppelklick: einmal Rechtsklick → *Öffnen*, dann
  merkt es sich das.
* Android will "Installation aus unbekannten Quellen" für die App, aus der du
  das APK öffnest.

Das zu ändern kostet Geld und Zeit: ein Windows-Zertifikat im Jahr, ein
Apple-Entwicklerkonto im Jahr. Für den Hausgebrauch im eigenen Netz ist es
das nicht wert.
