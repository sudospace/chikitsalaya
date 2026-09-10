# Clinic settings & integrations

## Clinic Settings

**Setup → My Clinic** (`/admin/company`):

- Clinic name, address, phone, email, registration number.
- **Logo** — upload to replace the header image / browser tab icon.
- **Letterhead** — pick a mode:
  - **HTML** — a block of typed HTML rendered at the top of every printed prescription.
  - **Image** — upload an image (e.g. a scanned physical letterhead) used instead.
- **Footer text** — shown at the bottom of prints.
- **Fee schedule** — First consultation fee and Follow-up fee. Set these to make the
  quick-pick fee options (on the booking page and the appointment fee editor) fill in
  automatically instead of requiring a typed amount every time.

## Integrations

**Setup → Integrations** (`/admin/integrations`).

### SMS

The one module actually wired up end-to-end. Point it at any HTTP endpoint that accepts
a JSON POST of `{"to": "...", "message": "..."}` — your own relay, or any gateway that
offers a plain webhook:

- **Webhook URL**
- Optional auth header (name + value) if the endpoint needs one
- Optional sender ID
- **Active** toggle

Turn it on and online-booking OTPs (see [Online booking](./06-online-booking.md)) go out
for real instead of just hitting the server console log.

### Email

SMTP host/port, username/password, from-address, and an active toggle — credentials are
stored and validated for form-completeness, but **nothing in the app actually sends an
email yet**. This is here to be ready for it, not a working feature today.

### Payment

Provider name, key ID/secret, active toggle — same situation as Email: saved, not yet
wired to anything. Fee capture (amount + paid/unpaid) works today without this; there's
no invoicing or payment-gateway checkout flow.
