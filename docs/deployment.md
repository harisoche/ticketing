# Deployment Readiness Checklist

Platform-neutral. This document is purposely opinionated about what *not*
to do — the educational build is single-instance and stores uploads on
local disk.

## Configuration

- [ ] `JWT_SECRET` is a fresh, random value ≥ 32 chars. Do **not** reuse the
      example string. A safe shell incantation:
      `head -c 48 /dev/urandom | base64`.
- [ ] `JWT_ACCESS_TOKEN_TTL` matches the operational reality (the token is
      bearer-only; rotating it is fast but cannot be revoked outside of
      logout).
- [ ] `DATABASE_URL` points at a managed Postgres instance (RDS, Cloud SQL,
      Supabase, …) or a secured self-hosted Postgres. Do not run on the
      same host long-term.
- [ ] `UPLOAD_STORAGE_DRIVER=local` is only appropriate for a single
      persistent instance. See "Scaling beyond one instance" below.
- [ ] `APP_ENV=production` (or your environment label). The application
      doesn't fork on env, but log aggregators key off the value.
- [ ] Logging is captured at INFO. Echo's default request log is JSON-line
      so it ships cleanly into Elastic / Loki / CloudWatch.

## Migrations & seeds

- [ ] Run migrations **before** serving traffic:
      `./migrate up` (or `docker compose run --rm api migrate up`).
- [ ] Do **not** run `./seed` in production. The seed command writes a
      well-known password and is documented "DO NOT use in production".

## Network

- [ ] The API serves plain HTTP on `APP_PORT`. Always front it with TLS:
      reverse proxy (Nginx / Caddy / Cloudflare) or platform load balancer.
- [ ] Configure CORS at the proxy if the mobile client lives on a
      different origin. Phase 8 does not bake in a CORS policy because the
      shape depends on the deployment.

## Database

- [ ] Backups are enabled at the database provider (PITR or daily
      snapshots).
- [ ] Connection pooling is bounded. The Go app uses GORM's default pool
      sized in `internal/infrastructure/database/postgres.go`
      (20 open / 10 idle / 30-minute lifetime).

## Uploads

Local-disk storage is the educational default. Operationally:

- [ ] The `UPLOAD_LOCAL_DIRECTORY` lives on a *persistent* volume — not the
      container's ephemeral filesystem. The compose file mounts a named
      volume; on Kubernetes use a `PersistentVolumeClaim`.
- [ ] Disk usage and inode usage are monitored. Phase 5 enforces a 5 MiB
      per-file cap but no global quota.
- [ ] The upload directory is **not** served by any reverse proxy. All
      downloads go through `/api/v1/tickets/:id/attachments/:id/download`
      which enforces authentication + ticket access.

## Secrets & logs

- [ ] Container logs are scrubbed of bearer tokens. Echo's default logger
      logs the URL and status — it does not log the `Authorization` header
      or request bodies.
- [ ] DB connection strings, JWT secrets, and `SEED_ADMIN_PASSWORD` are
      injected from a secret manager (AWS Secrets Manager, Doppler, sops,
      Kubernetes secrets), **not** baked into the image or shell history.
- [ ] Uploaded file *contents* are never logged.

## Identity

- [ ] Production admin / agent accounts are provisioned through the
      regular `POST /auth/register` flow plus
      `UPDATE users SET role='admin' WHERE email='…';` (or an equivalent
      migration / one-off script). The seed accounts are explicitly for
      development.
- [ ] Periodically rotate admin passwords using the regular auth flow.

## Smoke checks

Pre-deploy, run from a workstation with network access to the production
host:

```bash
make test                              # unit + handler
make vet                               # static analysis
gofmt -d .                             # zero diff = OK
DATABASE_URL=$PROD_DSN go run ./cmd/migrate status
```

After deploy, run a quick smoke flow against the production base URL:

```bash
curl -sS $BASE_URL/health
curl -sS -X POST $BASE_URL/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@your-org.example","password":"<from-vault>"}' \
  | jq '.data.access_token | length'
```

## Scaling beyond one instance

When you outgrow a single-node deploy, replace the local file storage
driver with cloud object storage. The `FileStorage` interface in
`internal/domain/service/file_storage.go` was deliberately small — an
S3-compatible implementation is a future advanced exercise. Until then,
horizontal scaling is not supported because attachment uploads would land
on whichever node served the request.

## What's intentionally out of scope

- Background workers, message brokers, cron schedulers.
- WebSockets, push notifications, external email.
- Kubernetes manifests, Terraform.
- Refresh tokens, password reset, email verification.

Those land in advanced exercises after Phase 8.
