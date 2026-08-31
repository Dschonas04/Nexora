#!/usr/bin/env python3
"""Erzeugt THIRD-PARTY.md aus dem, was wirklich ausgeliefert wird.

Zwei Regeln bestimmen, was hier hineinkommt.

Erstens zaehlt, was im Erzeugnis landet, nicht was in den Dateien steht. Auf der
Go-Seite ist das nicht der Inhalt von go.mod, sondern das, was der Binder
tatsaechlich einbindet: go.mod nennt ueber die Abhaengigkeiten von go-ldap auch
Kerberos-Pakete, von denen keines mitkompiliert wird. Auf der npm-Seite ist es
nicht node_modules, sondern der Produktivbaum ohne Bauwerkzeuge; caniuse-lite
und argparse stehen auf der Platte, aber nie im Buendel.

Zweitens muss die Datei mitwachsen. Ein Hinweisdokument, das beim naechsten
Paket still veraltet, erfuellt seinen Zweck nicht mehr und ist schlimmer als
keines, weil es Sicherheit vortaeuscht. Deshalb laesst sich dieses Programm mit
--pruefen aufrufen: dann schreibt es nichts, sondern meldet mit Rueckgabewert 1,
dass die Datei nicht mehr zum Stand passt. Genau so haengt es in der CI.

Die Pakete des Laufzeit-Abbilds stehen NICHT hier drin, sondern von Hand in
THIRD-PARTY.md. Sie zu ermitteln braucht einen laufenden Container, und ein
Programm, das dafuer Docker anwirft, liefe in keiner der beiden CI-Reihen.
Nachsehen laesst sich der Stand mit dem Befehl, der dort im Abschnitt steht.
"""

import json
import os
import re
import subprocess
import sys

WURZEL = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ZIEL = os.path.join(WURZEL, "THIRD-PARTY.md")

# Erkennung an der Lizenzdatei. Reihenfolge zaehlt: "Apache License" steht auch
# im Kopf mancher Datei, die daneben etwas anderes ist, deshalb wird das
# eindeutigste Merkmal zuerst gesucht.
MUSTER = [
    ("MPL-2.0", r"Mozilla Public License Version 2\.0"),
    ("Apache-2.0", r"Apache License\s+Version 2\.0"),
    ("GPL-2.0", r"GNU GENERAL PUBLIC LICENSE\s+Version 2"),
    ("GPL-3.0", r"GNU GENERAL PUBLIC LICENSE\s+Version 3"),
    ("LGPL", r"GNU LESSER GENERAL PUBLIC LICENSE"),
    ("BSD-3-Clause", r"Neither the name of .{0,80}nor the names of its"),
    ("BSD-2-Clause", r"Redistribution and use in source and binary forms"),
    ("ISC", r"ISC License|Permission to use, copy, modify, and/or distribute"),
    ("MIT", r"Permission is hereby granted, free of charge"),
]

# Was ohne Weiteres verkauft werden darf: keine Pflicht ausser dem Hinweis.
UNBEDENKLICH = {"MIT", "ISC", "BSD-2-Clause", "BSD-3-Clause", "Apache-2.0", "0BSD",
                "BlueOak-1.0.0", "Unlicense", "CC0-1.0", "Python-2.0", "CC-BY-4.0"}
# Auflagen, aber mit Verkauf vereinbar. Steht im Bericht eigens heraus.
AUFLAGEN = {"MPL-2.0", "LGPL"}


def erkenne(pfad):
    try:
        with open(pfad, encoding="utf-8", errors="replace") as f:
            text = f.read(6000)
    except OSError:
        return "?"
    for name, muster in MUSTER:
        if re.search(muster, text, re.I | re.S):
            return name
    return "?"


def lizenzdatei(verzeichnis):
    try:
        for eintrag in sorted(os.listdir(verzeichnis)):
            if re.match(r"^(LICEN[SC]E|COPYING)", eintrag, re.I):
                return os.path.join(verzeichnis, eintrag)
    except OSError:
        pass
    return None


def go_module():
    """Die Module, die wirklich mitkompiliert werden."""
    backend = os.path.join(WURZEL, "backend")
    roh = subprocess.run(
        ["go", "list", "-deps", "-f",
         "{{if .Module}}{{.Module.Path}}\t{{.Module.Version}}\t{{.Module.Dir}}{{end}}",
         "./..."],
        cwd=backend, capture_output=True, text=True, check=True).stdout

    gesehen = {}
    for zeile in roh.splitlines():
        teile = zeile.split("\t")
        if len(teile) != 3 or not teile[0]:
            continue
        pfad, fassung, verzeichnis = teile
        # Das eigene Modul gehoert nicht in eine Liste fremder Lizenzen.
        if pfad == "nexora" or not fassung:
            continue
        gesehen[pfad] = (fassung, verzeichnis)

    ergebnis = []
    for pfad, (fassung, verzeichnis) in sorted(gesehen.items()):
        datei = lizenzdatei(verzeichnis) if verzeichnis else None
        ergebnis.append((pfad, fassung, erkenne(datei) if datei else "?"))
    return ergebnis


