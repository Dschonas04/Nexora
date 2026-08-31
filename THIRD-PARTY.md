# Fremde Bestandteile

Nexora steht unter BUSL-1.1, ist aber nicht allein aus eigenem Quelltext gebaut.
Diese Datei sagt, was mitgeliefert wird und unter welchen Bedingungen.

Aufgeführt ist, was **ausgeliefert** wird, und nicht, was in den Paketdateien
steht. Der Unterschied ist nicht kosmetisch: `go.mod` nennt 48 Module,
eingebunden werden 34. Die Differenz ist der Kerberos-Zweig von `go-ldap`, den
kein Aufruf erreicht und der deshalb nicht mitkompiliert wird; mit ihm bleibt
das einzige MPL-lizenzierte Go-Modul draußen. Auf der npm-Seite liegen 317
Pakete im Verzeichnis, 162 landen im Bündel, der Rest sind Bauwerkzeuge.

Stand: 31.08.2026. Nachzählen lässt sich das jederzeit:

```bash
cd backend  && go list -deps -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}' ./... | sort -u
cd frontend && npm ls --omit=dev --all
```

## Womit eine Auflage verbunden ist

Genau ein Bestandteil des Quelltextbestands verlangt mehr als den Hinweis, und
einer der beiden Abbilder tut es auch. Alles andere ist MIT, BSD, Apache-2.0
oder ISC.

### BlockNote, der Editor: MPL-2.0

`@blocknote/core`, `@blocknote/react` und `@blocknote/mantine`, jeweils 0.15.11.

Die Mozilla Public License 2.0 ist **dateiweises** Copyleft und nicht virales.
Der eigene Quelltext bleibt davon unberührt, und das Erzeugnis darf verkauft
werden. Verlangt ist zweierlei:

1. Der Lizenzhinweis muss die **verteilte Form** begleiten. Verteilt wird das
   gebaute Bündel, und der Minimierer wirft Kommentare weg, auch die
   Lizenzköpfe der Pakete. Deshalb setzt `frontend/vite.config.ts` einen
   eigenen Kopf, der in jedem erzeugten Bündel steht. Wird dort etwas geändert,
   ist das die Stelle, an der die Auflage bricht, ohne dass es auffällt.
2. Änderungen an den MPL-Dateien selbst müssten unter MPL bleiben und verfügbar
   sein. Nexora benutzt die Pakete unverändert aus der Registry; der Quelltext
   liegt unter <https://github.com/TypeCellOS/BlockNote>.

### Das Laufzeit-Abbild: GPL-2.0

`backend/Dockerfile` setzt auf Alpine auf und installiert `poppler-utils` für
`pdftotext`, das den Volltext aus PDF-Anhängen zieht. Damit enthält das
verteilte Abbild Programme unter GPL:

| Paket | Lizenz | warum drin |
|---|---|---|
| poppler-utils, poppler | GPL-2.0-or-later | `pdftotext`, Volltext aus PDF-Anhängen |
| busybox, busybox-binsh, ssl_client | GPL-2.0-only | die Shell des Abbilds |
| alpine-baselayout, apk-tools, scanelf | GPL-2.0-only | Alpine selbst |
| libgcc, libstdc++ | GPL-2.0+ mit Laufzeitausnahme | Systembibliotheken |
| cairo | LGPL-2.1-or-later oder MPL-1.1 | über poppler |
| freetype, zstd-libs | Doppellizenz mit GPL-Möglichkeit | über poppler |
| musl | MIT | die C-Bibliothek |

Das Frontend-Abbild enthält kein poppler, wohl aber busybox aus derselben
Grundlage.

Zwei Fragen sind dabei auseinanderzuhalten.

**Wird Nexora dadurch GPL?** Nein. `pdftotext` wird als eigener Prozess über
eine Pipe aufgerufen, nichts davon wird eingebunden. Das ist ein unabhängiger
Aufruf und kein abgeleitetes Werk; das Go-Programm bleibt unter BUSL-1.1.

**Wer das Abbild weitergibt, verteilt GPL-Programme mit** und schuldet dafür den
zugehörigen Quelltext. Das Angebot dazu steht in [LICENSING.md](LICENSING.md).
Alpine veröffentlicht die Quellen aller Pakete unter
<https://gitlab.alpinelinux.org/alpine/aports>. Beide Abbilder tragen die Angabe
zusätzlich als Kennzeichnung bei sich, damit sie auch dann mitreist, wenn nur
das Abbild weitergereicht wird.

Was ein gebautes Abbild wirklich enthält:

```bash
docker run --rm --entrypoint sh nexora-backend -lc \
  "apk list -I | sed -E 's/^([^ ]+).*\((.*)\).*/\2  \1/' | sort"
```

Wer die GPL-Bestandteile vermeiden will, streicht `poppler-utils` aus
`backend/Dockerfile`. Dann fällt der Volltext aus PDF-Anhängen weg; eine reine
Go-Lösung scheitert an Schriftkodierungen, Spalten und eingebetteten Bildern in
echten PDF-Dateien.

## Go, eingebundene Module (34)

| Verteilung | |
|---|---|
| MIT | 17 |
| BSD-3-Clause | 8 |
| Apache-2.0 | 7 |
| BSD-2-Clause | 2 |

