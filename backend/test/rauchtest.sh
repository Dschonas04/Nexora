#!/usr/bin/env bash
# Smoke test: the built backend against a throwaway database.
#
# The checks run by "go test" touch no database, and that is exactly where the
# errors sit that one does not see: a query that cannot be parsed only shows up
# when it runs for the first time. A real world example was
# ($1 || ' days')::interval, with every check green and the trash never clearing
# itself all the same.
#
# Hence this: a real Postgres instance, the real program, real calls over HTTP.
# All of it in a throwaway directory that disappears at the end; the test must
# leave nothing behind, even when it fails.
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
    # A failed step does not abort right away: knowing which five of twenty
    # checks fall is worth more than knowing only the first.
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

# As a function, because the trash further down needs a second start with a
# different deadline; the sweeper clears once while coming up, and that is
# exactly what is to be checked.
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
# The deadline is a promise nobody recalculates without a check: put a page into
# the trash, back date its deletion time by five days and restart the service
# with the deadline set to 1. The sweeper clears once while coming up, and
# afterwards the page has to be gone.
#
# The back dating happens in this test's throwaway database, nowhere else.
# Otherwise an expiry spanning days could not be checked in seconds.
#
# The 0 is deliberately NOT checked: it means "never by itself", and the sweeper
# skips it. A test reading 0 as "everything at once" would have twisted the
# meaning.
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
pruefe "Frist steht in Stunden" "True" \
       "$(hole "$BASIS/api/sitzungen" | python3 -c '
import json, sys, datetime
s = json.load(sys.stdin)[0]
ab = datetime.datetime.fromisoformat(s["angelegtAm"].replace("Z", "+00:00"))
bis = datetime.datetime.fromisoformat(s["laeuftAb"].replace("Z", "+00:00"))
stunden = (bis - ab).total_seconds() / 3600
# Vorgabe sind zwoelf Stunden. Frueher waren es sieben Tage, also 168.
print(11.9 < stunden < 12.1)')"
pruefe "das Geraet wird benannt" "Firefox auf Windows" \
       "$(hole "$BASIS/api/sitzungen" | python3 -c '
import json,sys
print(next(s["browser"] for s in json.load(sys.stdin) if not s["diese"]))')"
FREMD=$(hole "$BASIS/api/sitzungen" | python3 -c '
import json,sys
print(next(s["id"] for s in json.load(sys.stdin) if not s["diese"]))')
pruefe "fremde Sitzung beenden" "200" "$(code -X DELETE "$BASIS/api/sitzungen/$FREMD")"
# Das Token der zweiten Anmeldung muss sofort wertlos sein, genau das konnte
# die alte, rein gerechnete Sitzung nicht.
pruefe "beendetes Token gilt nicht mehr" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$ARBEIT/kekse2.txt" "$BASIS/api/auth/me")"
pruefe "wieder nur eine Sitzung" "1" \
       "$(hole "$BASIS/api/sitzungen" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"
# Abmelden widerruft ebenfalls, frueher blieb das Token gueltig.
curl -s -c "$ARBEIT/kekse3.txt" -X POST "$BASIS/api/auth/login" -H 'Content-Type: application/json' \
     -d '{"email":"rauch@test.invalid","password":"rauchtest-passwort"}' >/dev/null
curl -s -b "$ARBEIT/kekse3.txt" -X POST "$BASIS/api/auth/logout" >/dev/null
pruefe "nach dem Abmelden gilt das Token nicht mehr" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$ARBEIT/kekse3.txt" "$BASIS/api/auth/me")"

echo "== SSO"
# Ohne Einrichtung und ohne Lizenz darf nichts angeboten werden, ein Knopf,
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
# A key pair of its own for the test: the public half sits fixed in the program,
# so nothing can be checked here with a real key, and a real one does not belong
# in a repository anyway. What is checked is therefore what MUST happen without a
# valid signature: rejection.
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

echo "== Umhängen und Reihenfolge"
# Three pages at the top level, in a known order. titel() reads the sidebar
# order back the way the interface sees it: the list as the API returns it.
# Only the three of them: pages from the earlier steps are lying around at the
# top level too, and their place is none of this section's business.
titel() { hole "$BASIS/api/pages" | python3 -c '
import json, sys
unsere = {"Eins", "Zwei", "Drei"}
print(",".join(p["title"] for p in json.load(sys.stdin)
               if p["parentId"] is None and p["spaceId"] is None and p["title"] in unsere))'; }
neue_seite() {
    hole -X POST "$BASIS/api/pages" -H 'Content-Type: application/json' \
         -d "{\"title\":\"$1\"}" | feld "['id']"
}
EINS=$(neue_seite Eins)
ZWEI=$(neue_seite Zwei)
DREI=$(neue_seite Drei)
pruefe "Ausgangsfolge" "Eins,Zwei,Drei" "$(titel)"

pruefe "Drei vor Eins" "200" \
       "$(code -X PUT "$BASIS/api/pages/$DREI/reihenfolge" -H 'Content-Type: application/json' \
          -d "{\"vorId\":\"$EINS\"}")"
pruefe "steht jetzt vorn" "Drei,Eins,Zwei" "$(titel)"

pruefe "Eins ans Ende" "200" \
       "$(code -X PUT "$BASIS/api/pages/$EINS/reihenfolge" -H 'Content-Type: application/json' \
          -d '{"vorId":null}')"
pruefe "steht jetzt hinten" "Drei,Zwei,Eins" "$(titel)"

# Hanging one page under another: it has to leave the top level and turn up as a
# child, and the order of what stays behind must survive it.
pruefe "Zwei unter Drei" "200" \
       "$(code -X PUT "$BASIS/api/pages/$ZWEI/reihenfolge" -H 'Content-Type: application/json' \
          -d "{\"elternId\":\"$DREI\"}")"
pruefe "oben nur noch zwei" "Drei,Eins" "$(titel)"
pruefe "hängt unter Drei" "$DREI" \
       "$(hole "$BASIS/api/pages/$ZWEI" | feld "['parentId']")"

# The guard against a page landing inside its own subtree. Without it the branch
# would hang below itself and be reachable from nowhere.
pruefe "Drei unter die eigene Unterseite wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/pages/$DREI/reihenfolge" -H 'Content-Type: application/json' \
          -d "{\"elternId\":\"$ZWEI\"}")"
pruefe "unter sich selbst wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/pages/$DREI/reihenfolge" -H 'Content-Type: application/json' \
          -d "{\"elternId\":\"$DREI\"}")"

