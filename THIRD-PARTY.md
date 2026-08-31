# Fremde Bestandteile

**Diese Datei wird erzeugt. Nicht von Hand ändern**, sondern
`python3 scripts/lizenzen.py` aufrufen. Die CI prüft bei jedem Lauf, dass sie
zum Stand der Abhängigkeiten passt.

Aufgeführt ist, was **ausgeliefert** wird: auf der Go-Seite die Module, die der
Binder wirklich einbindet, auf der npm-Seite der Produktivbaum ohne
Bauwerkzeuge. Was nur auf der Platte liegt, aber nie in ein Erzeugnis gerät,
steht nicht hier.


## Womit eine Auflage verbunden ist

Alles andere unten ist MIT, BSD, Apache-2.0 oder ISC und verlangt außer dem
Hinweis nichts. Die folgenden Bestandteile verlangen mehr:


| Bestandteil | Fassung | Lizenz |
|---|---|---|
| `@blocknote/core` | 0.15.11 | **MPL-2.0** |
| `@blocknote/mantine` | 0.15.11 | **MPL-2.0** |
| `@blocknote/react` | 0.15.11 | **MPL-2.0** |

**MPL-2.0** ist dateiweises Copyleft. Der eigene Quelltext bleibt davon
unberührt, und das Erzeugnis darf verkauft werden. Verlangt ist zweierlei:
der Lizenzhinweis muss die verteilte Form begleiten, und Änderungen an den
MPL-Dateien selbst müssen unter MPL bleiben und verfügbar sein. Nexora
benutzt diese Pakete unverändert aus der Registry; der Quelltext liegt
unter <https://github.com/TypeCellOS/BlockNote>.


## Das Laufzeit-Abbild

Der wichtigste Teil steht in keiner Paketdatei. `backend/Dockerfile` setzt auf
Alpine auf und installiert `poppler-utils` für `pdftotext`. Damit enthält das
verteilte Abbild Programme unter **GPL-2.0**:


| Paket | Lizenz | warum |
|---|---|---|
| poppler-utils, poppler | GPL-2.0-or-later | pdftotext, Volltext aus PDF-Anhängen |
| busybox, busybox-binsh, ssl_client | GPL-2.0-only | die Shell des Abbilds |
| alpine-baselayout, apk-tools, scanelf | GPL-2.0-only | Alpine selbst |
| libgcc, libstdc++ | GPL-2.0+ mit Laufzeitausnahme | Systembibliotheken |
| cairo | LGPL-2.1-or-later oder MPL-1.1 | über poppler |
| freetype, zstd-libs | Doppellizenz mit GPL-Möglichkeit | über poppler |
| musl | MIT | die C-Bibliothek |

Zwei Fragen sind dabei auseinanderzuhalten.

**Wird Nexora dadurch GPL?** Nein. `pdftotext` wird als eigener Prozess
aufgerufen, über eine Pipe, ohne Bindung. Das ist ein unabhängiger Aufruf und
kein abgeleitetes Werk; das Go-Programm bleibt unter seiner eigenen Lizenz.

**Wer das Abbild weitergibt, verteilt GPL-Programme** und schuldet dafür den
zugehörigen Quelltext. Das Angebot dazu steht in [LICENSING.md](LICENSING.md).
Alpine veröffentlicht die Quellen aller Pakete unter
<https://gitlab.alpinelinux.org/alpine/aports>.

Nachsehen, was ein gebautes Abbild wirklich enthält:

```bash
docker run --rm --entrypoint sh nexora-backend -lc \
  "apk list -I | sed -E 's/^([^ ]+).*\\((.*)\\).*/\\2  \\1/' | sort"
```


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
| (MIT OR CC0-1.0) | 1 |
| 0BSD | 1 |
| ISC | 1 |

