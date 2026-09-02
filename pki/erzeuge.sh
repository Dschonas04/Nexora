#!/bin/sh
# Zertifikate für den Innenverkehr des Verbunds.
#
# Läuft einmal beim Hochfahren, vor allen anderen Diensten, und legt in einem
# gemeinsamen Datenträger eine kleine eigene Zertifizierungsstelle ab und für
# jeden Dienst ein Zertifikat darunter. Danach sprechen Oberfläche, Dienst,
# Datenbank, Ablage und Zwischenspeicher untereinander verschlüsselt.
#
# Warum eigene und keine gekauften: die Namen hier heißen "backend", "db",
# "minio" -- Namen aus dem Netz des Verbunds, die es außerhalb nicht gibt. Dafür
# stellt keine Zertifizierungsstelle der Welt etwas aus, und sie soll es auch
# nicht: diese Verbindungen verlassen den Rechner nie.
#
# Warum überhaupt, wenn der Verkehr den Rechner nicht verlässt: weil er es doch
# tut, sobald jemand die Datenbank auf einen zweiten Rechner legt oder das
# Docker-Netz über mehrere Wirte spannt. Und weil ein Mitleser im selben Netz
# sonst Passwort-Hashes, Sitzungsschlüssel und jeden Seiteninhalt im Klartext
# vorbeiziehen sieht. Das Zertifikat der Oberfläche nach außen ist eine andere
# Sache, das liegt in frontend/tls-start.sh.
#
# Der Lauf ist wiederholbar: was schon da ist, bleibt. Ein Zertifikat, das sich
# bei jedem Hochfahren ändert, wäre kein Gewinn, sondern eine Fehlersuche.
set -eu

VERZ=${PKI_VERZ:-/pki}
TAGE=${PKI_TAGE:-3650}
# Die Namen, unter denen die Dienste im Verbund erreichbar sind. Wer seine
# Dienste anders nennt, setzt PKI_DIENSTE.
DIENSTE=${PKI_DIENSTE:-"backend db minio redis"}

mkdir -p "$VERZ"
cd "$VERZ"

# Jeder Dienst sucht seine Dateien dort, wo er sie sucht. MinIO besteht auf
# public.crt und private.key, die übrigen nehmen, was man ihnen nennt -- also
# heißen sie wie der Dienst.
zert_name() {
    if [ "$1" = "minio" ]; then echo "public.crt"; else echo "$1.crt"; fi
}
schluessel_name() {
    if [ "$1" = "minio" ]; then echo "private.key"; else echo "$1.key"; fi
}

# Die Kennung, unter der der Dienst später lesen darf: ein privater Schlüssel,
# den jeder lesen kann, ist keiner. PostgreSQL verweigert sogar den Start, wenn
# seiner zu weit offen liegt, und das zu Recht.
kennung_fuer() {
    case "$1" in
        db)      echo "70:70" ;;       # postgres im Alpine-Abbild
        redis)   echo "999:1000" ;;    # redis im Alpine-Abbild
        backend) echo "10001:10001" ;; # nexora, siehe backend/Dockerfile
        *)       echo "0:0" ;;         # minio und alles Weitere läuft als root
    esac
}

# ── Die Zertifizierungsstelle ───────────────────────────────────────────────
if [ ! -f ca.crt ] || [ ! -f ca.key ]; then
    echo "PKI: lege eine eigene Zertifizierungsstelle an"
    openssl req -x509 -newkey rsa:4096 -sha256 -days "$TAGE" -nodes \
        -keyout ca.key -out ca.crt \
        -subj "/CN=Nexora interne Stelle" \
        -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
        -addext "keyUsage=critical,keyCertSign,cRLSign" \
        >/dev/null 2>&1
    # Der Schlüssel der Stelle geht keinen der Dienste etwas an. Er wird nur
    # hier gebraucht, beim Ausstellen.
    chmod 600 ca.key
    chmod 644 ca.crt
else
    echo "PKI: vorhandene Zertifizierungsstelle wird benutzt"
fi

# ── Je Dienst ein Zertifikat ────────────────────────────────────────────────
for dienst in $DIENSTE; do
    mkdir -p "$dienst"
    zert="$dienst/$(zert_name "$dienst")"
    schluessel="$dienst/$(schluessel_name "$dienst")"

    if [ -f "$zert" ] && [ -f "$schluessel" ]; then
        echo "PKI: $dienst hat bereits ein Zertifikat"
    else
        echo "PKI: stelle ein Zertifikat für $dienst aus"
        # localhost und 127.0.0.1 stehen mit drin, damit ein Dienst sich auch
        # selbst prüfen kann -- etwa eine Bereitschaftsprobe, die im eigenen
        # Container gegen die eigene Adresse läuft.
        openssl req -newkey rsa:2048 -sha256 -nodes \
            -keyout "$schluessel" -out "$dienst/anfrage.csr" \
            -subj "/CN=$dienst" >/dev/null 2>&1
        openssl x509 -req -in "$dienst/anfrage.csr" -days "$TAGE" -sha256 \
            -CA ca.crt -CAkey ca.key -CAcreateserial -out "$zert" \
            -extfile /dev/stdin >/dev/null 2>&1 <<ERWEITERUNG
subjectAltName = DNS:$dienst, DNS:localhost, IP:127.0.0.1
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
ERWEITERUNG
        rm -f "$dienst/anfrage.csr"
    fi

    # MinIO sucht die fremden Stellen in einem Unterverzeichnis, sonst traut es
    # beim Selbstversuch der eigenen Adresse nicht.
    if [ "$dienst" = "minio" ]; then
        mkdir -p minio/CAs
        cp -f ca.crt minio/CAs/nexora.crt
    fi

    # Rechte und Kennung werden bei jedem Lauf gesetzt und nicht nur beim
    # Ausstellen: ein Datenträger, den jemand von Hand angefasst hat, soll sich
    # beim nächsten Hochfahren von selbst wieder einrenken.
    # Schlägt das Setzen der Kennung fehl -- etwa weil dieser Container ohne
    # root läuft --, ist das ein Hinweis und kein Abbruch: der Verbund soll
    # hochkommen und sagen, was fehlt, statt stumm stehenzubleiben.
    chown -R "$(kennung_fuer "$dienst")" "$dienst" 2>/dev/null ||
        echo "PKI: Kennung für $dienst nicht setzbar, $dienst liest seinen Schlüssel womöglich nicht"
    chmod 700 "$dienst"
    chmod 600 "$schluessel"
    chmod 644 "$zert"
done

echo "PKI: fertig, $(echo "$DIENSTE" | wc -w) Zertifikate und eine Stelle liegen in $VERZ."