# A subpage follows its parent into the parent's space, so the two cannot drift
# into different sections of the sidebar.
pruefe "Drei in die Ablage" "200" \
       "$(code -X PUT "$BASIS/api/pages/$DREI/reihenfolge" -H 'Content-Type: application/json' \
          -d "{\"elternId\":null,\"spaceId\":\"$ABL_ID\"}")"
pruefe "die Unterseite zieht mit" "$ABL_ID" \
       "$(hole "$BASIS/api/pages/$ZWEI" | feld "['spaceId']")"

echo "== Reihenfolge der Ablagen"
ZWEITE=$(hole -X POST "$BASIS/api/spaces" -H 'Content-Type: application/json' \
         -d '{"name":"Zweite Ablage"}' | feld "['id']")
ablagen() { hole "$BASIS/api/spaces" | python3 -c '
import json, sys
print(",".join(a["name"] for a in json.load(sys.stdin)))'; }
pruefe "nach Namen sortiert" "Umzug,Zweite Ablage" "$(ablagen)"
pruefe "Reihenfolge gesetzt" "204" \
       "$(code -X PUT "$BASIS/api/spaces/reihenfolge" -H 'Content-Type: application/json' \
          -d "{\"ids\":[\"$ZWEITE\",\"$ABL_ID\"]}")"
pruefe "steht jetzt so da" "Zweite Ablage,Umzug" "$(ablagen)"
pruefe "leere Liste wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/spaces/reihenfolge" -H 'Content-Type: application/json' \
          -d '{"ids":[]}')"

echo "== Satzspiegel einer Seite"
BREIT=$(hole -X POST "$BASIS/api/pages" -H 'Content-Type: application/json' \
        -d '{"title":"Breite Seite"}' | feld "['id']")
# Leer heisst: keine eigene Wahl, es gilt die Vorgabe der Instanz.
pruefe "steht anfangs auf der Vorgabe" "" "$(hole "$BASIS/api/pages/$BREIT" | feld "['breite']")"
pruefe "und die ist volle Breite" "voll" \
       "$(hole "$BASIS/api/design" | feld "['seitenbreite']")"
pruefe "auf breit gesetzt" "200" \
       "$(code -X PUT "$BASIS/api/pages/$BREIT/breite" -H 'Content-Type: application/json' \
          -d '{"breite":"breit"}')"
pruefe "steht jetzt auf breit" "breit" "$(hole "$BASIS/api/pages/$BREIT" | feld "['breite']")"
pruefe "Unsinn wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/pages/$BREIT/breite" -H 'Content-Type: application/json' \
          -d '{"breite":"riesig"}')"
pruefe "zurueck auf die Vorgabe geht auch" "" \
       "$(hole -X PUT "$BASIS/api/pages/$BREIT/breite" -H 'Content-Type: application/json' \
          -d '{"breite":""}' | feld "['breite']")"

echo "== Anmeldeversuche"
# Die Auswertung rechnet mit Intervallen aus einer Zahl und mit FILTER-Zählungen.
# Beides fällt erst auf, wenn es wirklich gegen Postgres läuft, siehe der Kopf
# dieser Datei. Ein Fehlversuch wird deshalb hier von Hand ausgelöst.
curl -s -o /dev/null -X POST "$BASIS/api/auth/login" -H 'Content-Type: application/json' \
     -A "Mozilla/5.0 (X11; Linux x86_64) Firefox/141.0" \
     -d '{"kennung":"rauch@test.invalid","password":"falsch"}'
curl -s -o /dev/null -X POST "$BASIS/api/auth/login" -H 'Content-Type: application/json' \
     -d '{"kennung":"gibtesnicht@test.invalid","password":"egal"}'
pruefe "Auswertung antwortet" "200" "$(code "$BASIS/api/system/anmeldungen")"
pruefe "beide Fehlversuche stehen da" "2" \
       "$(hole "$BASIS/api/system/anmeldungen?nur=fehl" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["versuche"]))')"
pruefe "falsches Passwort wird als solches vermerkt" "Passwort falsch" \
       "$(hole "$BASIS/api/system/anmeldungen?nur=fehl" | python3 -c '
import json, sys
d = json.load(sys.stdin)["versuche"]
print(next(v["grund"] for v in d if v["kennung"] == "rauch@test.invalid"))')"
pruefe "unbekannte Kennung ebenso" "Kennung unbekannt" \
       "$(hole "$BASIS/api/system/anmeldungen?nur=fehl" | python3 -c '
import json, sys
d = json.load(sys.stdin)["versuche"]
print(next(v["grund"] for v in d if v["kennung"] == "gibtesnicht@test.invalid"))')"
pruefe "der Weg steht dabei" "passwort" \
       "$(hole "$BASIS/api/system/anmeldungen?nur=fehl" | feld "['versuche'][0]['weg']")"
pruefe "die Adresse steht dabei" "127.0.0.1" \
       "$(hole "$BASIS/api/system/anmeldungen?nur=fehl" | feld "['versuche'][0]['ip']")"
pruefe "der Browser steht dabei" "True" \
       "$(hole "$BASIS/api/system/anmeldungen?nur=fehl" | python3 -c '
import json, sys
d = json.load(sys.stdin)["versuche"]
print(any("Firefox" in v["browser"] for v in d))')"
pruefe "gelungene Anmeldungen sind auch verzeichnet" "True" \
       "$(hole "$BASIS/api/system/anmeldungen?nur=erfolg" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["versuche"]) > 0)')"
pruefe "Herkunft fasst die Adresse zusammen" "127.0.0.1" \
       "$(hole "$BASIS/api/system/anmeldungen" | feld "['herkunft'][0]['ip']")"
pruefe "die Zusammenfassung zählt die Fehlversuche" "2" \
       "$(hole "$BASIS/api/system/anmeldungen" | feld "['zusammenfassung']['fehl24h']")"
