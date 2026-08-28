# Lizenzierung

Nexora steht vollständig unter der **Business Source License 1.1** — siehe
[LICENSE](LICENSE).

Das ist keine Open-Source-Lizenz im Sinne der OSI, aber auch keine geschlossene:
der Quelltext liegt offen, und der Additional Use Grant erlaubt ausdrücklich den
produktiven, auch kommerziellen Einsatz — solange die kostenpflichtigen Zusätze
nicht ohne Schlüssel benutzt werden.

**Am 19.08.2030 fällt die Beschränkung weg.** Ab dann gilt Apache 2.0. Das steht
so in der Lizenz und ist nicht widerruflich.

## Was ohne Schlüssel erlaubt ist

Der Kern darf produktiv laufen, auch in Firmen, auch kommerziell, ohne dass
jemand gefragt oder etwas bezahlt werden muss:

Editor, verschachtelte Seiten, Spaces, Tags, Favoriten, Papierkorb,
Volltextsuche über Seiten, Rückverweise, Wissensgraph, Konten und Rollen.

Lesen, verändern, bauen und testen ist für das **gesamte** Werk immer erlaubt,
auch zur kommerziellen Bewertung.

## Was einen Schlüssel braucht

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
| Export als PDF und Word, Space-Export | `export` | Export je Seite als Markdown |
| Kommentare | `kommentare` | keine Kommentare |
| Konflikterkennung | `konflikte` | letzter Schreibvorgang gewinnt |

Zwei davon zeichnen auch ohne Lizenz weiter auf: **Versionsverlauf** und
**Prüfspur**. Andernfalls klaffte nach dem Freischalten eine Lücke genau über
dem unlizenzierten Zeitraum — und eine Prüfspur mit einem Loch ist keine.

Der Server weist gesperrte Aufrufe mit `402 Payment Required` ab, die
Oberfläche blendet die zugehörigen Bedienelemente aus. Das Ausblenden ist
Höflichkeit, die Abweisung ist der Schutz.

## Wo die Prüfung sitzt

Die Schlüsselprüfung liegt in `backend/premium`, das Tor selbst in
`backend/internal/lizenz`. Das Tor weiß nichts von Signaturen — es fragt nur,
wer sich als Prüfer registriert hat. Deshalb lässt sich der Kern auch ohne das
Premium-Verzeichnis bauen:

```bash
rm -rf backend/premium
cd backend && go build -tags nur_kern ./...
```

Jeder Zusatz antwortet dann mit `402`, alles andere läuft unverändert.

## Ein Schlüssel lässt sich nicht zurückziehen

Geprüft wird offline gegen einen eingebauten Ed25519-Schlüssel. Kein
Lizenzserver, kein Heimtelefonieren, funktioniert in Netzen ohne Internet.

Der Preis dafür: ein ausgestellter Schlüssel bleibt gültig. Ein Ablaufdatum ist
der einzige Hebel, weshalb Schlüssel für zahlende Kunden eines tragen sollten.

## Und der Quelltext liegt offen

Wer `backend/premium/lizenz/pruefer.go` liest, findet die Zeile, die die
Signatur prüft, und kann sie entfernen. Das ist bei jeder Software so, die auf
fremden Rechnern läuft — pfSense, GitLab, Sentry und Elastic arbeiten alle so.

Was schützt, ist nicht die Technik, sondern die Lizenz: ein solcher Eingriff ist
ein Lizenzverstoß, kein Kniff. Und wer ihn vornimmt, hätte ohnehin nicht bezahlt.

## Lizenz erwerben

Für kommerzielle Lizenzen und abweichende Vereinbarungen wende dich an den
Lizenzgeber, Jonas Groll.
