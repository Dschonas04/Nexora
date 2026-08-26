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

exec nginx -g 'daemon off;'
