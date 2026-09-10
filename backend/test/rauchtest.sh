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
# A free port instead of a fixed one.
#
# Fixed numbers went well until two runs overlapped: then one run's database was
# already on the port the other wanted, and the second run failed with "could
# not start server" without anything being wrong with it. Whoever specifies a
# number still gets it.
freier_port() {
    # Upwards from the given number until one belongs to nobody. The attempt to
    # connect is the probe: if it succeeds, somebody is already listening
    # there.
    local p="$1"
    while [ "$p" -lt $((${1} + 200)) ]; do
        if ! (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then
            echo "$p"
            return 0
        fi
        exec 3>&- 2>/dev/null
        p=$((p + 1))
    done
    echo "$1" # nichts frei: mit der Vorgabe weitermachen und den Fehler zeigen
}
PGPORT="${PGPORT:-$(freier_port 55432)}"
APIPORT="${APIPORT:-$(freier_port 58080)}"
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
# If the database does not come up, its log is the only thing that answers the
# question -- and it lies in a directory the cleanup removes right afterwards.
# So it has to come out beforehand, otherwise all that stands there in the end
# is "Examine the log output" and the log is already gone.
if ! pg_ctl -D "$ARBEIT/db" -l "$ARBEIT/pg.log" \
            -o "-p $PGPORT -k $ARBEIT -h 127.0.0.1" -w start >/dev/null; then
    echo "Die Datenbank kam nicht hoch. Ihr Protokoll:"
    sed "s/^/    /" "$ARBEIT/pg.log" 2>/dev/null || echo "    (kein Protokoll geschrieben)"
    echo "Wer horcht auf Port $PGPORT:"
    if (exec 3<>"/dev/tcp/127.0.0.1/$PGPORT") 2>/dev/null; then
        exec 3>&- 2>/dev/null
        echo "    jemand -- auf dem Port horcht bereits ein Dienst"
    else
        echo "    niemand -- der Port war frei, es lag also nicht daran"
    fi
    exit 1
fi
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

# The colour of a space. It sits in the space itself and not in a setting of
# the viewer: it should apply to everybody who sees it.
pruefe "anfangs ohne eigene Farbe" "" \
       "$(hole "$BASIS/api/spaces" | python3 -c '
import json, sys
print(json.load(sys.stdin)[0].get("farbe", ""))')"
pruefe "eine Farbe laesst sich setzen" "#2383e2" \
       "$(hole -X PUT "$BASIS/api/spaces/$ABL_ID/farbe" -H 'Content-Type: application/json' \
          -d '{"farbe":"#2383e2"}' | feld "['farbe']")"
pruefe "und steht in der Liste" "#2383e2" \
       "$(hole "$BASIS/api/spaces" | python3 -c '
import json, sys
print(json.load(sys.stdin)[0]["farbe"])')"
# Not a colour value but some text: reject instead of writing it into the row.
# Out of the row it would later come back out into a style attribute.
pruefe "ein erfundener Wert wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/spaces/$ABL_ID/farbe" -H 'Content-Type: application/json' \
          -d '{"farbe":"rot; background:url(x)"}')"
pruefe "die alte Farbe steht noch" "#2383e2" \
       "$(hole "$BASIS/api/spaces" | python3 -c '
import json, sys
print(json.load(sys.stdin)[0]["farbe"])')"
pruefe "leer setzt zurueck" "" \
       "$(hole -X PUT "$BASIS/api/spaces/$ABL_ID/farbe" -H 'Content-Type: application/json' \
          -d '{"farbe":""}' | feld "['farbe']")"

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
# The default is twelve hours. It used to be seven days, that is 168.
print(11.9 < stunden < 12.1)')"
pruefe "das Geraet wird benannt" "Firefox auf Windows" \
       "$(hole "$BASIS/api/sitzungen" | python3 -c '
