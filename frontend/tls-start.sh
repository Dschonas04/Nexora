#!/bin/sh
# Creates a certificate on start when none is there, then starts nginx.
#
# Self signed, on purpose: Nexora runs in one's own network under an IP address,
# and no certificate authority issues anything for that. The browser will
# therefore warn, and that is the price. The connection is encrypted all the
# same, and that is the point here: the session cookie and the passwords shall
# not travel over the network in the clear.
#
# Whoever has a real certificate mounts it under /etc/nginx/tls (zertifikat.pem
# and schluessel.pem). Then nothing is generated.
set -eu

VERZ=/etc/nginx/tls
ZERT="$VERZ/zertifikat.pem"
SCHLUESSEL="$VERZ/schluessel.pem"

mkdir -p "$VERZ"

if [ ! -f "$ZERT" ] || [ ! -f "$SCHLUESSEL" ]; then
    echo "TLS: kein Zertifikat gefunden, es wird eines erzeugt"
    # The name comes from NEXORA_TLS_NAME, failing that the host name. The usual
    # local names and addresses go in as alternative names as well: without them
    # the browser complains not only about the issuer but about the name on top
    # of it, two warnings instead of one.
    NAME="${NEXORA_TLS_NAME:-$(hostname)}"
    openssl req -x509 -newkey rsa:2048 -sha256 -days 825 -nodes \
        -keyout "$SCHLUESSEL" -out "$ZERT" \
        -subj "/CN=$NAME" \
        -addext "subjectAltName=DNS:$NAME,DNS:localhost,IP:127.0.0.1${NEXORA_TLS_IP:+,IP:$NEXORA_TLS_IP}" \
        >/dev/null 2>&1
    chmod 600 "$SCHLUESSEL"
    echo "TLS: Zertifikat für $NAME erzeugt, gültig 825 Tage"
else
    echo "TLS: vorhandenes Zertifikat wird benutzt"
fi

# ── Wie der Dienst dahinter angesprochen wird ───────────────────────────────
#
# In der Vorlage stehen zwei Platzhalter, hier werden sie eingesetzt. Nur diese
# zwei: nginx hat selbst Variablen in derselben Schreibweise ($host,
# $request_uri), und ein envsubst ohne Liste fräse sie alle weg.
#
# Vorgabe ist verschlüsselt. Wer den Dienst ohne Zertifikat betreibt, setzt
# NEXORA_DIENST_SCHEMA=http und NEXORA_DIENST_PORT=8080.
export NEXORA_DIENST_SCHEMA="${NEXORA_DIENST_SCHEMA:-https}"
export NEXORA_DIENST_PORT="${NEXORA_DIENST_PORT:-8443}"

if [ -f /etc/nginx/vorlage.conf ]; then
    envsubst '${NEXORA_DIENST_SCHEMA} ${NEXORA_DIENST_PORT}' \
        < /etc/nginx/vorlage.conf > /etc/nginx/conf.d/default.conf
    echo "Dienst: $NEXORA_DIENST_SCHEMA://backend:$NEXORA_DIENST_PORT"
fi

# Ohne die Stelle des Verbunds käme nginx nicht an den Dienst heran, und die
# Meldung dazu stünde erst bei der ersten Anfrage im Protokoll. Lieber hier
# sagen, woran es liegt.
if [ "$NEXORA_DIENST_SCHEMA" = "https" ] && [ ! -f /pki/ca.crt ]; then
    echo "ACHTUNG: /pki/ca.crt fehlt. Der Dienst ist verschlüsselt eingestellt," >&2
    echo "         aber ohne die Stelle lässt sich sein Zertifikat nicht prüfen." >&2
fi

exec nginx -g 'daemon off;'