# tage=0 heißt "alles" und wird intern zu einer sehr großen Zahl. Genau daran
# ist die Intervall-Rechnung schon einmal gescheitert.
pruefe "tage=0 liefert alles" "200" "$(code "$BASIS/api/system/anmeldungen?tage=0")"
pruefe "Filter nach Adresse greift" "0" \
       "$(hole "$BASIS/api/system/anmeldungen?ip=10.9.9.9" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["versuche"]))')"

echo "== Puls"
# Gezaehlt wird ohne Sperre auf dem heissen Weg, und die Faecher werden ueber
# die Uhr gewechselt. Ob das im laufenden Dienst wirklich zusammenpasst, zeigt
# sich erst hier: die Einheitspruefungen sehen nur das Paket, nicht die Kette.
pruefe "Puls antwortet" "200" "$(code "$BASIS/api/system/puls")"
pruefe "der Vorrat nennt seine Obergrenze" "True" \
       "$(hole "$BASIS/api/system/puls" | python3 -c 'import json,sys;print(json.load(sys.stdin)["vorrat"]["hoechstens"] > 0)')"
# Nicht die Zahl der Zugriffe ohne freie Verbindung: die steht auch auf einer
# unbelasteten Instanz ueber null, weil der Vorrat beim Start leer ist und die
# ersten Zugriffe ihre Verbindung erst aufbauen lassen. Aussagekraeftig ist die
# mittlere Wartezeit, und die muss hier verschwindend sein.
pruefe "kaum Wartezeit auf eine Verbindung" "True" \
       "$(hole "$BASIS/api/system/puls" | python3 -c 'import json,sys;print(json.load(sys.stdin)["vorrat"]["mittelWarteMs"] < 1.0)')"
pruefe "die Minute hat 59 Faecher" "59" \
       "$(hole "$BASIS/api/system/puls" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["anfragen"]["minute"]))')"
# Die vielen Aufrufe der Abschnitte davor muessen sich niedergeschlagen haben.
pruefe "es wurde etwas gezaehlt" "True" \
       "$(hole "$BASIS/api/system/puls" | python3 -c 'import json,sys;print(json.load(sys.stdin)["anfragen"]["gesamt"] > 50)')"
# Der Abfrageweg zaehlt sich nicht selbst mit, sonst stuende er als
# Grundrauschen in jeder Messung, die er anzeigen soll.
VORHER=$(hole "$BASIS/api/system/puls" | feld "['anfragen']['gesamt']")
hole "$BASIS/api/system/puls" >/dev/null
hole "$BASIS/api/system/puls" >/dev/null
pruefe "der Puls zaehlt sich nicht selbst" "$VORHER" \
       "$(hole "$BASIS/api/system/puls" | feld "['anfragen']['gesamt']")"
pruefe "ohne Anmeldung verschlossen" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' "$BASIS/api/system/puls")"

echo "== Gemeinsames Bearbeiten"
# Ohne Lizenz bleibt die Leitung zu, der Blick der Verwaltung darauf aber offen:
# der Schalter im Panel soll auch dann etwas anzeigen, wenn der Zusatz nicht
# freigeschaltet ist, sonst sieht die Seite kaputt aus statt verschlossen.
pruefe "der Zustand ist abrufbar" "200" "$(code "$BASIS/api/system/mitschrift")"
pruefe "er sagt, dass die Lizenz fehlt" "False" \
       "$(hole "$BASIS/api/system/mitschrift" | feld "['lizenziert']")"
pruefe "eingeschaltet ist es trotzdem" "True" \
       "$(hole "$BASIS/api/system/mitschrift" | feld "['an']")"
pruefe "es sitzt niemand in einem Raum" "0" \
       "$(hole "$BASIS/api/system/mitschrift" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["raeume"]))')"
pruefe "ohne Anmeldung verschlossen" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' "$BASIS/api/system/mitschrift")"
# Die Leitung selbst und die Zahl der Mitschreibenden haengen am Zusatz.
pruefe "die Leitung ist ohne Lizenz zu" "402" "$(code "$BASIS/api/echtzeit/$BREIT")"
pruefe "die Zahl der Mitschreibenden ist zu" "402" \
       "$(code "$BASIS/api/pages/$BREIT/mitschreibende")"
# Und die Seite sagt es dem Browser: ohne Lizenz wird nicht gemeinsam
# geschrieben, also macht er auch keine Sitzung dafuer auf.
pruefe "die Seite meldet sich als nicht gemeinsam" "False" \
       "$(hole "$BASIS/api/pages/$BREIT" | feld "['gemeinsam']")"
# Der Schalter ist eine gewoehnliche Einstellung und laesst sich stellen.
pruefe "die Einstellung steht in der Liste" "1" \
       "$(hole "$BASIS/api/einstellungen" | python3 -c '
import json, sys
print(len([e for e in json.load(sys.stdin) if e["schluessel"] == "echtzeit"]))')"
hole -X PUT "$BASIS/api/einstellungen" -H 'Content-Type: application/json' \
     -d '{"schluessel":"echtzeit","wert":"nein"}' >/dev/null
pruefe "ausgeschaltet meldet der Zustand es auch" "False" \
       "$(hole "$BASIS/api/system/mitschrift" | feld "['an']")"
hole -X PUT "$BASIS/api/einstellungen" -H 'Content-Type: application/json' \
     -d '{"schluessel":"echtzeit","wert":"ja"}' >/dev/null
pruefe "wieder eingeschaltet" "True" \
       "$(hole "$BASIS/api/system/mitschrift" | feld "['an']")"

echo "== Sicherung"
# Der eigentliche Beweis ist nicht, dass ein Archiv herauskommt, sondern dass
# sich das Ergebnis zurueckspielen laesst. Alles andere ist eine Behauptung.
pruefe "Umfang ist lesbar" "200" "$(code "$BASIS/api/system/sicherung/umfang")"
pruefe "die Instanz haelt sich fuer bereit" "True" \
       "$(hole "$BASIS/api/system/sicherung/umfang" | feld "['bereit']")"
# Der Stand VOR der Sicherung. Das Erstellen vermerkt sich selbst in der
# Pruefspur, und zwar nachdem der Dump gezogen ist; ein Vergleich hinterher
# waere um genau diesen einen Eintrag daneben und saehe wie Datenverlust aus.
declare -A VORHER
for T in pages users pruefspur attachments einstellungen; do
    VORHER[$T]=$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc "SELECT count(*) FROM $T")