import json,sys
print(next(s["browser"] for s in json.load(sys.stdin) if not s["diese"]))')"
FREMD=$(hole "$BASIS/api/sitzungen" | python3 -c '
import json,sys
print(next(s["id"] for s in json.load(sys.stdin) if not s["diese"]))')
pruefe "fremde Sitzung beenden" "200" "$(code -X DELETE "$BASIS/api/sitzungen/$FREMD")"
# The token of the second sign-in has to be worthless at once, which is exactly
# what the old, purely computed session could not do.
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
# Without setup and without a licence nothing may be offered; a button that
# then answers 402 would be a promise without cover.
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
# Empty means: no choice of its own, the instance default applies.
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

echo "== Aussehen am eigenen Konto"
# Since the move, base tone and accent lie in the account's row and no longer
# in the settings table. That can only be checked against a real database:
# whether the columns are there, whether the value survives a reload and whether
# nonsense is rejected instead of landing in a CSS variable.
pruefe "Vorgabe ist grau" "grau" "$(hole "$BASIS/api/design" | feld "['grundton']")"
pruefe "Vorgabe ist Blau" "#2383e2" "$(hole "$BASIS/api/design" | feld "['akzent']")"
pruefe "eigene Wahl wird angenommen" "200" \
       "$(code -X PUT "$BASIS/api/design" -H 'Content-Type: application/json' \
          -d '{"grundton":"dunkel","akzent":"#8250df"}')"
pruefe "und steht beim naechsten Abruf da" "dunkel" \
       "$(hole "$BASIS/api/design" | feld "['grundton']")"
pruefe "samt Akzent" "#8250df" "$(hole "$BASIS/api/design" | feld "['akzent']")"
pruefe "ein unbekannter Ton wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/design" -H 'Content-Type: application/json' \
          -d '{"grundton":"neon","akzent":"#8250df"}')"
pruefe "und eine Farbe, die keine ist, auch" "400" \
       "$(code -X PUT "$BASIS/api/design" -H 'Content-Type: application/json' \
          -d '{"grundton":"dunkel","akzent":"rot; background:url(x)"}')"
pruefe "leer setzt auf die Vorgabe zurueck" "grau" \
       "$(hole -X PUT "$BASIS/api/design" -H 'Content-Type: application/json' \
          -d '{"grundton":"","akzent":""}' | feld "['grundton']")"
# The appearance no longer appears among the administration's settings.
pruefe "keine Design-Einstellung mehr in der Verwaltung" "0" \
       "$(hole "$BASIS/api/einstellungen" | python3 -c 'import json,sys;print(sum(1 for e in json.load(sys.stdin) if e["schluessel"].startswith("design_")))')"

echo "== Anmeldeversuche"
# The evaluation computes with intervals out of a number and with FILTER
# counts. Both only show up once it really runs against Postgres, see the head
# of this file. A failed attempt is therefore triggered here by hand.
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
# tage=0 means "everything" and becomes a very large number internally. That is
# exactly what the interval arithmetic has failed on once before.
pruefe "tage=0 liefert alles" "200" "$(code "$BASIS/api/system/anmeldungen?tage=0")"
pruefe "Filter nach Adresse greift" "0" \
       "$(hole "$BASIS/api/system/anmeldungen?ip=10.9.9.9" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["versuche"]))')"

echo "== Puls"
# Counting happens without a lock on the hot path, and the slots are switched by
# the clock. Whether that really fits together in the running service only shows
# up here: the unit tests see only the package, not the chain.
pruefe "Puls antwortet" "200" "$(code "$BASIS/api/system/puls")"
pruefe "der Vorrat nennt seine Obergrenze" "True" \
       "$(hole "$BASIS/api/system/puls" | python3 -c 'import json,sys;print(json.load(sys.stdin)["vorrat"]["hoechstens"] > 0)')"
# Not the number of accesses without a free connection: that stands above zero
# even on an unloaded instance, because the pool is empty at start-up and the
# first accesses have their connection opened first. What is telling is the mean
# wait time, and that has to be vanishing here.
pruefe "kaum Wartezeit auf eine Verbindung" "True" \
       "$(hole "$BASIS/api/system/puls" | python3 -c 'import json,sys;print(json.load(sys.stdin)["vorrat"]["mittelWarteMs"] < 1.0)')"
