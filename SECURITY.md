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
| latest  | ✅        |

## Deployment Best Practices

### Required

- [ ] Set a strong `POSTGRES_PASSWORD` (≥ 24 random characters)
- [ ] Set a strong `JWT_SECRET` (≥ 32 random characters): `openssl rand -hex 32`
- [ ] Never commit `.env` to version control
- [ ] Confirm `.env` is actually being read

> **A missing `.env` does not stop the stack.** `docker-compose.yml` declares
> fallbacks (`${POSTGRES_PASSWORD:-nexora}`, `${JWT_SECRET:-change-me-in-production}`),
> so an instance without the file starts happily with a known database password and a
> publicly documented signing secret — and logs no warning about it. Anyone holding
> that secret can mint a session token for any account. Verify after deploying:
>
> ```bash
> docker compose exec backend printenv JWT_SECRET
> ```

### Recommended

- [ ] Terminate TLS in front of Nexora (reverse proxy with Let's Encrypt)
- [ ] Bind the exposed port to localhost (`127.0.0.1:3000:80`) if not public
- [ ] Regularly rebuild with fresh base images: `docker compose build --pull`
- [ ] Back up **both** volumes: `nexora_db` (pages, users, versions) and
      `nexora_files` (attachments). The database alone is not a complete backup —
      uploaded files live outside it, and restoring only `nexora_db` leaves every
      attachment row pointing at a file that is gone

## Notes

- Passwords are hashed with bcrypt (cost 12); sessions use signed JWTs in
  httpOnly cookies.
- All containers run as non-root users on Alpine base images.
- When serving over HTTPS, the session cookie should be marked `Secure`
  (set via the reverse proxy / a future config flag).