done
curl -s -b "$KEKSE" "$BASIS/api/system/sicherung" -o "$ARBEIT/sicherung.zip"
pruefe "ein Archiv kam an" "True" \
       "$(python3 -c "import os;print(os.path.getsize('$ARBEIT/sicherung.zip')>1000)")"
pruefe "es ist ein gueltiges ZIP" "True" \
       "$(python3 -c "import zipfile;print(zipfile.is_zipfile('$ARBEIT/sicherung.zip'))")"
# Die Marke am Ende. Ohne sie waere ein mittendrin abgebrochenes Archiv nicht
# von einem vollstaendigen zu unterscheiden, denn ein halbes ZIP ist ein
# gueltiges ZIP.
pruefe "die Marke FERTIG steht darin" "True" \
       "$(python3 -c "
import zipfile
z = zipfile.ZipFile('$ARBEIT/sicherung.zip')
print(any(n.endswith('/FERTIG') for n in z.namelist()))")"
pruefe "Dump und Anleitung liegen darin" "True" \
       "$(python3 -c "
import zipfile
n = zipfile.ZipFile('$ARBEIT/sicherung.zip').namelist()
print(any(x.endswith('/datenbank.sql') for x in n) and any(x.endswith('/LIESMICH.md') for x in n))")"
pruefe "der Dump ist nicht leer" "True" \
       "$(python3 -c "
import zipfile
z = zipfile.ZipFile('$ARBEIT/sicherung.zip')
d = next(n for n in z.namelist() if n.endswith('/datenbank.sql'))
print(z.getinfo(d).file_size > 2000)")"
# Der Suchindex gehoert NICHT hinein: such_tsv ist eine GENERATED-Spalte,
# PostgreSQL rechnet sie beim Einspielen neu. Stuende sie im Dump, waere das
# Zurueckspielen an genau dieser Stelle gescheitert.
pruefe "die Suchspalte steht als Vorschrift darin, nicht als Daten" "True" \
       "$(python3 -c "
import zipfile
z = zipfile.ZipFile('$ARBEIT/sicherung.zip')
d = next(n for n in z.namelist() if n.endswith('/datenbank.sql'))
t = z.read(d).decode('utf-8', 'replace')
vorschrift = 'GENERATED ALWAYS AS' in t
# In keiner COPY-Spaltenliste darf such_tsv auftauchen. Genau das ist die
# Eigenschaft, auf die es ankommt; die Spaltenreihenfolge zu raten waere ein
# Test, der beim naechsten ALTER TABLE grundlos faellt.
inDaten = any(z.startswith('COPY public.') and 'such_tsv' in z.split(')')[0]
              for z in t.splitlines())
print(vorschrift and not inDaten)")"

echo "== Sicherung fuer ein Skript"
# Der Weg fuer die Automatisierung. Ein Skript hat keinen Keks; ohne
# Losungswort darf es deshalb NICHTS bekommen, und mit dem richtigen alles.
pruefe "ohne Anmeldung und ohne Wort verschlossen" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' "$BASIS/api/system/sicherung")"
SWORT=$(hole -X POST "$BASIS/api/system/sicherung/token" | feld "['token']")
pruefe "ein Losungswort wurde erzeugt" "64" "$(printf '%s' "$SWORT" | wc -c | tr -d ' ')"
pruefe "damit geht es ohne Keks" "200" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $SWORT" "$BASIS/api/system/sicherung")"
pruefe "mit falschem Wort nicht" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer falsch' "$BASIS/api/system/sicherung")"
# Das Archiv aus dem Skript-Weg muss dasselbe sein wie aus dem Panel, und der
# Abruf muss eine Spur hinterlassen. Gezaehlt wird die DIFFERENZ: die Zeilen
# darueber haben schon einmal abgerufen, eine feste Summe haenge davon ab, wie
# oft dieser Abschnitt zwischendurch etwas holt.
spurZahl() {
    psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc \
        "SELECT count(*) FROM pruefspur WHERE aktion='sicherung.erstellt' AND akteur_name='Skript mit Losungswort'"
}
SPUR_VOR=$(spurZahl)
curl -s -H "Authorization: Bearer $SWORT" "$BASIS/api/system/sicherung" -o "$ARBEIT/skript.zip"
SPUR_NACH=$(spurZahl)
pruefe "auch dieses Archiv ist vollstaendig" "True" \
       "$(python3 -c "
import zipfile
z = zipfile.ZipFile('$ARBEIT/skript.zip')
print(any(n.endswith('/FERTIG') for n in z.namelist()))")"
pruefe "genau ein neuer Eintrag in der Pruefspur" "1" "$((SPUR_NACH - SPUR_VOR))"
pruefe "und er traegt die Adresse" "127.0.0.1" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc \
          "SELECT ip FROM pruefspur WHERE akteur_name='Skript mit Losungswort' ORDER BY zeitpunkt DESC LIMIT 1")"
pruefe "entfernt" "200" "$(code -X DELETE "$BASIS/api/system/sicherung/token")"
pruefe "danach ist der Weg wieder zu" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $SWORT" "$BASIS/api/system/sicherung")"

echo "== Sicherung laesst sich zurueckspielen"
# Eine zweite Datenbank daneben, den Dump hinein, und nachzaehlen. Ohne diesen
# Schritt ist eine Sicherung eine Vermutung.
createdb -h 127.0.0.1 -p "$PGPORT" -U nexora rueck
python3 -c "
import zipfile
z = zipfile.ZipFile('$ARBEIT/sicherung.zip')
d = next(n for n in z.namelist() if n.endswith('/datenbank.sql'))
open('$ARBEIT/rueck.sql','wb').write(z.read(d))"
psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d rueck -q -f "$ARBEIT/rueck.sql" > "$ARBEIT/rueck.log" 2>&1
pruefe "eingespielt ohne Fehler" "0" "$(grep -ci '^ERROR' "$ARBEIT/rueck.log" || true)"
for T in pages users pruefspur attachments einstellungen; do
    B=$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d rueck -tAc "SELECT count(*) FROM $T")
    pruefe "$T vollstaendig" "${VORHER[$T]}" "$B"
