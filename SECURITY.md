# Security Policy

## Reporting Vulnerabilities

If you discover a security vulnerability, **please do not open a public issue**.

Instead, report it privately:

1. Contact via [GitHub](https://github.com/Dschonas04)
2. Or use GitHub's [private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)

Please include a description, steps to reproduce, and potential impact.

## Supported Versions

| Version | Supported |
| ------- | --------- |
| latest  | yes       |

## Deployment Best Practices

### Required

- [ ] Set a strong `POSTGRES_PASSWORD` (≥ 24 random characters)
- [ ] Set a strong `JWT_SECRET` (≥ 32 random characters): `openssl rand -hex 32`
- [ ] Never commit `.env` to version control
- [ ] Confirm `.env` is actually being read
- [ ] Read the startup log: dangerous defaults are named there
- [ ] Turn `registrierung_offen` off once the accounts that should exist do —
      but not before the first one, which becomes the administrator
- [ ] With LDAP: leave `ldap_starttls` and `ldap_tls_pruefen` on. Without them
      directory credentials cross the network in the clear

> **A missing `.env` does not stop the stack.** `docker-compose.yml` declares
> fallbacks (`${POSTGRES_PASSWORD:-nexora}`, `${JWT_SECRET:-change-me-in-production}`),
> so an instance without the file starts happily with a known database password and a
> publicly documented signing secret, and logs no warning about it. Anyone holding
> that secret can mint a session token for any account. Verify after deploying:
>
> ```bash
> docker compose exec backend printenv JWT_SECRET
> ```
>
> Since the configuration file landed, the server also says so itself on every
> boot:
>
> ```
> ACHTUNG: jwt_geheimnis steht auf der Vorgabe -- jede Sitzung ist fälschbar
> ```

### Recommended

- [ ] Terminate TLS in front of Nexora (reverse proxy with Let's Encrypt)
- [ ] Bind the exposed port to localhost (`127.0.0.1:3000:80`) if not public
- [ ] Regularly rebuild with fresh base images: `docker compose build --pull`
- [ ] Back up **both** volumes: `nexora_db` (pages, users, versions) and
      `nexora_files` (attachments). The database alone is not a complete backup:
      uploaded files live outside it, and restoring only `nexora_db` leaves every
      attachment row pointing at a file that is gone

## The license signing key

If you issue license keys for your own build, the private Ed25519 key is the
single most sensitive secret in the project. It exists once, it never belongs in
the repository, and because verification is offline, **a key signed with it can
never be withdrawn**. Give issued keys an expiry date; that is the only lever
there is.

Replacing the public key in `backend/premium/lizenz/pruefer.go` invalidates every
key ever issued — which is the emergency exit, and the reason it is a constant
rather than a setting.

## Notes

- Passwords are hashed with bcrypt (cost 12); sessions use signed JWTs in
  httpOnly cookies.
- The audit trail records sign-ins including failed ones, with the address that
  was tried. Passwords are never recorded, not even on a failed attempt.
- All containers run as non-root users on Alpine base images.
- When serving over HTTPS, the session cookie should be marked `Secure`
  (set via the reverse proxy / a future config flag).
