# Third-party components

Nexora is licensed under BUSL-1.1 but is not built from its own source alone.
This file states what ships with it and under which terms.

Listed is what is **delivered**, not what the package files name. The
difference is not cosmetic: `go.mod` names 48 modules, 34 are linked in. The
difference is the Kerberos branch of `go-ldap`, which no call reaches and which
is therefore not compiled in; with it, the only MPL-licensed Go module stays
out. On the npm side 173 packages end up in the bundle, the rest in the
directory are build tools.

As of: 01.09.2026. This can be recounted at any time:

```bash
cd backend  && go list -deps -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}' ./... | sort -u
cd frontend && npm ls --omit=dev --all
```

## What comes with an obligation

Exactly one component of the source tree demands more than the notice, and one
of the two images does as well. Everything else is MIT, BSD, Apache-2.0 or ISC.

### BlockNote, the editor: MPL-2.0

`@blocknote/core`, `@blocknote/react` and `@blocknote/mantine`, each 0.15.11.

The Mozilla Public License 2.0 is **file-level** copyleft, not viral. Nexora's
own source is unaffected, and the product may be sold. Two things are required:

1. The licence notice has to accompany the **distributed form**. What is
   distributed is the built bundle, and the minifier throws comments away, the
   packages' licence headers included. That is why `frontend/vite.config.ts`
   sets a header of its own that stands in every generated bundle. If something
   is changed there, that is the spot where the obligation breaks without
   anybody noticing.
2. Changes to the MPL files themselves would have to stay under the MPL and be
   available. Nexora uses the packages unchanged from the registry; the source
   lives at <https://github.com/TypeCellOS/BlockNote>.

### The runtime image: GPL-2.0

`backend/Dockerfile` builds on Alpine and installs `poppler-utils` for
`pdftotext`, which extracts the full text of PDF attachments. The distributed
image therefore contains programs under the GPL:

| Package | Licence | why it is in there |
|---|---|---|
| poppler-utils, poppler | GPL-2.0-or-later | `pdftotext`, full text of PDF attachments |
| busybox, busybox-binsh, ssl_client | GPL-2.0-only | the image's shell |
| alpine-baselayout, apk-tools, scanelf | GPL-2.0-only | Alpine itself |
| libgcc, libstdc++ | GPL-2.0+ with runtime exception | system libraries |
| cairo | LGPL-2.1-or-later or MPL-1.1 | via poppler |
| freetype, zstd-libs | dual licence with a GPL option | via poppler |
| musl | MIT | the C library |

The frontend image contains no poppler, but it does contain busybox from the
same base.

Two questions have to be kept apart here.

**Does this make Nexora GPL?** No. `pdftotext` is called as a separate process
over a pipe, none of it is linked in. That is an independent invocation and not
a derived work; the Go program stays under BUSL-1.1.

**Whoever passes on the image distributes GPL programs along with it** and owes
the corresponding source for them. The offer for that is in
[LICENSING.md](LICENSING.md). Alpine publishes the sources of all packages at
<https://gitlab.alpinelinux.org/alpine/aports>. Both images additionally carry
this information as labels, so that it travels along even when only the image
is passed on.

What a built image really contains:

```bash
docker run --rm --entrypoint sh nexora-backend -lc \
  "apk list -I | sed -E 's/^([^ ]+).*\((.*)\).*/\2  \1/' | sort"
```

Whoever wants to avoid the GPL components removes `poppler-utils` from
`backend/Dockerfile`. The full text of PDF attachments is then lost; a pure Go
solution fails on font encodings, columns and embedded images in real PDF files.

## Go, linked modules (34)

| Distribution | |
|---|---|
| MIT | 17 |
| BSD-3-Clause | 8 |
| Apache-2.0 | 7 |
| BSD-2-Clause | 2 |

| Module | Version | Licence |
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

## npm, production tree (247)

| Distribution | |
|---|---|
| MIT | 235 |
| MPL-2.0 | 3 |
| 0BSD | 2 |
| Apache-2.0 | 1 |
| BSD-2-Clause | 1 |
| BSD-3-Clause | 1 |
| ISC | 1 |
| Python-2.0 | 1 |
| MIT and Zlib | 1 |
| MIT or CC0-1.0 | 1 |

Listing all 247 individually would mean maintaining a list that silently goes
stale at the next `npm install`. Named here are therefore the thirteen direct
dependencies and every package in the tree that is not MIT. The rest is the
usual foundation of React and ProseMirror and MIT throughout; it can be looked
up with the command above.

**Direct:**

| Package | Version | Licence |
|---|---|---|
| `@blocknote/core` | 0.15.11 | **MPL-2.0** |
| `@blocknote/mantine` | 0.15.11 | **MPL-2.0** |
| `@blocknote/react` | 0.15.11 | **MPL-2.0** |
| `@tiptap/core` | 2.27.2 | MIT |
| `@tiptap/pm` | 2.27.2 | MIT |
| `lib0` | 0.2.117 | MIT |
| `pdf-lib` | 1.17.1 | MIT |
| `pdfjs-dist` | 6.3.289 | **Apache-2.0** |
| `react` | 18.3.1 | MIT |
| `react-dom` | 18.3.1 | MIT |
| `react-router-dom` | 6.26.2 | MIT |
| `y-protocols` | 1.0.7 | MIT |
| `yjs` | 13.6.32 | MIT |

**Everything else in the tree that is not MIT:**

| Package | Version | Licence |
|---|---|---|
| `argparse` | 2.0.1 | Python-2.0 |
| `diff` | 5.2.2 | BSD-3-Clause |
| `entities` | 4.5.0 | BSD-2-Clause |
| `hast-util-from-dom` | 4.2.0 | ISC |
| `pako` | 1.0.11 | MIT and Zlib |
| `tslib` | 1.14.1, 2.8.1 | 0BSD |
| `type-fest` | 4.41.0 | MIT or CC0-1.0 |

`pdfjs-dist` is licensed under Apache-2.0 and is thus the first delivered
package besides BlockNote that demands more than a copyright notice: section 4
of the licence wants the licence text and a notice of changes. Nothing is
changed -- the library is bundled unchanged --, and the reference to this file
stands in the header of every bundle.

## Build only, not in the product

TypeScript (Apache-2.0), Vite (MIT), `@vitejs/plugin-react` (MIT) and their
foundation. One of them stands out when looking through and is named for that
reason, so that nobody takes it for a finding: `caniuse-lite` is licensed under
CC-BY-4.0. A build tool, it never gets into a delivered bundle.

On the Go side the same holds for `stretchr/testify`, `davecgh/go-spew` and
`pmezard/go-difflib`: test tools, all MIT or ISC, not in the binary.