done
# Und die Suche muss in der zurueckgespielten Datenbank von selbst wieder gehen.
# psql schreibt Wahrheitswerte als t und f, nicht als True.
pruefe "die Suchspalte wurde beim Einspielen neu berechnet" "t" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d rueck -tAc \
          "SELECT count(*) > 0 FROM pages WHERE such_tsv IS NOT NULL")"
# Und die Suche muss darauf wirklich greifen, nicht bloss nicht null sein. Der
# Vergleich geht gegen den eigenen Titel jeder Seite: ein festes Suchwort waere
# ein Test, der davon abhaengt, welche Seiten die Abschnitte davor gerade
# stehen lassen, und der Abschnitt Papierkorb entfernt eine davon endgueltig.
pruefe "und die Suche greift darauf" "t" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d rueck -tAc \
          "SELECT count(*) > 0 FROM pages
             WHERE title <> '' AND such_tsv @@ plainto_tsquery('german', title)")"
dropdb -h 127.0.0.1 -p "$PGPORT" -U nexora rueck

echo "== Sicherung wieder einspielen"
# Der eigentliche Beweis: sichern, etwas aendern, einspielen, und die Aenderung
# muss weg sein. Alles darunter -- Marke, Rueckweg, Neustart -- haengt daran.
curl -s -b "$KEKSE" "$BASIS/api/system/sicherung" -o "$ARBEIT/stand.zip"
SEITEN_VORHER=$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc "SELECT count(*) FROM pages")
# Eine Seite, die es in der Sicherung NICHT gibt.
DANACH=$(hole -X POST "$BASIS/api/pages" -H 'Content-Type: application/json' \
         -d '{"title":"Nach der Sicherung entstanden"}' | feld "['id']")
pruefe "die neue Seite ist da" "200" "$(code "$BASIS/api/pages/$DANACH")"

# Ein Archiv ohne die Marke muss abgelehnt werden. Es ist ein gueltiges ZIP,
# und genau das ist die Falle: es liesse sich sonst einspielen und legte einen
# halben Bestand ueber einen ganzen.
python3 -c "
import zipfile, shutil
shutil.copy('$ARBEIT/stand.zip', '$ARBEIT/halb.zip')
alt = zipfile.ZipFile('$ARBEIT/stand.zip')
neu = zipfile.ZipFile('$ARBEIT/halb.zip', 'w')
for n in alt.namelist():
    if not n.endswith('/FERTIG'):
        neu.writestr(n, alt.read(n))
neu.close()"
pruefe "ein Archiv ohne FERTIG wird abgelehnt" "400" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$KEKSE" -X POST "$BASIS/api/system/wiederherstellung" \
          -F "datei=@$ARBEIT/halb.zip" -F "bestaetigung=wiederherstellen")"
pruefe "ohne Bestaetigung ebenfalls" "400" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$KEKSE" -X POST "$BASIS/api/system/wiederherstellung" \
          -F "datei=@$ARBEIT/stand.zip")"
pruefe "die neue Seite steht immer noch" "200" "$(code "$BASIS/api/pages/$DANACH")"

# Jetzt richtig.
EINSPIEL=$(curl -s -b "$KEKSE" -X POST "$BASIS/api/system/wiederherstellung" \
           -F "datei=@$ARBEIT/stand.zip" -F "bestaetigung=wiederherstellen")
pruefe "eingespielt" "True" "$(printf '%s' "$EINSPIEL" | feld "['ok']")"
RUECKWEG=$(printf '%s' "$EINSPIEL" | feld "['rueckweg']")
pruefe "ein Rueckweg wurde abgelegt" "True" \
       "$(python3 -c "
import os
p = os.path.join('$ARBEIT/anhaenge', '$RUECKWEG')
print(os.path.exists(p) and os.path.getsize(p) > 1000)")"

# Der Dienst beendet sich nach dem Einspielen selbst. Im Betrieb startet Docker
# ihn neu; hier muss der Test das tun.
sleep 3
halte_dienst_an
starte_dienst
pruefe "die Seite von nach der Sicherung ist weg" "404" "$(code "$BASIS/api/pages/$DANACH")"
pruefe "der Bestand entspricht wieder der Sicherung" "$SEITEN_VORHER" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc "SELECT count(*) FROM pages")"
pruefe "und die Anmeldung gilt noch" "200" "$(code "$BASIS/api/auth/me")"
# Die Suche muss nach dem Einspielen von selbst wieder greifen, ohne dass
# jemand den Index neu aufbaut.
pruefe "die Suche greift ohne Zutun" "t" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc \
          "SELECT count(*) > 0 FROM pages WHERE title <> '' AND such_tsv @@ plainto_tsquery('german', title)")"

echo "== Verzeichnis-Verwaltung"
# Nachsehen darf ein Administrator immer, auch ohne Lizenz: sonst sieht eine
# Instanz nicht einmal, dass da etwas eingerichtet ist, das nicht laeuft.
pruefe "Einrichtung ist lesbar" "200" "$(code "$BASIS/api/system/ldap")"
pruefe "hier ist nichts eingerichtet" "False" "$(hole "$BASIS/api/system/ldap" | feld "['aktiv']")"
pruefe "und nichts freigeschaltet" "False" "$(hole "$BASIS/api/system/ldap" | feld "['lizenziert']")"
# Der Filter ist leer in der Konfiguration und darf trotzdem nicht leer
# herauskommen: es greift dieselbe Vorgabe wie beim Anmelden.
pruefe "der Filter zeigt die Vorgabe" "True" \
       "$(hole "$BASIS/api/system/ldap" | python3 -c 'import json,sys;print("objectClass=person" in json.load(sys.stdin)["benutzerFilter"])')"
pruefe "das Dienstkonto-Passwort steht nicht drin" "True" \
       "$(hole "$BASIS/api/system/ldap" | python3 -c '
import json, sys
print("bindPasswort" not in json.load(sys.stdin))')"
pruefe "Probieren ohne Lizenz weist ab" "402" \
       "$(code -X POST "$BASIS/api/system/ldap/test" -H 'Content-Type: application/json' \
          -d '{"benutzer":"wer"}')"

