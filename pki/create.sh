#!/bin/sh
# Certificates for traffic inside the compound.
#
# Runs once at start-up, before all other services, and puts a small private
# certificate authority into a shared volume plus one certificate per service
# below it. After that interface, service, database, object store and cache
# speak to each other encrypted.
#
# Why private ones and not bought ones: the names here are "backend", "db",
# "minio" -- names from the compound's network that do not exist outside it. No
# certificate authority in the world issues anything for those, and none should:
# these connections never leave the machine.
#
# Why at all, when the traffic does not leave the machine: because it does, as
# soon as somebody puts the database on a second machine or spans the Docker
# network across several hosts. And because a listener on the same network would
# otherwise see password hashes, session keys and every page's content go by in
# the clear. The interface's certificate facing outwards is another matter, that
# lies in frontend/tls-start.sh.
#
# The run is repeatable: whatever is already there stays. A certificate changing
# on every start-up would be no gain but a debugging session.
set -eu

VERZ=${PKI_VERZ:-/pki}
TAGE=${PKI_TAGE:-3650}
# The names the services are reachable under inside the compound. Whoever names
# their services differently sets PKI_DIENSTE.
DIENSTE=${PKI_DIENSTE:-"backend db minio redis"}

mkdir -p "$VERZ"
cd "$VERZ"

# Every service looks for its files where it looks for them. MinIO insists on
# public.crt and private.key, the others take whatever they are told -- so they
# are named after the service.
zert_name() {
    if [ "$1" = "minio" ]; then echo "public.crt"; else echo "$1.crt"; fi
}
schluessel_name() {
    if [ "$1" = "minio" ]; then echo "private.key"; else echo "$1.key"; fi
}

# The uid the service may later read under: a private key everybody can read is
# none. PostgreSQL even refuses to start when its own lies open too widely, and
# rightly so.
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
    # The authority's key is none of the services' business. It is only needed
    # here, when issuing.
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
        # localhost and 127.0.0.1 are in there too, so that a service can also
        # check itself -- a readiness probe, say, running inside its own
        # container against its own address.
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

    # MinIO looks for the foreign authorities in a subdirectory, otherwise it
    # does not trust its own address when testing itself.
    if [ "$dienst" = "minio" ]; then
        mkdir -p minio/CAs
        cp -f ca.crt minio/CAs/nexora.crt
    fi

    # Permissions and ownership are set on every run and not only when issuing:
    # a volume somebody has touched by hand should right itself again at the
    # next start-up.
    # If setting the ownership fails -- because this container runs without root,
    # say -- that is a notice and not an abort: the compound should come up and
    # say what is missing instead of standing there mute.
    chown -R "$(kennung_fuer "$dienst")" "$dienst" 2>/dev/null ||
        echo "PKI: Kennung für $dienst nicht setzbar, $dienst liest seinen Schlüssel womöglich nicht"
    chmod 700 "$dienst"
    chmod 600 "$schluessel"
    chmod 644 "$zert"
done

echo "PKI: fertig, $(echo "$DIENSTE" | wc -w) Zertifikate und eine Stelle liegen in $VERZ."