pruefe "die Minute hat 59 Faecher" "59" \
       "$(hole "$BASIS/api/system/puls" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["anfragen"]["minute"]))')"
# The many calls of the sections above must have left their mark.
pruefe "es wurde etwas gezaehlt" "True" \
       "$(hole "$BASIS/api/system/puls" | python3 -c 'import json,sys;print(json.load(sys.stdin)["anfragen"]["gesamt"] > 50)')"
# The query route does not count itself along, otherwise it would stand as
# background noise in every measurement it is meant to display.
VORHER=$(hole "$BASIS/api/system/puls" | feld "['anfragen']['gesamt']")
hole "$BASIS/api/system/puls" >/dev/null
hole "$BASIS/api/system/puls" >/dev/null
pruefe "der Puls zaehlt sich nicht selbst" "$VORHER" \
       "$(hole "$BASIS/api/system/puls" | feld "['anfragen']['gesamt']")"
pruefe "ohne Anmeldung verschlossen" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' "$BASIS/api/system/puls")"

echo "== Gemeinsames Bearbeiten"
# Without a licence the wire stays shut, but the administration's view of it
# stays open: the switch in the panel should show something even when the add-on
# is not enabled, otherwise the page looks broken instead of closed.
pruefe "der Zustand ist abrufbar" "200" "$(code "$BASIS/api/system/mitschrift")"
pruefe "er sagt, dass die Lizenz fehlt" "False" \
       "$(hole "$BASIS/api/system/mitschrift" | feld "['lizenziert']")"
pruefe "eingeschaltet ist es trotzdem" "True" \
       "$(hole "$BASIS/api/system/mitschrift" | feld "['an']")"
pruefe "es sitzt niemand in einem Raum" "0" \
       "$(hole "$BASIS/api/system/mitschrift" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["raeume"]))')"
pruefe "ohne Anmeldung verschlossen" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' "$BASIS/api/system/mitschrift")"
# The wire itself and the number of people writing along hang on the add-on.
pruefe "die Leitung ist ohne Lizenz zu" "402" "$(code "$BASIS/api/echtzeit/$BREIT")"
pruefe "die Zahl der Mitschreibenden ist zu" "402" \
       "$(code "$BASIS/api/pages/$BREIT/mitschreibende")"
# And the page says so to the browser: without a licence there is no writing
# together, so it does not open a session for it either.
pruefe "die Seite meldet sich als nicht gemeinsam" "False" \
       "$(hole "$BASIS/api/pages/$BREIT" | feld "['gemeinsam']")"
# The switch is an ordinary setting and can be set.
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
# The actual proof is not that an archive comes out but that the result can be
# restored. Everything else is a claim.
pruefe "Umfang ist lesbar" "200" "$(code "$BASIS/api/system/sicherung/umfang")"
pruefe "die Instanz haelt sich fuer bereit" "True" \
       "$(hole "$BASIS/api/system/sicherung/umfang" | feld "['bereit']")"
# The state BEFORE the backup. Creating one records itself in the audit trail,
# namely after the dump has been pulled; a comparison afterwards would be off by
# exactly that one entry and would look like data loss.
declare -A VORHER
for T in pages users pruefspur attachments einstellungen; do
    VORHER[$T]=$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc "SELECT count(*) FROM $T")
done
curl -s -b "$KEKSE" "$BASIS/api/system/sicherung" -o "$ARBEIT/sicherung.zip"
pruefe "ein Archiv kam an" "True" \
       "$(python3 -c "import os;print(os.path.getsize('$ARBEIT/sicherung.zip')>1000)")"
pruefe "es ist ein gueltiges ZIP" "True" \
       "$(python3 -c "import zipfile;print(zipfile.is_zipfile('$ARBEIT/sicherung.zip'))")"