echo "== Grenzprobe"
# Der Weg nimmt einen Rumpf an und wirft ihn weg. Geprueft wird, dass er
# wirklich zaehlt, was ankommt: die Oberflaeche schachtelt die Grenze anhand
# dieser Antwort ein, eine geratene Zahl waere schlimmer als keine.
head -c 1048576 /dev/zero > "$ARBEIT/ein-mb"
pruefe "ein Megabyte kommt an" "1048576" \
       "$(curl -s -b "$KEKSE" -X POST "$BASIS/api/system/grenzprobe" \
          -H 'Content-Type: application/octet-stream' \
          --data-binary "@$ARBEIT/ein-mb" | feld "['bytes']")"
pruefe "ein leerer Rumpf ist kein Fehler" "0" \
       "$(curl -s -b "$KEKSE" -X POST "$BASIS/api/system/grenzprobe" \
          -H 'Content-Type: application/octet-stream' --data-binary '' | feld "['bytes']")"
pruefe "ohne Anmeldung verschlossen" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASIS/api/system/grenzprobe" \
          -H 'Content-Type: application/octet-stream' --data-binary '')"

echo "== Was ohne Lizenz zu bleibt"
# Anhänge, Freigaben, Kommentare und die Ausgabe einer ganzen Ablage sind
# kostenpflichtige Zusätze. Ohne Lizenz lässt sich hier nichts davon
# durchspielen; geprüft wird darum, dass die Wege verschlossen sind und nicht
# etwa mit einem Fehler antworten, der nach einem Defekt aussieht.
pruefe "Anhänge sind zu" "402" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$KEKSE" "$BASIS/api/pages/$BREIT/attachments")"
pruefe "Freigabe ist zu" "402" "$(code -X POST "$BASIS/api/pages/$BREIT/share")"
pruefe "die @-Liste ist zu" "402" "$(code "$BASIS/api/pages/$BREIT/erwaehnbare")"
pruefe "die Ausgabe einer Ablage ist zu" "402" "$(code "$BASIS/api/spaces/$ABL_ID/export")"
pruefe "der öffentliche Weg zu einer Datei ist zu" "402" \
       "$(curl -s -o /dev/null -w '%{http_code}' "$BASIS/api/public/egal/dateien/egal")"
# 402 und nicht 404: der Weg zum Ersetzen einer markierten PDF ist eingetragen,
# er ist nur verschlossen. Ein 404 hiesse, die Route fehlt -- und das faende
# niemand, bevor eine Lizenz da ist.
pruefe "das Ersetzen einer markierten PDF ist zu" "402" \
       "$(curl -s -o /dev/null -w '%{http_code}' -X PUT -b "$KEKSE" \
          -H 'Content-Type: application/pdf' --data-binary '%PDF-1.4' \
          "$BASIS/api/pages/$BREIT/attachments/egal/pdf")"

echo "== Eigenes Profil"
SELBST_ID=$(hole "$BASIS/api/auth/me" | feld "['id']")
# Name und Bild gehoeren dem Konto selbst, nicht der Verwaltung. Und das Bild
# wird nach seinem INHALT geprueft: was der Browser als Typ behauptet, sagt
# nichts darueber, was wirklich ankommt.
pruefe "anfangs kein Bild" "404" "$(code "$BASIS/api/users/$SELBST_ID/bild")"
pruefe "der Name laesst sich aendern" "Rauch Umbenannt" \
       "$(hole -X PUT "$BASIS/api/profil" -H 'Content-Type: application/json' \
          -d '{"name":"Rauch Umbenannt"}' | feld "['name']")"
pruefe "und steht sofort am Konto" "Rauch Umbenannt" "$(hole "$BASIS/api/auth/me" | feld "['name']")"
pruefe "ein leerer Name wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/profil" -H 'Content-Type: application/json' -d '{"name":"  "}')"

# Ein winziges echtes PNG, von Hand gebaut: ein Pixel reicht, um zu pruefen,
# dass die Erkennung nach dem Inhalt geht. Es wiegt 69 Byte -- und genau daran
# ist einmal eine Untergrenze in Bytes gescheitert, die gut gemeint war.
python3 - "$ARBEIT" <<'PYTHON'
import struct, sys, zlib
def stueck(art, inhalt):
    roh = art + inhalt
    return struct.pack(">I", len(inhalt)) + roh + struct.pack(">I", zlib.crc32(roh))
kopf = struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)
daten = zlib.compress(b"\x00\xff\x00\x00")
png = b"\x89PNG\r\n\x1a\n" + stueck(b"IHDR", kopf) + stueck(b"IDAT", daten) + stueck(b"IEND", b"")
open(sys.argv[1] + "/bild.png", "wb").write(png)
PYTHON
pruefe "ein Bild laesst sich setzen" "True" \
       "$(hole -X PUT "$BASIS/api/profil/bild" -H 'Content-Type: image/png' \
          --data-binary "@$ARBEIT/bild.png" | feld "['ok']")"
pruefe "danach ist es abrufbar" "200" "$(code "$BASIS/api/users/$SELBST_ID/bild")"
pruefe "und kommt als PNG heraus" "image/png" \
       "$(curl -s -o /dev/null -w '%{content_type}' -b "$KEKSE" "$BASIS/api/users/$SELBST_ID/bild")"
pruefe "das Konto weiss jetzt von einem Bild" "True" \
       "$(hole "$BASIS/api/auth/me" | python3 -c 'import json,sys;print(bool(json.load(sys.stdin).get("bildStand")))')"
# Ein umbenanntes Programm mit dem Typ eines Bildes darf NICHT durch: sonst
# laege es spaeter als image/png in der Zeile und kaeme so auch wieder heraus.
printf '\177ELF\002\001\001\000und noch etwas mehr Fuellung als hundert Zeichen, damit es nicht schon an der Mindestlaenge scheitert.' > "$ARBEIT/kein-bild.png"
pruefe "eine leere Anfrage wird abgewiesen" "400" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$KEKSE" -X PUT \
          -H 'Content-Type: image/png' --data-binary '' "$BASIS/api/profil/bild")"
pruefe "ein Programm mit Bildnamen nicht" "400" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$KEKSE" -X PUT \
          -H 'Content-Type: image/png' --data-binary "@$ARBEIT/kein-bild.png" \
          "$BASIS/api/profil/bild")"