| Modul | Fassung | Lizenz |
|---|---|---|
| `github.com/Azure/go-ntlmssp` | v0.1.1 | MIT |
| `github.com/cespare/xxhash/v2` | v2.3.0 | MIT |
| `github.com/coreos/go-oidc/v3` | v3.20.0 | Apache-2.0 |
| `github.com/dustin/go-humanize` | v1.0.1 | MIT |
| `github.com/go-asn1-ber/asn1-ber` | v1.5.8 | MIT |
| `github.com/go-chi/chi/v5` | v5.0.12 | MIT |
| `github.com/go-jose/go-jose/v4` | v4.1.4 | Apache-2.0 |
| `github.com/go-ldap/ldap/v3` | v3.4.14 | MIT |
| `github.com/golang-jwt/jwt/v5` | v5.2.1 | MIT |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause |
| `github.com/jackc/pgpassfile` | v1.0.0 | MIT |
| `github.com/jackc/pgservicefile` | v0.0.0-20221227161230-091c0ba34f0a | MIT |
| `github.com/jackc/pgx/v5` | v5.5.5 | MIT |
| `github.com/jackc/puddle/v2` | v2.2.1 | MIT |
| `github.com/klauspost/compress` | v1.19.2 | Apache-2.0 |
| `github.com/klauspost/cpuid/v2` | v2.4.0 | MIT |
| `github.com/klauspost/crc32` | v1.3.0 | BSD-3-Clause |
| `github.com/minio/crc64nvme` | v1.1.1 | Apache-2.0 |
| `github.com/minio/md5-simd` | v1.1.2 | Apache-2.0 |
| `github.com/minio/minio-go/v7` | v7.3.0 | Apache-2.0 |
| `github.com/philhofer/fwd` | v1.2.0 | MIT |
| `github.com/redis/go-redis/v9` | v9.22.0 | BSD-2-Clause |
| `github.com/rs/xid` | v1.6.0 | MIT |
| `github.com/tinylib/msgp` | v1.6.4 | MIT |
| `github.com/zeebo/xxh3` | v1.1.0 | BSD-2-Clause |
| `go.uber.org/atomic` | v1.11.0 | MIT |
| `go.yaml.in/yaml/v3` | v3.0.5 | MIT |
| `golang.org/x/crypto` | v0.55.0 | BSD-3-Clause |
| `golang.org/x/net` | v0.58.0 | BSD-3-Clause |
| `golang.org/x/oauth2` | v0.36.0 | BSD-3-Clause |
| `golang.org/x/sync` | v0.22.0 | BSD-3-Clause |
| `golang.org/x/sys` | v0.47.0 | BSD-3-Clause |
| `golang.org/x/text` | v0.41.0 | BSD-3-Clause |
| `gopkg.in/ini.v1` | v1.67.3 | Apache-2.0 |

## npm, Produktivbaum (162)

| Verteilung | |
|---|---|
| MIT | 156 |
| MPL-2.0 | 3 |
| ISC | 1 |
| 0BSD | 1 |
| MIT oder CC0-1.0 | 1 |

Alle 162 einzeln aufzuführen hieße, eine Liste zu pflegen, die beim nächsten
`npm install` still veraltet. Namentlich stehen hier deshalb die acht direkten
Abhängigkeiten und jedes Paket im Baum, das nicht MIT ist. Der Rest ist der
übliche Unterbau von React und ProseMirror und durchgehend MIT; nachsehen lässt
er sich mit dem Befehl oben.

**Direkt:**

| Paket | Fassung | Lizenz |
|---|---|---|
| `@blocknote/core` | 0.15.11 | **MPL-2.0** |
| `@blocknote/mantine` | 0.15.11 | **MPL-2.0** |
| `@blocknote/react` | 0.15.11 | **MPL-2.0** |
| `@tiptap/core` | 2.27.2 | MIT |
| `@tiptap/pm` | 2.27.2 | MIT |
| `react` | 18.3.1 | MIT |
| `react-dom` | 18.3.1 | MIT |
| `react-router-dom` | 6.26.2 | MIT |

**Alles Übrige im Baum, das nicht MIT ist:**

| Paket | Fassung | Lizenz |
|---|---|---|
| `hast-util-from-dom` | 4.2.0 | ISC |
| `tslib` | 2.8.1 | 0BSD |
| `type-fest` | 4.41.0 | MIT oder CC0-1.0 |

## Nur beim Bauen, nicht im Erzeugnis

TypeScript (Apache-2.0), Vite (MIT), `@vitejs/plugin-react` (MIT) sowie deren
Unterbau. Zwei davon fallen beim Durchsehen auf und seien darum genannt, damit
niemand sie für einen Fund hält: `caniuse-lite` steht unter CC-BY-4.0 und
`argparse` unter Python-2.0. Beides sind Bauwerkzeuge, beides gerät nie in ein
ausgeliefertes Bündel.

Auf der Go-Seite gilt dasselbe für `stretchr/testify`, `davecgh/go-spew` und
`pmezard/go-difflib`: Prüfwerkzeuge, alle MIT oder ISC, nicht im Binärprogramm.