# The marker at the end. Without it an archive broken off midway would be
# indistinguishable from a complete one, because half a ZIP is a valid ZIP.
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
# The search index does NOT belong in it: such_tsv is a GENERATED column,
# PostgreSQL recomputes it on restore. If it stood in the dump, restoring would
# have failed at exactly this point.
pruefe "die Suchspalte steht als Vorschrift darin, nicht als Daten" "True" \
       "$(python3 -c "
import zipfile
z = zipfile.ZipFile('$ARBEIT/sicherung.zip')
d = next(n for n in z.namelist() if n.endswith('/datenbank.sql'))
t = z.read(d).decode('utf-8', 'replace')
vorschrift = 'GENERATED ALWAYS AS' in t
# such_tsv must not appear in any COPY column list. That is exactly the
# property that matters; guessing the column order would be a test that fails
# for no reason at the next ALTER TABLE.
inDaten = any(z.startswith('COPY public.') and 'such_tsv' in z.split(')')[0]
              for z in t.splitlines())
print(vorschrift and not inDaten)")"

echo "== Sicherung fuer ein Skript"
# The route for automation. A script has no cookie; without a password it must
# therefore get NOTHING, and with the right one everything.
pruefe "ohne Anmeldung und ohne Wort verschlossen" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' "$BASIS/api/system/sicherung")"
SWORT=$(hole -X POST "$BASIS/api/system/sicherung/token" | feld "['token']")
pruefe "ein Losungswort wurde erzeugt" "64" "$(printf '%s' "$SWORT" | wc -c | tr -d ' ')"
pruefe "damit geht es ohne Keks" "200" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $SWORT" "$BASIS/api/system/sicherung")"
pruefe "mit falschem Wort nicht" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer falsch' "$BASIS/api/system/sicherung")"
# The archive from the script route has to be the same as from the panel, and
# the request has to leave a trail. What is counted is the DIFFERENCE: the lines
# above have already fetched once, and a fixed total would depend on how often
# this section fetches something in between.
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
# A second database beside it, the dump into it, and count. Without this step a
# backup is a guess.
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
# And the search has to work again by itself in the restored database. psql
# writes booleans as t and f, not as True.
pruefe "die Suchspalte wurde beim Einspielen neu berechnet" "t" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d rueck -tAc \
          "SELECT count(*) > 0 FROM pages WHERE such_tsv IS NOT NULL")"
# And the search really has to bite on it, not merely be non-null. The
# comparison runs against every page's own title: a fixed search word would be a
# test depending on which pages the sections above happen to leave standing, and
# the wastebasket section removes one of them for good.
pruefe "und die Suche greift darauf" "t" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d rueck -tAc \
          "SELECT count(*) > 0 FROM pages
             WHERE title <> '' AND such_tsv @@ plainto_tsquery('german', title)")"
dropdb -h 127.0.0.1 -p "$PGPORT" -U nexora rueck

echo "== Sicherung wieder einspielen"
# The actual proof: back up, change something, restore, and the change has to
# be gone. Everything below -- marker, way back, restart -- hangs on that.
curl -s -b "$KEKSE" "$BASIS/api/system/sicherung" -o "$ARBEIT/stand.zip"
SEITEN_VORHER=$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc "SELECT count(*) FROM pages")
# A page that does NOT exist in the backup.
DANACH=$(hole -X POST "$BASIS/api/pages" -H 'Content-Type: application/json' \
         -d '{"title":"Nach der Sicherung entstanden"}' | feld "['id']")
pruefe "die neue Seite ist da" "200" "$(code "$BASIS/api/pages/$DANACH")"

# An archive without the marker has to be rejected. It is a valid ZIP, and that
# is precisely the trap: it could otherwise be restored and would lay half a
# body over a whole one.
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

