#!/bin/sh
# Installs Nexora on Kubernetes: database, backend, frontend and ingress.
#
#     ./install.sh                       host name nexora.example.org
#     ./install.sh wiki.example.org      the host name the ingress answers to
#     NEXORA_NAMESPACE=wiki ./install.sh wiki.example.org
#     NEXORA_VERSION=3.1.0 ./install.sh  a fixed version instead of latest
#
# On the first run it writes secret.env with random passwords; later runs keep
# it and apply the manifests again -- which is also how an update is done.
# Needs kubectl with access to the cluster, and a default storage class for the
# two volumes (attachments and database).
set -eu

hier=$(cd "$(dirname "$0")" && pwd)
cd "$hier"

say() { printf '%s\n' "$*"; }
fail() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

command -v kubectl >/dev/null 2>&1 || fail "kubectl is not installed."
kubectl version --request-timeout=10s >/dev/null 2>&1 || fail "kubectl reaches no cluster (check KUBECONFIG)."

host=${1:-nexora.example.org}
ns=${NEXORA_NAMESPACE:-nexora}
version=${NEXORA_VERSION:-latest}

zufall() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    od -An -tx1 -N32 /dev/urandom | tr -d ' \n'
  fi
}

if [ -f secret.env ]; then
  say "Keeping the existing secret.env."
else
  pw=$(zufall)
  umask 077
  cat > secret.env <<EOF
POSTGRES_PASSWORD=$pw
DATABASE_URL=postgres://nexora:$pw@postgres:5432/nexora?sslmode=disable
JWT_SECRET=$(zufall)
EOF
  say "Wrote secret.env with random passwords (keep it: without it the database stays locked)."
fi

# Namespace, host name and version go in through an overlay, so the files here
# stay as they are in the repository. The base is rendered into the overlay's
# own directory first: kustomize accepts neither an absolute path to it nor a
# directory that contains the overlay.
overlay=$(mktemp -d)
trap 'rm -rf "$overlay"' EXIT
kubectl kustomize "$hier" > "$overlay/basis.yaml"
cat > "$overlay/kustomization.yaml" <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: $ns
resources:
  - basis.yaml
images:
  - name: ghcr.io/dschonas04/nexora-backend
    newTag: "$version"
  - name: ghcr.io/dschonas04/nexora-frontend
    newTag: "$version"
patches:
  - target:
      kind: Ingress
      name: nexora
    patch: |-
      - op: replace
        path: /spec/rules/0/host
        value: "$host"
  - target:
      kind: Namespace
      name: nexora
    patch: |-
      - op: replace
        path: /metadata/name
        value: "$ns"
EOF

say "Applying to namespace $ns…"
kubectl apply -k "$overlay"

say "Waiting for the database, the backend and the frontend…"
kubectl -n "$ns" rollout status statefulset/postgres --timeout=300s
kubectl -n "$ns" rollout status deployment/backend --timeout=300s
kubectl -n "$ns" rollout status deployment/frontend --timeout=300s

say ""
say "Nexora is running in namespace $ns."
say "  Through the ingress:   http://$host  (TLS: see ingress.yaml)"
say "  Without an ingress:    kubectl -n $ns port-forward svc/frontend 8080:80"
say "                         then http://localhost:8080"
say "The first account you register becomes the administrator."
