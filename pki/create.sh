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

# PKI_VERZ, PKI_TAGE and PKI_DIENSTE were the earlier names and are still read,
# so an existing setup keeps working.
DIR=${PKI_DIR:-${PKI_VERZ:-/pki}}
DAYS=${PKI_DAYS:-${PKI_TAGE:-3650}}
# The names the services are reachable under inside the compound. Whoever names
# their services differently sets PKI_SERVICES.
SERVICES=${PKI_SERVICES:-${PKI_DIENSTE:-"backend db minio redis"}}

mkdir -p "$DIR"
cd "$DIR"

# Every service looks for its files where it looks for them. MinIO insists on
# public.crt and private.key, the others take whatever they are told -- so they
# are named after the service.
cert_name() {
    if [ "$1" = "minio" ]; then echo "public.crt"; else echo "$1.crt"; fi
}
key_name() {
    if [ "$1" = "minio" ]; then echo "private.key"; else echo "$1.key"; fi
}

# The uid the service may later read under: a private key everybody can read is
# none. PostgreSQL even refuses to start when its own lies open too widely, and
# rightly so.
owner_for() {
    case "$1" in
        db)      echo "70:70" ;;       # postgres in the alpine image
        redis)   echo "999:1000" ;;    # redis in the alpine image
        backend) echo "10001:10001" ;; # nexora, see backend/Dockerfile
        *)       echo "0:0" ;;         # minio and everything else runs as root
    esac
}

# -- The certificate authority ----------------------------------------------
if [ ! -f ca.crt ] || [ ! -f ca.key ]; then
    echo "PKI: setting up a certificate authority of our own"
    openssl req -x509 -newkey rsa:4096 -sha256 -days "$DAYS" -nodes \
        -keyout ca.key -out ca.crt \
        -subj "/CN=Nexora internal authority" \
        -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
        -addext "keyUsage=critical,keyCertSign,cRLSign" \
        >/dev/null 2>&1
    # The authority's key is none of the services' business. It is only needed
    # here, when issuing.
    chmod 600 ca.key
    chmod 644 ca.crt
else
    echo "PKI: using the authority that is already there"
fi

# -- One certificate per service --------------------------------------------
for service in $SERVICES; do
    mkdir -p "$service"
    cert="$service/$(cert_name "$service")"
    key="$service/$(key_name "$service")"

    if [ -f "$cert" ] && [ -f "$key" ]; then
        echo "PKI: $service already has a certificate"
    else
        echo "PKI: issuing a certificate for $service"
        # localhost and 127.0.0.1 are in there too, so that a service can also
        # check itself -- a readiness probe, say, running inside its own
        # container against its own address.
        openssl req -newkey rsa:2048 -sha256 -nodes \
            -keyout "$key" -out "$service/request.csr" \
            -subj "/CN=$service" >/dev/null 2>&1
        openssl x509 -req -in "$service/request.csr" -days "$DAYS" -sha256 \
            -CA ca.crt -CAkey ca.key -CAcreateserial -out "$cert" \
            -extfile /dev/stdin >/dev/null 2>&1 <<EXTENSIONS
subjectAltName = DNS:$service, DNS:localhost, IP:127.0.0.1
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
EXTENSIONS
        rm -f "$service/request.csr"
    fi

    # MinIO looks for the foreign authorities in a subdirectory, otherwise it
    # does not trust its own address when testing itself.
    if [ "$service" = "minio" ]; then
        mkdir -p minio/CAs
        cp -f ca.crt minio/CAs/nexora.crt
    fi

    # Permissions and ownership are set on every run and not only when issuing:
    # a volume somebody has touched by hand should right itself again at the
    # next start-up.
    # If setting the ownership fails -- because this container runs without root,
    # say -- that is a notice and not an abort: the compound should come up and
    # say what is missing instead of standing there mute.
    chown -R "$(owner_for "$service")" "$service" 2>/dev/null ||
        echo "PKI: cannot set the owner of $service, it may not be able to read its key"
    chmod 700 "$service"
    chmod 600 "$key"
    chmod 644 "$cert"
done

echo "PKI: done, $(echo "$SERVICES" | wc -w) certificates and one authority are in $DIR."