def npm_pakete():
    """Der Produktivbaum, also das, was im Buendel landet."""
    frontend = os.path.join(WURZEL, "frontend")
    roh = subprocess.run(["npm", "ls", "--omit=dev", "--all", "--json"],
                         cwd=frontend, capture_output=True, text=True).stdout
    baum = json.loads(roh)

    namen = {}

    def geh(knoten):
        for name, wert in (knoten.get("dependencies") or {}).items():
            if name in namen:
                continue
            namen[name] = wert.get("version", "?")
            geh(wert)

    geh(baum)

    ergebnis = []
    for name in sorted(namen):
        pfad = os.path.join(frontend, "node_modules", *name.split("/"))
        lizenz = "?"
        try:
            with open(os.path.join(pfad, "package.json"), encoding="utf-8") as f:
                d = json.load(f)
            wert = d.get("license") or d.get("licenses") or "?"
            if isinstance(wert, list):
                wert = " OR ".join(
                    x.get("type", "?") if isinstance(x, dict) else str(x) for x in wert)
            if isinstance(wert, dict):
                wert = wert.get("type", "?")
            lizenz = str(wert)
        except (OSError, ValueError):
            pass
        # Der Feldwert ist eine Behauptung des Pakets. Wo eine Lizenzdatei
        # daneben liegt, wird sie gelesen; weichen beide ab, faellt das auf.
        datei = lizenzdatei(pfad)
        gelesen = erkenne(datei) if datei else "?"
        ergebnis.append((name, namen[name], lizenz, gelesen))
    return ergebnis


def zaehlen(liste, stelle):
    zahlen = {}
    for eintrag in liste:
        zahlen[eintrag[stelle]] = zahlen.get(eintrag[stelle], 0) + 1
    return sorted(zahlen.items(), key=lambda x: (-x[1], x[0]))


