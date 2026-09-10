# Roles & permissions

## The three roles

| Role | Created by | Notes |
|---|---|---|
| `admin` | CLI (`--seed-admin`) for the first one; any existing admin after that | Full clinic setup and staff management |
| `doctor` | Any admin | Sees only their own appointments/schedule by default |
| `receptionist` | Any admin | Front-desk operations |

An admin can hold an additional "acting as doctor" session hat at the same time as being
admin (if linked to a practitioner record) — the header shows both, e.g. *Admin + Doctor*.

## Per-module view/edit permissions

Every login has, per module, one of:

- **No access** — the module doesn't appear at all.
- **View only** — can see the data but not add/edit/delete.
- **Full access** — can see and modify.

Modules that can be independently restricted: Patients, Practitioners, Appointments,
Consultations (encounters), Clinic Settings, Integrations, Service Units, Drugs,
Dosage/Duration, Lab Tests, Procedures, Users.

These are set when an admin creates a login (**Setup → Users → New**), including the
very first admin (granted full access on every module automatically by the
`--seed-admin` bootstrap). They're visible read-only to the account holder themselves on
their **My Account → Credentials** tab (see [Account page](./05-account-page.md)) — so
anyone can check what their own login is restricted to without needing to ask.

There's no separate screen to bulk-edit an existing user's permissions after creation
beyond re-editing that user from **Setup → Users**. There's also no admin-of-admins
tier — if an admin restricts their own account (or the only other admin's) down to no
access on Users/Clinic Settings, recovering requires direct database access, not an
in-app fix. Be deliberate when editing your own permissions.
