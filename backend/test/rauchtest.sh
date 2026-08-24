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

# Als Funktion, weil der Papierkorb weiter unten einen zweiten Start mit einer
# anderen Frist braucht -- die Uhr räumt beim Hochfahren einmal auf, und genau
# das soll geprüft werden.
starte_dienst() {
    DATABASE_URL="postgres://nexora@127.0.0.1:${PGPORT}/nexora?sslmode=disable" \
    JWT_SECRET="rauchtest-geheimnis-lang-genug-fuer-hs256" \
    NEXORA_DATA_DIR="$ARBEIT/anhaenge" \
    PORT="$APIPORT" \
    NEXORA_CONFIG="/dev/null" \
    NEXORA_PAPIERKORB_TAGE="${1:-}" \
    "$ARBEIT/nexora" >> "$ARBEIT/dienst.log" 2>&1 &
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
    curl -fsS --max-time 2 "$BASIS/healthz" >/dev/null || {
        echo "Backend antwortet nicht" >&2; cat "$ARBEIT/dienst.log" >&2; exit 1; }
}

halte_dienst_an() {
    kill "$DIENST_PID" 2>/dev/null || true
    wait "$DIENST_PID" 2>/dev/null || true
    for i in $(seq 1 20); do
        curl -fsS --max-time 1 "$BASIS/healthz" >/dev/null 2>&1 || break
        sleep 0.3
    done
}

echo "== Backend starten"
starte_dienst

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

echo "== Einfuhr als eigene Ablage"
printf '# Aus dem Archiv\n\nInhalt.\n' > "$ARBEIT/ablage.md"
pruefe "Vorschau nennt die Ablage" "Umzug" \
       "$(curl -s -b "$KEKSE" -X POST "$BASIS/api/import" -F "file=@$ARBEIT/ablage.md" \
          -F "neueAblage=Umzug" -F "vorschau=1" | feld "['ablage']")"
pruefe "Vorschau legt keine Ablage an" "0" \
       "$(hole "$BASIS/api/spaces" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
ABL=$(curl -s -b "$KEKSE" -X POST "$BASIS/api/import" -F "file=@$ARBEIT/ablage.md" -F "neueAblage=Umzug")
pruefe "Ablage angelegt" "Umzug" "$(printf '%s' "$ABL" | feld "['ablage']['name']")"
pruefe "Ablage steht in der Liste" "1" \
       "$(hole "$BASIS/api/spaces" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
ABL_ID=$(printf '%s' "$ABL" | feld "['ablage']['id']")
pruefe "Seite liegt in der Ablage" "1" \
       "$(hole "$BASIS/api/pages" | ABL_ID="$ABL_ID" python3 -c '
import json, os, sys
ziel = os.environ["ABL_ID"]
print(sum(1 for p in json.load(sys.stdin) if p.get("spaceId") == ziel))')"
pruefe "beides zusammen wird abgewiesen" "400" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$KEKSE" -X POST "$BASIS/api/import" \
          -F "file=@$ARBEIT/ablage.md" -F "neueAblage=Zwei" -F "spaceId=$ABL_ID")"

echo "== Ausgabe"
pruefe "Markdown" "200" "$(code "$BASIS/api/pages/$SEITE/markdown")"

echo "== Papierkorb"
hole -X DELETE "$BASIS/api/pages/$SEITE" >/dev/null
pruefe "liegt im Papierkorb" "1" \
       "$(hole "$BASIS/api/pages/trash" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
pruefe "trägt ein Verfallsdatum" "True" \
       "$(hole "$BASIS/api/pages/trash" | python3 -c 'import json,sys;print(json.load(sys.stdin)[0]["verfaelltAm"] is not None)')"
pruefe "zurückgeholt" "200" "$(code -X POST "$BASIS/api/pages/$SEITE/restore")"
pruefe "Papierkorb wieder leer" "0" \
       "$(hole "$BASIS/api/pages/trash" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
hole -X DELETE "$BASIS/api/pages/$SEITE" >/dev/null
pruefe "endgültig entfernt" "200" "$(code -X DELETE "$BASIS/api/pages/$SEITE/purge")"
pruefe "danach nicht mehr auffindbar" "404" "$(code "$BASIS/api/pages/$SEITE")"

echo "== Papierkorb räumt sich selbst"
# Die Frist ist eine Zusage, die ohne Prüfung niemand nachrechnet: eine Seite
# in den Papierkorb legen, ihren Löschzeitpunkt um fünf Tage zurückdatieren
# und den Dienst mit der Frist 1 neu starten. Die Uhr räumt beim Hochfahren
# einmal auf -- danach muss die Seite fort sein.
#
# Zurückdatiert wird in der Wegwerf-Datenbank dieses Tests, nicht irgendwo
# sonst. Anders ließe sich ein Ablauf über Tage nicht in Sekunden prüfen.
#
# Die 0 wird absichtlich NICHT geprüft: sie heißt "nie von selbst", und die Uhr
# überspringt sie. Ein Test, der 0 als "sofort alles" liest, hätte die
# Bedeutung verdreht.
FRIST=$(hole -X POST "$BASIS/api/pages" -H 'Content-Type: application/json' \
        -d '{"title":"Verfaellt"}' | feld "['id']")
hole -X DELETE "$BASIS/api/pages/$FRIST" >/dev/null
pruefe "liegt im Papierkorb" "1" \
       "$(hole "$BASIS/api/pages/trash" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -q \
     -c "UPDATE pages SET deleted_at = now() - interval '5 days' WHERE deleted_at IS NOT NULL"