# The service ends itself after the restore. In operation Docker restarts it;
# here the test has to do that.
sleep 3
halte_dienst_an
starte_dienst
pruefe "die Seite von nach der Sicherung ist weg" "404" "$(code "$BASIS/api/pages/$DANACH")"
pruefe "der Bestand entspricht wieder der Sicherung" "$SEITEN_VORHER" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc "SELECT count(*) FROM pages")"
pruefe "und die Anmeldung gilt noch" "200" "$(code "$BASIS/api/auth/me")"
# The search has to bite again by itself after the restore, without anybody
# rebuilding the index.
pruefe "die Suche greift ohne Zutun" "t" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc \
          "SELECT count(*) > 0 FROM pages WHERE title <> '' AND such_tsv @@ plainto_tsquery('german', title)")"

echo "== Verzeichnis-Verwaltung"
# An administrator may always look, even without a licence: otherwise an
# instance would not even see that something is set up there which is not
# running.
pruefe "Einrichtung ist lesbar" "200" "$(code "$BASIS/api/system/ldap")"
pruefe "hier ist nichts eingerichtet" "False" "$(hole "$BASIS/api/system/ldap" | feld "['aktiv']")"
pruefe "und nichts freigeschaltet" "False" "$(hole "$BASIS/api/system/ldap" | feld "['lizenziert']")"
# The filter is empty in the configuration and must not come out empty all the
# same: the same default applies as when signing in.
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
# The route accepts a body and throws it away. What is checked is that it really
# counts what arrives: the interface brackets in the limit using this answer, and
# a guessed number would be worse than none.
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
# Attachments, shares, comments and the export of a whole space are paid
# add-ons. Without a licence none of that can be run through here; what is
# checked is therefore that the routes are closed and do not answer with an
# error that looks like a defect.
pruefe "Anhänge sind zu" "402" \
       "$(curl -s -o /dev/null -w '%{http_code}' -b "$KEKSE" "$BASIS/api/pages/$BREIT/attachments")"
pruefe "Freigabe ist zu" "402" "$(code -X POST "$BASIS/api/pages/$BREIT/share")"
pruefe "die @-Liste ist zu" "402" "$(code "$BASIS/api/pages/$BREIT/erwaehnbare")"
pruefe "die Ausgabe einer Ablage ist zu" "402" "$(code "$BASIS/api/spaces/$ABL_ID/export")"
pruefe "der öffentliche Weg zu einer Datei ist zu" "402" \
       "$(curl -s -o /dev/null -w '%{http_code}' "$BASIS/api/public/egal/dateien/egal")"
# 402 and not 404: the route for replacing a marked-up PDF is registered, it is
# merely closed. A 404 would mean the route is missing -- and nobody would find
# that out before a licence is there.
pruefe "das Ersetzen einer markierten PDF ist zu" "402" \
       "$(curl -s -o /dev/null -w '%{http_code}' -X PUT -b "$KEKSE" \
          -H 'Content-Type: application/pdf' --data-binary '%PDF-1.4' \
          "$BASIS/api/pages/$BREIT/attachments/egal/pdf")"

echo "== Eigenes Profil"
SELBST_ID=$(hole "$BASIS/api/auth/me" | feld "['id']")
# Name and picture belong to the account itself, not to the administration. And
# the picture is checked by its CONTENT: what the browser claims as the type says
# nothing about what really arrives.
pruefe "anfangs kein Bild" "404" "$(code "$BASIS/api/users/$SELBST_ID/bild")"
pruefe "der Name laesst sich aendern" "Rauch Umbenannt" \
       "$(hole -X PUT "$BASIS/api/profil" -H 'Content-Type: application/json' \
          -d '{"name":"Rauch Umbenannt"}' | feld "['name']")"
pruefe "und steht sofort am Konto" "Rauch Umbenannt" "$(hole "$BASIS/api/auth/me" | feld "['name']")"
pruefe "ein leerer Name wird abgewiesen" "400" \
       "$(code -X PUT "$BASIS/api/profil" -H 'Content-Type: application/json' -d '{"name":"  "}')"

