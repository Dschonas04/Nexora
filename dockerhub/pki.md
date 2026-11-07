# Nexora: internal certificates (deprecated)

> **This image is deprecated and no longer maintained.** Since Nexora 3.2.0 the
> internal certificates come from the backend itself: the compose files start
> `nexora-backend` once with the command `pki`, it sets up the authority and the
> certificates, ends with exit code 0 and does not run again until the next
> start. The last release of this image is 3.1.2; it receives no updates.

## What to do

Nothing changes in the data: the `nexora_pki` volume keeps its authority and
keys, `nexora pki` reads what is there and only adds what is missing.

When you update to 3.2.0 or later, take the current compose file. Its `pki`
service looks like this:

```yaml
pki:
  image: ghcr.io/dschonas04/nexora-backend:${NEXORA_VERSION:-latest}
  command: ["pki"]
  user: "0:0"
  healthcheck:
    disable: true
  volumes:
    - nexora_pki:/pki
  restart: "no"
```

`docker ps -a` then shows the container `pki` as `Exited (0)`. That is the
intended state: it has done its work and stays stopped.

## Links

- Compose files: <https://github.com/Dschonas04/Nexora/tree/main/docker>
- Changelog 3.2.0: <https://github.com/Dschonas04/Nexora/blob/main/CHANGELOG.md>
- Backend image: [`dschohnas/nexora-backend`](https://hub.docker.com/r/dschohnas/nexora-backend)