def bauen():
    go = go_module()
    npm = npm_pakete()

    z = []
    z.append("# Fremde Bestandteile\n")
    z.append("**Diese Datei wird erzeugt. Nicht von Hand ändern**, sondern\n"
             "`python3 scripts/lizenzen.py` aufrufen. Die CI prüft bei jedem Lauf, dass sie\n"
             "zum Stand der Abhängigkeiten passt.\n")
    z.append("Aufgeführt ist, was **ausgeliefert** wird: auf der Go-Seite die Module, die der\n"
             "Binder wirklich einbindet, auf der npm-Seite der Produktivbaum ohne\n"
             "Bauwerkzeuge. Was nur auf der Platte liegt, aber nie in ein Erzeugnis gerät,\n"
             "steht nicht hier.\n")

    # ── Auflagen zuerst ──────────────────────────────────────────────────
    besonders = [e for e in npm if e[2] in AUFLAGEN or e[3] in AUFLAGEN]
    besonders += [(e[0], e[1], e[2], e[2]) for e in go if e[2] in AUFLAGEN]

    z.append("\n## Womit eine Auflage verbunden ist\n")
    if not besonders:
        z.append("Nichts. Alle ausgelieferten Bestandteile stehen unter Lizenzen, die außer\n"
                 "dem Hinweis nichts verlangen.\n")
    else:
        z.append("Alles andere unten ist MIT, BSD, Apache-2.0 oder ISC und verlangt außer dem\n"
                 "Hinweis nichts. Die folgenden Bestandteile verlangen mehr:\n")
        z.append("\n| Bestandteil | Fassung | Lizenz |\n|---|---|---|")
        for name, fassung, lizenz, _ in sorted(set(besonders)):
            z.append(f"| `{name}` | {fassung} | **{lizenz}** |")
        z.append("")
        z.append("**MPL-2.0** ist dateiweises Copyleft. Der eigene Quelltext bleibt davon\n"
                 "unberührt, und das Erzeugnis darf verkauft werden. Verlangt ist zweierlei:\n"
                 "der Lizenzhinweis muss die verteilte Form begleiten, und Änderungen an den\n"
                 "MPL-Dateien selbst müssen unter MPL bleiben und verfügbar sein. Nexora\n"
                 "benutzt diese Pakete unverändert aus der Registry; der Quelltext liegt\n"
                 "unter <https://github.com/TypeCellOS/BlockNote>.\n")

    # ── Laufzeit-Abbild ──────────────────────────────────────────────────
    z.append("\n## Das Laufzeit-Abbild\n")
    z.append("Der wichtigste Teil steht in keiner Paketdatei. `backend/Dockerfile` setzt auf\n"
             "Alpine auf und installiert `poppler-utils` für `pdftotext`. Damit enthält das\n"
             "verteilte Abbild Programme unter **GPL-2.0**:\n")
    z.append("\n| Paket | Lizenz | warum |\n|---|---|---|")
    for zeile in [
        ("poppler-utils, poppler", "GPL-2.0-or-later", "pdftotext, Volltext aus PDF-Anhängen"),
        ("busybox, busybox-binsh, ssl_client", "GPL-2.0-only", "die Shell des Abbilds"),
        ("alpine-baselayout, apk-tools, scanelf", "GPL-2.0-only", "Alpine selbst"),
        ("libgcc, libstdc++", "GPL-2.0+ mit Laufzeitausnahme", "Systembibliotheken"),
        ("cairo", "LGPL-2.1-or-later oder MPL-1.1", "über poppler"),
        ("freetype, zstd-libs", "Doppellizenz mit GPL-Möglichkeit", "über poppler"),
        ("musl", "MIT", "die C-Bibliothek"),
    ]:
        z.append("| " + " | ".join(zeile) + " |")
    z.append("")
    z.append("Zwei Fragen sind dabei auseinanderzuhalten.\n")
    z.append("**Wird Nexora dadurch GPL?** Nein. `pdftotext` wird als eigener Prozess\n"
             "aufgerufen, über eine Pipe, ohne Bindung. Das ist ein unabhängiger Aufruf und\n"
             "kein abgeleitetes Werk; das Go-Programm bleibt unter seiner eigenen Lizenz.\n")
    z.append("**Wer das Abbild weitergibt, verteilt GPL-Programme** und schuldet dafür den\n"
             "zugehörigen Quelltext. Das Angebot dazu steht in [LICENSING.md](LICENSING.md).\n"
             "Alpine veröffentlicht die Quellen aller Pakete unter\n"
             "<https://gitlab.alpinelinux.org/alpine/aports>.\n")
    z.append("Nachsehen, was ein gebautes Abbild wirklich enthält:\n")
    z.append("```bash\ndocker run --rm --entrypoint sh nexora-backend -lc \\\n"
             "  \"apk list -I | sed -E 's/^([^ ]+).*\\\\((.*)\\\\).*/\\\\2  \\\\1/' | sort\"\n```\n")

    # ── Listen ───────────────────────────────────────────────────────────
    z.append(f"\n## Go, eingebundene Module ({len(go)})\n")
    z.append("| Verteilung | |\n|---|---|")
    for lizenz, anzahl in zaehlen(go, 2):
        z.append(f"| {lizenz} | {anzahl} |")
    z.append("\n| Modul | Fassung | Lizenz |\n|---|---|---|")
    for pfad, fassung, lizenz in go:
        z.append(f"| `{pfad}` | {fassung} | {lizenz} |")

    z.append(f"\n## npm, Produktivbaum ({len(npm)})\n")
    z.append("| Verteilung | |\n|---|---|")
    for lizenz, anzahl in zaehlen(npm, 2):
        z.append(f"| {lizenz} | {anzahl} |")
    z.append("\n| Paket | Fassung | Lizenz |\n|---|---|---|")
    for name, fassung, lizenz, _ in npm:
        z.append(f"| `{name}` | {fassung} | {lizenz} |")

    unklar = [e[0] for e in go if e[2] == "?"] + [e[0] for e in npm if e[2] == "?"]
    if unklar:
        z.append("\n## Ungeklärt\n")
        z.append("Für diese Bestandteile war keine Lizenz zu ermitteln. Das ist ein Befund,\n"
                 "kein Schönheitsfehler: ungeklärt heißt nicht frei.\n")
        for name in sorted(unklar):
            z.append(f"- `{name}`")

    return "\n".join(z).rstrip() + "\n"


def main():
    inhalt = bauen()
    if "--pruefen" in sys.argv:
        alt = ""
        if os.path.exists(ZIEL):
            with open(ZIEL, encoding="utf-8") as f:
                alt = f.read()
        if alt != inhalt:
            print("THIRD-PARTY.md passt nicht mehr zum Stand der Abhängigkeiten.",
                  file=sys.stderr)
            print("Erneuern mit: python3 scripts/lizenzen.py", file=sys.stderr)
            return 1
        print("THIRD-PARTY.md ist aktuell.")
        return 0
    with open(ZIEL, "w", encoding="utf-8") as f:
        f.write(inhalt)
    print(f"THIRD-PARTY.md geschrieben.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
