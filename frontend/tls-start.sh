#!/bin/sh
# Legt beim Start ein Zertifikat an, falls keines liegt, und startet dann nginx.
#
# Selbst ausgestellt, mit Absicht: Nexora läuft im eigenen Netz unter einer
# IP-Adresse, und dafür stellt keine Zertifizierungsstelle etwas aus. Der Browser
# wird also warnen, das ist der Preis. Verschlüsselt ist die Verbindung
# trotzdem, und darum geht es hier: das Sitzungsplätzchen und die Passwörter
# sollen nicht im Klartext über das Netz gehen.
#
# Wer ein echtes Zertifikat hat, hängt es unter /etc/nginx/tls ein
# (zertifikat.pem und schluessel.pem). Dann wird nichts erzeugt.
set -eu

VERZ=/etc/nginx/tls
ZERT="$VERZ/zertifikat.pem"
SCHLUESSEL="$VERZ/schluessel.pem"

mkdir -p "$VERZ"

if [ ! -f "$ZERT" ] || [ ! -f "$SCHLUESSEL" ]; then
    echo "TLS: kein Zertifikat gefunden, es wird eines erzeugt"
    # Der Name steht in NEXORA_TLS_NAME, sonst der Rechnername. Zusätzlich
    # kommen die üblichen lokalen Namen und Adressen als Alternativnamen
    # hinein: ohne sie meckert der Browser nicht nur über den Aussteller,
    # sondern zusätzlich über den Namen, zwei Warnungen statt einer.
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