pruefe "das alte Bild steht noch" "200" "$(code "$BASIS/api/users/$SELBST_ID/bild")"
pruefe "es laesst sich entfernen" "True" "$(hole -X DELETE "$BASIS/api/profil/bild" | feld "['ok']")"
pruefe "danach gibt es keines mehr" "404" "$(code "$BASIS/api/users/$SELBST_ID/bild")"
# Zurueck auf den alten Namen, damit die folgenden Abschnitte ihn wiederfinden.
hole -X PUT "$BASIS/api/profil" -H 'Content-Type: application/json' \
     -d '{"name":"Rauch Test"}' >/dev/null

echo "== Passwort wechseln"
# Das bisherige Passwort ist Pflicht, auch bei offener Sitzung: sonst genuegte
# ein unbeaufsichtigter Browser, um das Konto zu uebernehmen.
pruefe "falsches altes Passwort wird abgewiesen" "403" \
       "$(code -X POST "$BASIS/api/auth/passwort" -H 'Content-Type: application/json' \
          -d '{"alt":"stimmt-nicht","neu":"neues-passwort"}')"
pruefe "ein zu kurzes neues auch" "400" \
       "$(code -X POST "$BASIS/api/auth/passwort" -H 'Content-Type: application/json' \
          -d '{"alt":"rauchtest-passwort","neu":"kurz"}')"
pruefe "dasselbe noch einmal ist kein Wechsel" "400" \
       "$(code -X POST "$BASIS/api/auth/passwort" -H 'Content-Type: application/json' \
          -d '{"alt":"rauchtest-passwort","neu":"rauchtest-passwort"}')"
pruefe "der Wechsel geht durch" "True" \
       "$(hole -X POST "$BASIS/api/auth/passwort" -H 'Content-Type: application/json' \
          -d '{"alt":"rauchtest-passwort","neu":"zweites-passwort"}' | feld "['ok']")"
# Das eigene Geraet bleibt angemeldet, alles andere faellt. Ohne diese Zeile
# waere der Wechsel eine Abmeldung.
pruefe "dieses Gerät bleibt angemeldet" "200" "$(code "$BASIS/api/auth/me")"
pruefe "das alte Passwort öffnet nicht mehr" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASIS/api/auth/login" \
          -H 'Content-Type: application/json' \
          -d '{"kennung":"rauch@test.invalid","password":"rauchtest-passwort"}')"
pruefe "das neue öffnet" "200" \
       "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASIS/api/auth/login" \
          -H 'Content-Type: application/json' \
          -d '{"kennung":"rauch@test.invalid","password":"zweites-passwort"}')"
# Die Pruefspur ist ein Zusatz und ohne Lizenz nicht LESBAR; geschrieben wird
# sie trotzdem, siehe main.go. Geprueft wird deshalb in der Datenbank.
pruefe "der Wechsel steht im Protokoll" "1" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc \
          "SELECT count(*) FROM pruefspur WHERE aktion='konto.passwort'")"

# Zuruecksetzen durch die Verwaltung, an einem zweiten Konto.
ZWEITER=$(hole -X POST "$BASIS/api/users" -H 'Content-Type: application/json' \
          -d '{"email":"zweiter@test.invalid","name":"Zweiter","password":"erstes-passwort"}' \
          | feld "['id']")
pruefe "das eigene Konto weist dieser Weg ab" "400" \
       "$(code -X PUT "$BASIS/api/users/$(hole "$BASIS/api/auth/me" | feld "['id']")/passwort" \
          -H 'Content-Type: application/json' -d '{"neu":"anderes-passwort"}')"
pruefe "ein fremdes Konto lässt sich zurücksetzen" "True" \
       "$(hole -X PUT "$BASIS/api/users/$ZWEITER/passwort" -H 'Content-Type: application/json' \
          -d '{"neu":"gesetztes-passwort"}' | feld "['ok']")"
pruefe "das gesetzte Passwort öffnet" "200" \
       "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASIS/api/auth/login" \
          -H 'Content-Type: application/json' \
          -d '{"kennung":"zweiter@test.invalid","password":"gesetztes-passwort"}')"

echo "== Eigene Rechner"
pruefe "die Liste ist zunächst leer" "0" \
       "$(hole "$BASIS/api/system/rechner" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["rechner"]))')"
pruefe "ohne Port wird abgewiesen" "400" \
       "$(code -X POST "$BASIS/api/system/rechner" -H 'Content-Type: application/json' \
          -d '{"name":"ohne","ziel":"10.0.0.5"}')"
pruefe "ein fremdes Schema auch" "400" \
       "$(code -X POST "$BASIS/api/system/rechner" -H 'Content-Type: application/json' \
          -d '{"name":"ssh","ziel":"ssh://10.0.0.5:22"}')"
# Der Dienst klopft an sich selbst: eine Adresse, die im Rauchtest verlaesslich
# antwortet, ohne dass ein zweiter Rechner im Spiel waere.
SELBST=$(hole -X POST "$BASIS/api/system/rechner" -H 'Content-Type: application/json' \
         -d "{\"name\":\"ich selbst\",\"ziel\":\"127.0.0.1:$APIPORT\"}" | feld "['id']")
pruefe "der eigene Port antwortet" "antwortet" \
       "$(hole "$BASIS/api/system/rechner" | python3 -c '
import json, sys
print(next(r["zustand"] for r in json.load(sys.stdin)["rechner"] if r["name"] == "ich selbst"))')"
# Port 9 ist discard und in keinem Container belegt.
hole -X POST "$BASIS/api/system/rechner" -H 'Content-Type: application/json' \
     -d '{"name":"stiller","ziel":"127.0.0.1:9"}' >/dev/null
pruefe "ein toter Port heißt still" "still" \
       "$(hole "$BASIS/api/system/rechner" | python3 -c '
import json, sys
print(next(r["zustand"] for r in json.load(sys.stdin)["rechner"] if r["name"] == "stiller"))')"
# Der Dienst nennt sich selbst nicht in einer Kopfzeile Server, deshalb bleibt
# die Spalte hier leer -- geraten wird nichts.
pruefe "ohne Kennung bleibt die Spalte leer" "" \
       "$(hole "$BASIS/api/system/rechner" | python3 -c '
import json, sys
print(next((r.get("fassung", "") for r in json.load(sys.stdin)["rechner"] if r["name"] == "ich selbst"), "FEHLT"))')"
pruefe "die Zeile lässt sich ändern" "anders benannt" \
       "$(hole -X PUT "$BASIS/api/system/rechner/$SELBST" -H 'Content-Type: application/json' \
          -d "{\"name\":\"anders benannt\",\"ziel\":\"127.0.0.1:$APIPORT\"}" | feld "['name']")"
