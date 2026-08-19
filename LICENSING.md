# Lizenzierung

Nexora ist **Open Core**. Die beiden Hälften tragen verschiedene Lizenzen.

| Teil | Lizenz | Was darin steckt |
|---|---|---|
| alles außer `backend/premium` | **Apache 2.0** ([LICENSE](LICENSE)) | die vollständige Anwendung |
| `backend/premium` | **BSL 1.1** ([backend/premium/LICENSE](backend/premium/LICENSE)) | die Prüfung des Lizenzschlüssels |

GitHub weist das Projekt als Apache 2.0 aus. Das ist richtig für alles, was
man beim Klonen bekommt und laufen lässt — mit der einen Ausnahme dieses
Unterverzeichnisses, das seine eigene Lizenzdatei mitbringt.

## Was frei ist

Der Kern ist ein vollständiges Wiki und bleibt es: Editor, verschachtelte
Seiten, Spaces, Tags, Suche, Wissensgraph, Papierkorb, Konten und Rollen.

Apache 2.0 heißt, was es heißt. Du darfst den Kern produktiv betreiben,
kommerziell einsetzen, verändern, weitergeben, forken und verkaufen, ohne
jemanden zu fragen und ohne etwas zu bezahlen.

## Was kostenpflichtig ist

Zwölf Zusätze fragen nach einem Lizenzschlüssel:

| Zusatz | Name im Schlüssel | Ohne Schlüssel |
|---|---|---|
| Versionsverlauf | `versionen` | Schnappschüsse laufen weiter, Ansehen und Zurückholen gesperrt |
| Anhänge | `anhaenge` | kein Upload-Bereich |
| Teilen und öffentliche Links | `freigeben` | Seiten bleiben bei ihrem Eigentümer |
| Prüfspur | `pruefspur` | Aufzeichnung läuft weiter, Lesen gesperrt |
| Gruppen und Space-Rechte | `gruppen` | Rechte je Seite wie bisher |
| SSO über OIDC | `sso` | Anmeldung mit Passwort |
| LDAP und Active Directory | `ldap` | Anmeldung mit Passwort |
| Volltext in Anhängen | `anhangsuche` | Suche über Seiten, nicht über Dateien |
| Space-Export | `export` | Export je Seite als Markdown |
| Vorlagen | `vorlagen` | leere Seiten |
| Kommentare | `kommentare` | keine Kommentare |
| Konflikterkennung | `konflikte` | letzter Schreibvorgang gewinnt |

Zwei davon zeichnen auch ohne Lizenz weiter auf: **Versionsverlauf** und
**Prüfspur**. Andernfalls klaffte nach dem Freischalten eine Lücke genau über
dem unlizenzierten Zeitraum — und eine Prüfspur mit einem Loch ist keine.

Der Server weist gesperrte Aufrufe mit `402 Payment Required` ab, die
Oberfläche blendet die zugehörigen Bedienelemente aus. Das Ausblenden ist
Höflichkeit, die Abweisung ist der Schutz.

## Ohne die bezahlte Hälfte bauen

```bash
rm -rf backend/premium
cd backend && go build -tags nur_kern ./...
```

Das ergibt ein Programm unter reinem Apache 2.0. Jeder Zusatz antwortet dann
mit `402`, alles andere läuft unverändert. Möglich ist das, weil das Tor
selbst auf der Apache-Seite in `internal/lizenz` sitzt und nichts von
Signaturen weiß: es fragt nur, wer sich als Prüfer registriert hat.

## Was die BSL bedeutet

Die Business Source License 1.1 ist **keine** Open-Source-Lizenz im Sinne der
OSI. Du darfst den Code in `backend/premium` lesen, verändern, bauen und
testen — auch um zu bewerten, ob sich ein Kauf lohnt. Was ohne Lizenzschlüssel
nicht erlaubt ist: ihn produktiv einsetzen, um die Zusätze freizuschalten.

**Am 19.08.2030 fällt die Beschränkung weg.** Ab dann gilt auch für dieses
Verzeichnis Apache 2.0. Das steht so in der Lizenz und ist nicht widerruflich.

## Ein Schlüssel lässt sich nicht zurückziehen

Geprüft wird offline gegen einen eingebauten Ed25519-Schlüssel. Kein
Lizenzserver, kein Heimtelefonieren, funktioniert in Netzen ohne Internet.

Der Preis dafür: ein ausgestellter Schlüssel bleibt gültig. Ein Ablaufdatum
ist der einzige Hebel, weshalb Schlüssel für zahlende Kunden eines tragen
sollten.

## Und der Quelltext liegt offen

Wer `backend/premium/lizenz/pruefer.go` liest, findet die Zeile, die die
Signatur prüft, und kann sie entfernen. Das ist bei jeder Software so, die auf
fremden Rechnern läuft — pfSense, GitLab, Sentry, Elastic arbeiten alle so.

Was schützt, ist nicht die Technik, sondern die Lizenz: ein solcher Eingriff
ist ein Lizenzverstoß, kein Kniff. Und wer ihn vornimmt, hätte ohnehin nicht
bezahlt.

## Lizenz erwerben

Für kommerzielle Lizenzen und abweichende Vereinbarungen wende dich an den
Lizenzgeber, Jonas Groll.