# A tiny real PNG, built by hand: one pixel is enough to check that detection
# goes by the content. It weighs 69 bytes -- and that is exactly what a
# well-meant lower bound in bytes once failed on.
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
# A renamed executable with an image's type must NOT get through: otherwise it
# would later lie in the row as image/png and would come back out that way
# too.
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
# Back to the old name, so the following sections find it again.
hole -X PUT "$BASIS/api/profil" -H 'Content-Type: application/json' \
     -d '{"name":"Rauch Test"}' >/dev/null

echo "== Passwort wechseln"
# The previous password is mandatory, even with an open session: otherwise an
# unattended browser would be enough to take over the account.
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
# One's own device stays signed in, everything else falls. Without this line the
# change would be a sign-out.
pruefe "dieses Gerät bleibt angemeldet" "200" "$(code "$BASIS/api/auth/me")"
pruefe "das alte Passwort öffnet nicht mehr" "401" \
       "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASIS/api/auth/login" \
          -H 'Content-Type: application/json' \
          -d '{"kennung":"rauch@test.invalid","password":"rauchtest-passwort"}')"
pruefe "das neue öffnet" "200" \
       "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASIS/api/auth/login" \
          -H 'Content-Type: application/json' \
          -d '{"kennung":"rauch@test.invalid","password":"zweites-passwort"}')"
# The audit trail is an add-on and not READABLE without a licence; it is
# written all the same, see main.go. The check therefore happens in the
# database.
pruefe "der Wechsel steht im Protokoll" "1" \
       "$(psql -h 127.0.0.1 -p "$PGPORT" -U nexora -d nexora -tAc \
          "SELECT count(*) FROM pruefspur WHERE aktion='konto.passwort'")"

# Reset by an administrator, on a second account.
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
# The service knocks on itself: an address that answers reliably in the smoke
# test without a second machine being involved.
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
# The service does not name itself in a Server header, so the column stays empty
# here -- nothing is guessed.
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
# And none of all this needs a Prometheus. That one is removed together with
# Grafana: no setting, no password, no route. It is checked here, because a
# removal otherwise quietly creeps back in.
pruefe "keine Prometheus-Einstellung mehr" "0" \
       "$(hole "$BASIS/api/einstellungen" | python3 -c '
import json, sys
schluessel = {e["schluessel"] for e in json.load(sys.stdin)}
print(len(schluessel & {"prometheus_adresse", "metriken_token"}))')"
pruefe "den Weg /metrics gibt es nicht mehr" "404" "$(code "$BASIS/metrics")"
pruefe "und seine Verwaltung auch nicht" "404" "$(code "$BASIS/api/system/metriken")"

echo "== Programme werden nicht angenommen"
# Attachments are an add-on and closed without a licence; what is checked is
# therefore the detection itself and the rejection on the route that is open: the
# import. An archive with an ELF file may create the page and not the file.
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
# The smoke test gets no further here: attachments are an add-on and are not
# even touched without a licence, so the executable already drops out one step
# earlier. That the four bytes at the beginning are recognised is checked by
# TestLinuxProgrammWirdErkannt in internal/handlers.
pruefe "ohne Lizenz kommt keine Beilage mit" "" \
       "$(printf '%s' "$EINFUHR" | python3 -c '
import json, sys
d = json.load(sys.stdin)
print(d.get("beilagen", ""))')"

echo "== Verschlüsselt sprechen"
# The service can do HTTPS itself. It is checked with a second start using a
# purpose-made certificate: that it accepts the file, that it really answers
# encrypted, and that a counterpart which knows the authority trusts it. That is
# exactly what the interface does inside the compound, see pki/erzeuge.sh.
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
# Whoever does not know the authority does not get through. Without this line
# the previous one would only prove that something answers.
pruefe "ohne die Stelle bleibt es zu" "000" \
       "$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$SICHER/healthz")"
# And nothing gets through unencrypted any more: to a request in the clear Go
# answers 400 with the remark that TLS is spoken here -- not with the page. So
# 400 is the desired result here and not an error.
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
