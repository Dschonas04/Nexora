# Nexora on Kubernetes

Plain manifests for k3s or any other cluster, applied with kustomize:
PostgreSQL, backend, frontend and an ingress.

## Install

```bash
./install.sh wiki.example.org
```

writes `secret.env` with random values (database password, session key),
applies everything into the namespace `nexora` with the ingress on the given
host name, and waits until the database, the backend and the frontend are
ready. The first account you register becomes the administrator.

| Variable | Default | |
|---|---|---|
| `NEXORA_NAMESPACE` | `nexora` | Namespace to install into |
| `NEXORA_VERSION` | `latest` | Image tag, e.g. `3.1.0` |

```bash
NEXORA_NAMESPACE=wiki NEXORA_VERSION=3.1.0 ./install.sh wiki.example.org
```

Keep `secret.env` (it is not committed): PostgreSQL fixes its password at the
first start. **Update:** run `./install.sh` again with the same arguments.

Without an ingress controller:

```bash
kubectl -n nexora port-forward svc/frontend 8080:80     # http://localhost:8080
```

Needs kubectl with access to the cluster and a default storage class for the
two volumes (10 GiB each). Tested on k3s with local-path and Traefik.

## By hand

```bash
cp secret.env.example secret.env      # POSTGRES_PASSWORD, DATABASE_URL, JWT_SECRET
$EDITOR ingress.yaml                  # the host name, TLS
kubectl apply -k .
```

## The files

| File | What it is |
|---|---|
| `install.sh` | Writes `secret.env`, applies with namespace, host and version, waits |
| `kustomization.yaml` | The list, the secret from `secret.env`, the image tags |
| `namespace.yaml` | Namespace `nexora` |
| `postgres.yaml` | PostgreSQL 18 as a StatefulSet with its own volume |
| `pvc.yaml` | The volume for the attachments |
| `backend.yaml` | The Go service on 8080 |
| `frontend.yaml` | The interface on 80 |
| `ingress.yaml` | The way in from outside; TLS commented in |

**A database of your own:** remove `postgres.yaml` from `kustomization.yaml`
and point `DATABASE_URL` in `secret.env` at it.

**TLS:** uncomment the `tls:` block in `ingress.yaml` and point it at a
certificate secret, by hand or through cert-manager.

More: [operations](../docs/operations.md#kubernetes).
