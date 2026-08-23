#!/usr/bin/env bash
# Rauchtest: das gebaute Backend gegen eine Wegwerf-Datenbank.
#
# Die Prüfungen mit "go test" fassen keine Datenbank an, und genau dort sitzen
# die Fehler, die man nicht sieht: eine Abfrage, die sich nicht übersetzen
# lässt, fällt erst auf, wenn sie zum ersten Mal läuft. Ein Beispiel aus der
# Wirklichkeit war ($1 || ' days')::interval -- alle Prüfungen grün, und der
# Papierkorb räumte trotzdem nie auf.
#
# Deshalb hier: echte Postgres-Instanz, echtes Programm, echte Aufrufe über
# HTTP. Alles in einem Wegwerf-Verzeichnis, das am Ende verschwindet -- der
# Test darf nichts hinterlassen, auch wenn er scheitert.
set -euo pipefail

WURZEL="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARBEIT="$(mktemp -d)"
PGPORT="${PGPORT:-55432}"
APIPORT="${APIPORT:-58080}"
BASIS="http://127.0.0.1:${APIPORT}"
KEKSE="$ARBEIT/kekse.txt"

fehler=0

aufraeumen() {
    set +e
    [ -n "${DIENST_PID:-}" ] && kill "$DIENST_PID" 2>/dev/null
    pg_ctl -D "$ARBEIT/db" -m immediate stop >/dev/null 2>&1
    rm -rf "$ARBEIT"
}
trap aufraeumen EXIT

melde() {
    # Ein fehlgeschlagener Schritt bricht nicht sofort ab: es ist mehr wert zu
    # wissen, welche fünf von zwanzig Prüfungen fallen, als nur die erste.
    if [ "$1" = "ok" ]; then
        printf '  ok    %s\n' "$2"
    else
        printf '  FEHLT %s\n' "$2"
        fehler=$((fehler + 1))
    fi
}

pruefe() {
    # pruefe "Beschreibung" "erwartet" "bekommen"
    if [ "$2" = "$3" ]; then melde ok "$1"; else melde fehler "$1 (erwartet $2, bekam $3)"; fi
}

echo "== Datenbank anwerfen"
initdb -D "$ARBEIT/db" -U nexora --auth=trust --encoding=UTF8 --locale=C >/dev/null
pg_ctl -D "$ARBEIT/db" -l "$ARBEIT/pg.log" \
       -o "-p $PGPORT -k $ARBEIT -h 127.0.0.1" -w start >/dev/null
createdb -h 127.0.0.1 -p "$PGPORT" -U nexora nexora

echo "== Backend bauen"
( cd "$WURZEL" && go build -o "$ARBEIT/nexora" . )

echo "== Backend starten"
DATABASE_URL="postgres://nexora@127.0.0.1:${PGPORT}/nexora?sslmode=disable" \
JWT_SECRET="rauchtest-geheimnis-lang-genug-fuer-hs256" \
NEXORA_DATA_DIR="$ARBEIT/anhaenge" \
PORT="$APIPORT" \
NEXORA_CONFIG="/dev/null" \
"$ARBEIT/nexora" > "$ARBEIT/dienst.log" 2>&1 &
DIENST_PID=$!

for i in $(seq 1 40); do
    sleep 0.5
    if curl -fsS --max-time 2 "$BASIS/healthz" >/dev/null 2>&1; then break; fi
    if ! kill -0 "$DIENST_PID" 2>/dev/null; then
        echo "Das Backend ist beim Start ausgestiegen:" >&2
        cat "$ARBEIT/dienst.log" >&2
        exit 1
    fi
done
curl -fsS --max-time 2 "$BASIS/healthz" >/dev/null || { echo "Backend antwortet nicht" >&2; cat "$ARBEIT/dienst.log" >&2; exit 1; }

code() { curl -s -o /dev/null -w '%{http_code}' -b "$KEKSE" "$@"; }
hole() { curl -s -b "$KEKSE" "$@"; }
feld() { python3 -c "import json,sys;d=json.load(sys.stdin);print(d$1)"; }

echo "== Konto und Seite"
curl -s -c "$KEKSE" -X POST "$BASIS/api/auth/register" -H 'Content-Type: application/json' \
     -d '{"email":"rauch@test.invalid","name":"Rauch Test","password":"rauchtest-passwort"}' >/dev/null
pruefe "angemeldet" "200" "$(code "$BASIS/api/auth/me")"

SEITE=$(hole -X POST "$BASIS/api/pages" -H 'Content-Type: application/json' \
        -d '{"title":"Rauchprobe"}' | feld "['id']")
[ -n "$SEITE" ] || { echo "keine Seite angelegt" >&2; exit 1; }
hole -X PUT "$BASIS/api/pages/$SEITE" -H 'Content-Type: application/json' \
     -d '{"content":[{"type":"paragraph","content":[{"type":"text","text":"Der Ofen läuft."}]}]}' >/dev/null

echo "== Suche"
pruefe "Volltext findet die Seite" "Rauchprobe" \
       "$(hole "$BASIS/api/search?q=Ofen" | feld "[0]['title']")"
pruefe "Filter nach Alter" "1" \
       "$(hole "$BASIS/api/search?q=Ofen&tage=7" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
pruefe "Filter nach Ablage ohne" "1" \
       "$(hole "$BASIS/api/search?q=Ofen&space=ohne" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
pruefe "kaputte Kennung wird abgewiesen" "400" "$(code "$BASIS/api/search?q=Ofen&space=keine-uuid")"

echo "== Einfuhr"
printf '# Eingefuehrt\n\nEin Absatz.\n' > "$ARBEIT/probe.md"
EIN=$(curl -s -b "$KEKSE" -X POST "$BASIS/api/import" -F "file=@$ARBEIT/probe.md")
pruefe "eine Seite eingeführt" "1" "$(printf '%s' "$EIN" | feld "['seiten']")"
pruefe "Vorschau legt nichts an" "1" \
       "$(curl -s -b "$KEKSE" -X POST "$BASIS/api/import" -F "file=@$ARBEIT/probe.md" -F "vorschau=1" | feld "['seiten']")"

echo "== Ausgabe"
pruefe "Markdown" "200" "$(code "$BASIS/api/pages/$SEITE/markdown")"

echo "== Papierkorb"
hole -X DELETE "$BASIS/api/pages/$SEITE" >/dev/null
pruefe "liegt im Papierkorb" "1" \
       "$(hole "$BASIS/api/pages/trash" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
pruefe "trägt ein Verfallsdatum" "True" \
       "$(hole "$BASIS/api/pages/trash" | python3 -c 'import json,sys;print(json.load(sys.stdin)[0]["verfaelltAm"] is not None)')"

echo "== Postfach"
pruefe "Postfach antwortet" "200" "$(code "$BASIS/api/postfach")"
pruefe "Zähler antwortet" "0" "$(hole "$BASIS/api/postfach/anzahl" | feld "['ungelesen']")"

echo
if [ "$fehler" -gt 0 ]; then
    echo "$fehler Prüfungen sind gefallen." >&2
    echo "--- Protokoll des Dienstes ---" >&2
    tail -40 "$ARBEIT/dienst.log" >&2
    exit 1
fi
echo "Rauchtest bestanden."
