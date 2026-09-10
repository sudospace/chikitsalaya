# Chikitsalaya

A lightweight, self-hosted outpatient (OPD) clinic management system — patients,
practitioners, appointment scheduling (in-person and online), consultations,
prescriptions, lab/procedure orders, and printable prescriptions. Built for a single
clinic per deployment, with per-module view/edit permissions and mandatory 2FA on every
account.

Single Go binary + Postgres. No build step for the frontend — server-rendered HTML
styled with [Oat](https://oat.ink), live bits handled with [htmx](https://htmx.org).

## Stack

| | |
|---|---|
| Backend | Go, `chi` router, `pgx/v5` |
| Database | PostgreSQL 16, migrations via `goose` (embedded, run automatically on boot) |
| Sessions | `alexedwards/scs`, Postgres-backed |
| Frontend | Server-rendered `html/template` + Oat (vendored) + htmx — no JS build step |
| Auth | bcrypt passwords + mandatory TOTP 2FA on every account |

## Quick start (Docker — no local Go toolchain needed)

```bash
make docker-up          # builds the image, starts Postgres + the app
make seed-admin EMAIL=you@clinic.test PASSWORD=some-strong-password
```

Open **http://localhost:8090**, log in with that email/password, and follow the setup
guide in [`Onboarding/`](./Onboarding).

`make docker-down` stops everything. `make docker-logs` tails the app container.

### Local development (Go installed, Postgres via Docker)

```bash
make up                 # starts just Postgres (host port 5442)
cp .env.example .env    # PORT + DATABASE_URL for local dev
make dev                # go run ./cmd/chikitsalaya — migrates + serves on :8090
```

Bootstrap the clinic and its first admin locally (no `docker compose exec` needed):

```bash
go run ./cmd/chikitsalaya --seed-admin --admin-email=you@clinic.test --admin-password=some-strong-password
```

This single command creates the clinic and the first `admin` login in one shot — there's
no separate "create a company" step.

## Roles, at a glance

| Role | Created by |
|---|---|
| `admin` | CLI (`--seed-admin`) for the first one; any existing admin after that |
| `doctor` | Any admin |
| `receptionist` | Any admin |

An admin can also link their own login to a practitioner record and switch into that
doctor's view — see [`Onboarding/03-admin-setup.md`](./Onboarding/03-admin-setup.md).

## Project layout

```
cmd/chikitsalaya/       entrypoint — wires config, DB, and every module's routes
internal/
  auth/                 users, roles, sessions, mandatory 2FA, permissions
  config/ db/ web/      shared infra (env config, migrations/pool, template rendering)
  opd/                  the OPD module — one package per concern:
    company/ patient/ practitioner/ schedule/ appointment/
    encounter/ inventory/ masters/ staff/ onboarding/
    booking/ notify/ integrations/ account/
web/
  templates/            server-rendered HTML (layouts, pages, htmx partials, print views)
  static/                vendored Oat + htmx, plus a small hand-written app.js and app.css
```

## Documentation

Full step-by-step usage instructions — for every role, every screen — live in
[`Onboarding/`](./Onboarding/README.md). Start there for anything beyond initial setup.

## Data & backups

Postgres data lives in the `chikitsalaya_pgdata` Docker volume; uploaded logos and
letterheads live in `chikitsalaya_uploads`. Back up both if you care about not losing
anything — `docker compose down` alone doesn't touch volumes, but be deliberate before
running anything with `-v`.