halte_dienst_an
starte_dienst 1
pruefe "von der Frist geräumt" "0" \
       "$(hole "$BASIS/api/pages/trash" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
pruefe "Räumung steht im Protokoll" "1" \
       "$(grep -c 'Papierkorb: 1 Seiten nach 1 Tag' "$ARBEIT/dienst.log" || true)"
pruefe "Frist steht in der Prüfspur" "1" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc \
          "SELECT count(*) FROM pruefspur WHERE akteur_name='Frist'")"

echo "== Sitzungen"
pruefe "eine Sitzung steht in der Liste" "1" \
       "$(hole "$BASIS/api/sitzungen" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
pruefe "sie ist als diese markiert" "True" \
       "$(hole "$BASIS/api/sitzungen" | python3 -c 'import json,sys;print(json.load(sys.stdin)[0]["diese"])')"
# Zweite Anmeldung von einem anderen "Geraet", eigene Keksdose.
curl -s -c "$ARBEIT/kekse2.txt" -X POST "$BASIS/api/auth/login" -H 'Content-Type: application/json' \
     -A "Mozilla/5.0 (Windows NT 10.0) Firefox/140.0" \
     -d '{"email":"rauch@test.invalid","password":"rauchtest-passwort"}' >/dev/null
pruefe "jetzt zwei Sitzungen" "2" \
       "$(hole "$BASIS/api/sitzungen" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
pruefe "das Geraet wird benannt" "Firefox auf Windows" \
       "$(hole "$BASIS/api/sitzungen" | python3 -c '
import json,sys
print(next(s["browser"] for s in json.load(sys.stdin) if not s["diese"]))')"
FREMD=$(hole "$BASIS/api/sitzungen" | python3 -c '
import json,sys
print(next(s["id"] for s in json.load(sys.stdin) if not s["diese"]))')
pruefe "fremde Sitzung beenden" "200" "$(code -X DELETE "$BASIS/api/sitzungen/$FREMD")"
# Das Token der zweiten Anmeldung muss sofort wertlos sein -- genau das konnte
# die alte, rein gerechnete Sitzung nicht.
pruefe "beendetes Token gilt nicht mehr" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$ARBEIT/kekse2.txt" "$BASIS/api/auth/me")"
pruefe "wieder nur eine Sitzung" "1" \
       "$(hole "$BASIS/api/sitzungen" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
# Abmelden widerruft ebenfalls -- frueher blieb das Token gueltig.
curl -s -c "$ARBEIT/kekse3.txt" -X POST "$BASIS/api/auth/login" -H 'Content-Type: application/json' \
     -d '{"email":"rauch@test.invalid","password":"rauchtest-passwort"}' >/dev/null
curl -s -b "$ARBEIT/kekse3.txt" -X POST "$BASIS/api/auth/logout" >/dev/null
pruefe "nach dem Abmelden gilt das Token nicht mehr" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$ARBEIT/kekse3.txt" "$BASIS/api/auth/me")"

echo "== SSO"
# Ohne Einrichtung und ohne Lizenz darf nichts angeboten werden -- ein Knopf,
# der danach mit 402 antwortet, waere ein Versprechen ohne Deckung.
pruefe "nichts angeboten, weil nichts eingerichtet" "False" \
       "$(hole "$BASIS/api/auth/sso" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d["oidc"] or d["ldap"])')"
pruefe "Passwort bleibt moeglich" "True" \
       "$(hole "$BASIS/api/auth/sso" | feld "['passwort']")"
pruefe "OIDC ohne Lizenz weist ab" "402" "$(code "$BASIS/api/auth/oidc/start")"
pruefe "LDAP ohne Lizenz weist ab" "402" \
       "$(code -X POST "$BASIS/api/auth/ldap" -H 'Content-Type: application/json' \
          -d '{"benutzer":"wer","passwort":"was"}')"

echo "== Lizenz"
# Ein eigenes Schlüsselpaar für den Test: der öffentliche Teil steckt fest im
# Programm, also lässt sich hier nicht mit einem echten Schlüssel prüfen -- und
# ein echter gehört ohnehin nicht in ein Verzeichnis. Geprüft wird deshalb, was
# ohne gültige Signatur passieren MUSS: Zurückweisung.
pruefe "unsinniger Schlüssel wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/system/lizenz" -H 'Content-Type: application/json' \
          -d '{"schluessel":"kein.schluessel"}')"
pruefe "Ausstellen ohne Signierschlüssel geht nicht" "501" \
       "$(code -X POST "$BASIS/api/system/lizenz/ausstellen" -H 'Content-Type: application/json' \
          -d '{"inhaber":"Wer auch immer","stufe":"pro"}')"
pruefe "Stufen stehen im Status" "4" \
       "$(hole "$BASIS/api/lizenz" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["stufen"]))')"
pruefe "Business enthält alles" "True" \
       "$(hole "$BASIS/api/lizenz" | python3 -c '
import json,sys
d = json.load(sys.stdin)
alle = set(d["alle_extras"])
business = next(s for s in d["stufen"] if s["name"] == "business")
print(set(business["funktionen"]) == alle)')"
pruefe "frei enthält nichts" "0" \
       "$(hole "$BASIS/api/lizenz" | python3 -c '
import json,sys
d = json.load(sys.stdin)
print(len(next(s for s in d["stufen"] if s["name"] == "free")["funktionen"]))')"
pruefe "leerer Schlüssel nimmt die Lizenz zurück" "200" \
       "$(code -X PUT "$BASIS/api/system/lizenz" -H 'Content-Type: application/json' -d '{"schluessel":""}')"

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