pruefe "und entfernen" "True" \
       "$(hole -X DELETE "$BASIS/api/system/rechner/$SELBST" | feld "['ok']")"
pruefe "danach steht nur noch der stille da" "1" \
       "$(hole "$BASIS/api/system/rechner" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["rechner"]))')"
# Und nichts von alledem braucht einen Prometheus. Der ist samt Grafana
# ausgebaut: keine Einstellung, kein Losungswort, kein Weg. Geprueft wird das
# hier, weil eine Ausbaustelle sich sonst still zurueckschleicht.
pruefe "keine Prometheus-Einstellung mehr" "0" \
       "$(hole "$BASIS/api/einstellungen" | python3 -c '
import json, sys
schluessel = {e["schluessel"] for e in json.load(sys.stdin)}
print(len(schluessel & {"prometheus_adresse", "metriken_token"}))')"
pruefe "den Weg /metrics gibt es nicht mehr" "404" "$(code "$BASIS/metrics")"
pruefe "und seine Verwaltung auch nicht" "404" "$(code "$BASIS/api/system/metriken")"

echo "== Programme werden nicht angenommen"
# Anhaenge sind ein Zusatz und ohne Lizenz zu; geprueft wird deshalb die
# Erkennung selbst und die Ablehnung an dem Weg, der offen ist: die Einfuhr.
# Ein Archiv mit einer ELF-Datei darf die Seite anlegen und die Datei nicht.
PROG="$ARBEIT/programm"
printf '\177ELF\002\001\001\000ohne alles' > "$PROG"
python3 - "$ARBEIT" <<'PYTHON'
import sys, zipfile
arbeit = sys.argv[1]
with zipfile.ZipFile(arbeit + "/programm.zip", "w") as z:
    z.writestr("Notiz.md", "# Mit Beilage\n\n[Werkzeug](werkzeug)\n")
    z.write(arbeit + "/programm", "werkzeug")
PYTHON
EINFUHR=$(hole -X POST "$BASIS/api/import" -F "file=@$ARBEIT/programm.zip")
pruefe "die Seite kommt an" "1" "$(printf '%s' "$EINFUHR" | feld "['seiten']")"
# Weiter kommt der Rauchtest hier nicht: Beilagen sind ein Zusatz und werden
# ohne Lizenz gar nicht erst angefasst, das Programm faellt also schon eine
# Stufe vorher heraus. Dass die vier Bytes am Anfang erkannt werden, prueft
# TestLinuxProgrammWirdErkannt in internal/handlers.
pruefe "ohne Lizenz kommt keine Beilage mit" "" \
       "$(printf '%s' "$EINFUHR" | python3 -c '
import json, sys
d = json.load(sys.stdin)
print(d.get("beilagen", ""))')"

echo "== Verschlüsselt sprechen"
# Der Dienst kann selbst HTTPS. Geprueft wird an einem zweiten Start mit einem
# eigens erzeugten Zertifikat: dass er die Datei annimmt, dass er wirklich
# verschluesselt antwortet, und dass ein Gegenueber, das die Stelle kennt, ihm
# glaubt. Genau das tut die Oberflaeche im Verbund, siehe pki/erzeuge.sh.
halte_dienst_an

openssl req -x509 -newkey rsa:2048 -sha256 -days 2 -nodes \
    -keyout "$ARBEIT/dienst.key" -out "$ARBEIT/dienst.crt" \
    -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" >/dev/null 2>&1

DATABASE_URL="postgres://nexora@127.0.0.1:${PGPORT}/nexora?sslmode=disable" \
JWT_SECRET="rauchtest-geheimnis-lang-genug-fuer-hs256" \
NEXORA_DATA_DIR="$ARBEIT/anhaenge" \
PORT="$APIPORT" \
NEXORA_CONFIG="/dev/null" \
NEXORA_TLS_ZERTIFIKAT="$ARBEIT/dienst.crt" \
NEXORA_TLS_SCHLUESSEL="$ARBEIT/dienst.key" \
NEXORA_TLS_WURZEL="$ARBEIT/dienst.crt" \
"$ARBEIT/nexora" >> "$ARBEIT/dienst.log" 2>&1 &
DIENST_PID=$!

SICHER="https://127.0.0.1:$APIPORT"
for i in $(seq 1 40); do
    sleep 0.5
    curl -fsS --max-time 2 --cacert "$ARBEIT/dienst.crt" "$SICHER/healthz" >/dev/null 2>&1 && break
done

pruefe "verschlüsselt erreichbar, mit Prüfung des Zertifikats" "ok" \
       "$(curl -s --max-time 3 --cacert "$ARBEIT/dienst.crt" "$SICHER/healthz")"
# Wer die Stelle nicht kennt, kommt nicht durch. Ohne diese Zeile bewiese die
# vorige nur, dass irgendetwas antwortet.
pruefe "ohne die Stelle bleibt es zu" "000" \
       "$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$SICHER/healthz")"
# Und unverschluesselt geht nichts mehr durch: auf eine Anfrage im Klartext
# antwortet Go mit 400 und der Bemerkung, hier werde TLS gesprochen -- nicht mit
# der Seite. 400 ist hier also das gewuenschte Ergebnis und kein Fehler.
pruefe "im Klartext kommt nichts mehr durch" "400" \
       "$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$BASIS/healthz")"
pruefe "und er sagt auch, warum" "1" \
       "$(curl -s --max-time 3 "$BASIS/healthz" | grep -c 'HTTPS server')"
pruefe "die Anmeldung geht auch verschlüsselt" "200" \
       "$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 --cacert "$ARBEIT/dienst.crt" \
          -X POST "$SICHER/api/auth/login" -H 'Content-Type: application/json' \
          -d '{"kennung":"rauch@test.invalid","password":"zweites-passwort"}')"

echo
if [ "$fehler" -gt 0 ]; then
    echo "$fehler Prüfungen sind gefallen." >&2
    echo "Protokoll des Dienstes:" >&2
    tail -40 "$ARBEIT/dienst.log" >&2
    exit 1
fi
echo "Rauchtest bestanden."
