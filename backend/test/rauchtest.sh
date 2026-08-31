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
pruefe "steht anfangs auf normal" "normal" "$(hole "$BASIS/api/pages/$BREIT" | feld "['breite']")"
pruefe "auf breit gesetzt" "200" \
       "$(code -X PUT "$BASIS/api/pages/$BREIT/breite" -H 'Content-Type: application/json' \
          -d '{"breite":"breit"}')"
pruefe "steht jetzt auf breit" "breit" "$(hole "$BASIS/api/pages/$BREIT" | feld "['breite']")"
pruefe "Unsinn wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/pages/$BREIT/breite" -H 'Content-Type: application/json' \
          -d '{"breite":"riesig"}')"

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
# Das Wort der Kennzahlen darf hier NICHT gelten. Zwei Reichweiten, zwei
# Woerter: das eine gibt eine Zusammenfassung heraus, das andere den ganzen
# Bestand. Hier eigens erzeugt und gleich wieder entfernt, weil der Abschnitt
# Kennzahlen erst weiter unten kommt und eine leere Variable hier alles
# durchgehen liesse, ohne dass der Test es merkt.
MWORT=$(hole -X POST "$BASIS/api/system/metriken/token" | feld "['token']")
pruefe "das Kennzahlen-Wort ist ein anderes" "True" \
       "$(python3 -c "print('$MWORT' != '$SWORT' and len('$MWORT') > 10)")"
pruefe "und oeffnet die Sicherung nicht" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $MWORT" "$BASIS/api/system/sicherung")"
pruefe "umgekehrt oeffnet das Sicherungs-Wort die Kennzahlen nicht" "404" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $SWORT" "$BASIS/metrics")"
hole -X DELETE "$BASIS/api/system/metriken/token" >/dev/null
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

echo "== Kennzahlen"
# Ohne Losungswort gibt es den Weg nicht, und zwar mit 404 und nicht mit 401:
# dass es ihn gibt, braucht niemand zu erfahren, der ihn nicht abholen darf.
pruefe "ohne Losungswort nicht vorhanden" "404" "$(code "$BASIS/metrics")"
pruefe "auch mit erfundenem Losungswort nicht" "404" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer erfunden' "$BASIS/metrics")"

# Einschalten aus dem Panel heraus. Das Losungswort wird erzeugt, nicht
# eingetippt, und muss danach sofort gelten: es steht in der Datenbank, und
# der Zwischenspeicher muss mitgezogen sein, sonst gaelte weiter der alte Wert.
pruefe "Zustand ist lesbar" "200" "$(code "$BASIS/api/system/metriken")"
pruefe "anfangs aus" "False" "$(hole "$BASIS/api/system/metriken" | feld "['aktiv']")"
WORT=$(hole -X POST "$BASIS/api/system/metriken/token" | feld "['token']")
pruefe "ein Losungswort wurde erzeugt" "48" "$(printf '%s' "$WORT" | wc -c | tr -d ' ')"
pruefe "jetzt an" "True" "$(hole "$BASIS/api/system/metriken" | feld "['aktiv']")"
pruefe "und der Weg antwortet" "200" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $WORT" "$BASIS/metrics")"
pruefe "mit einem alten Wort nicht" "404" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer erfunden' "$BASIS/metrics")"
pruefe "das Abholen wird vermerkt" "True" \
       "$(hole "$BASIS/api/system/metriken" | python3 -c 'import json,sys;print(json.load(sys.stdin)["abholungen"] > 0)')"
pruefe "der fertige Abschnitt traegt das Wort" "True" \
       "$(hole "$BASIS/api/system/metriken" | WORT="$WORT" python3 -c '
import json, os, sys
print(os.environ["WORT"] in json.load(sys.stdin)["prometheus"])')"
# Ein neues Wort macht das alte sofort ungueltig.
NEU=$(hole -X POST "$BASIS/api/system/metriken/token" | feld "['token']")
pruefe "das alte Wort gilt nicht mehr" "404" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $WORT" "$BASIS/metrics")"
pruefe "das neue schon" "200" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $NEU" "$BASIS/metrics")"
pruefe "abgeschaltet" "200" "$(code -X DELETE "$BASIS/api/system/metriken/token")"
pruefe "danach gibt es den Weg nicht mehr" "404" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $NEU" "$BASIS/metrics")"

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

echo
if [ "$fehler" -gt 0 ]; then
    echo "$fehler Prüfungen sind gefallen." >&2
    echo "Protokoll des Dienstes:" >&2
    tail -40 "$ARBEIT/dienst.log" >&2
    exit 1
fi
echo "Rauchtest bestanden."
