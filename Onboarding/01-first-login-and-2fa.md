# First login & 2FA

There is no public signup page. Every login is created by someone above it in the role
chain (see [roles & permissions](./07-roles-and-permissions.md)) — the very first admin
is the only exception, created once via the CLI (root README's "Quick start").

## Mandatory 2FA, every account

The very first time any account logs in, it's stopped at `/2fa/setup` before it can do
anything else:

1. A QR code is shown. Scan it with any TOTP authenticator app — Google Authenticator,
   Authy, 1Password, Microsoft Authenticator, etc.
2. Enter the 6-digit code the app shows to confirm the pairing.

From then on, every login (not just the first) asks for a fresh 6-digit code at
`/2fa/verify`, right after the password. There's no setting to turn this off, and no
"remember this device" — it's asked every time, for every role including admin.

**Lost access to the authenticator app:** there's no self-service recovery. Whoever
created the login has to reset it:
- For the first admin: re-run `--seed-admin` with the same email — it resets the
  password, and the next login re-triggers 2FA enrollment from scratch.
- For anyone else: currently there is no in-app "reset 2FA" button — if this happens in
  practice, reach out for a direct DB fix (`users.totp_secret` / `totp_enabled_at`
  cleared, which re-triggers enrollment on next login).

## What happens right after 2FA

- **Admin**, first login ever (right after `--seed-admin`): dropped straight into the
  clinic setup wizard (see [Admin setup](./03-admin-setup.md)).
- **Doctor / receptionist**, or anyone who has already been through onboarding: lands on
  the home dashboard.