| Paket | Fassung | Lizenz |
|---|---|---|
| `@babel/runtime` | 7.29.7 | MIT |
| `@blocknote/core` | 0.15.11 | MPL-2.0 |
| `@blocknote/mantine` | 0.15.11 | MPL-2.0 |
| `@blocknote/react` | 0.15.11 | MPL-2.0 |
| `@emoji-mart/data` | 1.2.1 | MIT |
| `@floating-ui/react` | 0.26.28 | MIT |
| `@mantine/core` | 7.17.8 | MIT |
| `@mantine/hooks` | 7.17.8 | MIT |
| `@mantine/utils` | 6.0.22 | MIT |
| `@remix-run/router` | 1.23.3 | MIT |
| `@tiptap/core` | 2.27.2 | MIT |
| `@tiptap/extension-bold` | 2.27.2 | MIT |
| `@tiptap/extension-code` | 2.27.2 | MIT |
| `@tiptap/extension-collaboration` | 2.27.2 | MIT |
| `@tiptap/extension-collaboration-cursor` | 2.26.2 | MIT |
| `@tiptap/extension-dropcursor` | 2.27.2 | MIT |
| `@tiptap/extension-gapcursor` | 2.27.2 | MIT |
| `@tiptap/extension-hard-break` | 2.27.2 | MIT |
| `@tiptap/extension-history` | 2.27.2 | MIT |
| `@tiptap/extension-horizontal-rule` | 2.27.2 | MIT |
| `@tiptap/extension-italic` | 2.27.2 | MIT |
| `@tiptap/extension-link` | 2.27.2 | MIT |
| `@tiptap/extension-paragraph` | 2.27.2 | MIT |
| `@tiptap/extension-strike` | 2.27.2 | MIT |
| `@tiptap/extension-table-cell` | 2.27.2 | MIT |
| `@tiptap/extension-table-header` | 2.27.2 | MIT |
| `@tiptap/extension-table-row` | 2.27.2 | MIT |
| `@tiptap/extension-text` | 2.27.2 | MIT |
| `@tiptap/extension-underline` | 2.27.2 | MIT |
| `@tiptap/pm` | 2.27.2 | MIT |
| `@types/extend` | 3.0.4 | MIT |
| `@types/hast` | 2.3.10 | MIT |
| `@types/mdast` | 3.0.15 | MIT |
| `@types/parse5` | 6.0.3 | MIT |
| `@types/prop-types` | 15.7.15 | MIT |
| `@types/react` | 18.3.31 | MIT |
| `@types/unist` | 3.0.3 | MIT |
| `ccount` | 2.0.1 | MIT |
| `character-entities-html4` | 2.1.0 | MIT |
| `character-entities-legacy` | 3.0.0 | MIT |
| `clsx` | 2.1.1 | MIT |
| `comma-separated-tokens` | 2.0.3 | MIT |
| `csstype` | 3.2.3 | MIT |
| `decode-named-character-reference` | 1.3.0 | MIT |
| `detect-node-es` | 1.1.0 | MIT |
| `emoji-mart` | 5.6.0 | MIT |
| `escape-string-regexp` | 5.0.0 | MIT |
| `extend` | 3.0.2 | MIT |
| `hast-util-embedded` | 3.0.0 | MIT |
| `hast-util-format` | 1.1.0 | MIT |
| `hast-util-from-dom` | 4.2.0 | ISC |
| `hast-util-from-parse5` | 7.1.2 | MIT |
| `hast-util-has-property` | 3.0.0 | MIT |
| `hast-util-is-body-ok-link` | 3.0.1 | MIT |
| `hast-util-is-element` | 3.0.0 | MIT |
| `hast-util-minify-whitespace` | 1.0.1 | MIT |
| `hast-util-parse-selector` | 3.1.1 | MIT |
| `hast-util-phrasing` | 3.0.1 | MIT |
| `hast-util-raw` | 7.2.3 | MIT |
| `hast-util-to-html` | 8.0.4 | MIT |
| `hast-util-to-mdast` | 8.4.1 | MIT |
| `hast-util-to-parse5` | 7.1.0 | MIT |
| `hast-util-to-text` | 3.1.2 | MIT |
| `hast-util-whitespace` | 3.0.0 | MIT |
| `hastscript` | 7.2.0 | MIT |
| `html-void-elements` | 2.0.1 | MIT |
| `html-whitespace-sensitive-tag-names` | 3.0.1 | MIT |
| `lib0` | 0.2.117 | MIT |
| `linkifyjs` | 4.3.3 | MIT |
| `markdown-table` | 3.0.4 | MIT |
| `mdast-util-definitions` | 5.1.2 | MIT |
| `mdast-util-find-and-replace` | 2.2.2 | MIT |
| `mdast-util-from-markdown` | 1.3.1 | MIT |
| `mdast-util-gfm` | 2.0.2 | MIT |
| `mdast-util-gfm-autolink-literal` | 1.0.3 | MIT |
| `mdast-util-gfm-footnote` | 1.0.2 | MIT |
| `mdast-util-gfm-strikethrough` | 1.0.3 | MIT |
| `mdast-util-gfm-table` | 1.0.7 | MIT |
| `mdast-util-gfm-task-list-item` | 1.0.2 | MIT |
| `mdast-util-phrasing` | 3.0.1 | MIT |
| `mdast-util-to-hast` | 12.3.0 | MIT |
| `mdast-util-to-markdown` | 1.5.0 | MIT |
| `mdast-util-to-string` | 3.2.0 | MIT |
| `micromark-core-commonmark` | 1.1.0 | MIT |
| `micromark-extension-gfm` | 2.0.3 | MIT |
| `micromark-extension-gfm-autolink-literal` | 1.0.5 | MIT |
| `micromark-extension-gfm-footnote` | 1.1.2 | MIT |
| `micromark-extension-gfm-strikethrough` | 1.0.7 | MIT |
| `micromark-extension-gfm-table` | 1.0.7 | MIT |
| `micromark-extension-gfm-tagfilter` | 1.0.2 | MIT |
| `micromark-extension-gfm-task-list-item` | 1.0.5 | MIT |
| `micromark-factory-destination` | 1.1.0 | MIT |
| `micromark-factory-label` | 1.1.0 | MIT |
| `micromark-factory-space` | 1.1.0 | MIT |
| `micromark-factory-title` | 1.1.0 | MIT |
| `micromark-factory-whitespace` | 1.1.0 | MIT |
| `micromark-util-character` | 1.2.0 | MIT |
| `micromark-util-chunked` | 1.1.0 | MIT |
| `micromark-util-classify-character` | 1.1.0 | MIT |
| `micromark-util-combine-extensions` | 1.1.0 | MIT |
| `micromark-util-html-tag-name` | 1.2.0 | MIT |
| `micromark-util-normalize-identifier` | 1.1.0 | MIT |
| `micromark-util-resolve-all` | 1.1.0 | MIT |
| `micromark-util-sanitize-uri` | 1.2.0 | MIT |
| `micromark-util-subtokenize` | 1.1.0 | MIT |
| `micromark-util-symbol` | 1.1.0 | MIT |
| `micromark-util-types` | 1.1.0 | MIT |
| `orderedmap` | 2.1.1 | MIT |
| `parse5` | 6.0.1 | MIT |
| `property-information` | 6.5.0 | MIT |
| `prosemirror-keymap` | 1.2.3 | MIT |
| `prosemirror-model` | 1.25.9 | MIT |
| `prosemirror-state` | 1.4.4 | MIT |
| `prosemirror-tables` | 1.8.5 | MIT |
| `prosemirror-transform` | 1.12.0 | MIT |
| `prosemirror-view` | 1.41.9 | MIT |
| `react` | 18.3.1 | MIT |
| `react-dom` | 18.3.1 | MIT |
| `react-icons` | 5.7.0 | MIT |
| `react-number-format` | 5.4.5 | MIT |
| `react-remove-scroll` | 2.7.2 | MIT |
| `react-remove-scroll-bar` | 2.3.8 | MIT |
| `react-router` | 6.30.4 | MIT |
| `react-router-dom` | 6.30.4 | MIT |
| `react-style-singleton` | 2.2.3 | MIT |
| `react-textarea-autosize` | 8.5.9 | MIT |
| `rehype-format` | 5.0.1 | MIT |
| `rehype-minify-whitespace` | 5.0.1 | MIT |
| `rehype-parse` | 8.0.5 | MIT |
| `rehype-remark` | 9.1.2 | MIT |
| `rehype-stringify` | 9.0.4 | MIT |
| `remark-gfm` | 3.0.1 | MIT |
| `remark-parse` | 10.0.2 | MIT |
| `remark-rehype` | 10.1.0 | MIT |
| `remark-stringify` | 10.0.3 | MIT |
| `space-separated-tokens` | 2.0.2 | MIT |
| `stringify-entities` | 4.0.4 | MIT |
| `trim-lines` | 3.0.1 | MIT |
| `trim-trailing-lines` | 2.1.0 | MIT |
| `tslib` | 2.8.1 | 0BSD |
| `type-fest` | 4.41.0 | (MIT OR CC0-1.0) |
| `unified` | 10.1.2 | MIT |
| `unist-util-find-after` | 4.0.1 | MIT |
| `unist-util-generated` | 2.0.1 | MIT |
| `unist-util-is` | 6.0.1 | MIT |
| `unist-util-position` | 4.0.4 | MIT |
| `unist-util-visit` | 4.1.2 | MIT |
| `unist-util-visit-parents` | 6.0.2 | MIT |
| `use-callback-ref` | 1.3.3 | MIT |
| `use-composed-ref` | 1.4.0 | MIT |
| `use-isomorphic-layout-effect` | 1.2.1 | MIT |
| `use-latest` | 1.3.0 | MIT |
| `use-sidecar` | 1.1.3 | MIT |
| `uuid` | 8.3.2 | MIT |
| `uvu` | 0.5.6 | MIT |
| `vfile` | 5.3.7 | MIT |
| `vfile-location` | 4.1.0 | MIT |
| `web-namespaces` | 2.0.1 | MIT |
| `y-prosemirror` | 1.2.12 | MIT |
| `y-protocols` | 1.0.7 | MIT |
| `yjs` | 13.6.31 | MIT |
| `zwitch` | 2.0.4 | MIT |
