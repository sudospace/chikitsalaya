# Admin setup

The first admin login is created by the operator via `--seed-admin` (see root README's
"Quick start") — this single command creates both the clinic and that first admin
account in one shot, so there's no separate "create a clinic" step to walk through.

That first login (and any login created via `/admin/users/new` with the Admin role
selected) runs a one-time setup wizard (`/onboarding/...`) covering the clinic's core
data. Every step lets you add as many records as you like before moving on, and has a
**Skip setup for now** escape hatch — nothing here is mandatory, and everything can be
added later from the regular admin screens under the **Catalog** and **Setup** menus in
the header.

## 1. Doctors

For each practitioner: name, qualification, specialization, phone/email, and
(optionally, right on the same step) their **weekly schedule** — day of week, time
range, slot length, and room/service unit. This schedule is what generates the bookable
time slots patients and staff pick from later, so add it now if you know it, or come
back to **Setup → Users** → open the doctor's practitioner record any time.

## 2. Medications (drug catalog)

Name, generic name, form/strength, and an optional **opening stock quantity** if you
want the stock ledger to start non-zero instead of at 0. Duplicate names (case-
insensitive) are rejected — if a drug already exists, it's reused rather than creating a
second record with a slightly different capitalization.

## 3. Lab tests

The catalog of orderable lab tests, used when a doctor orders one during a consultation.

## 4. Procedures

Same idea, for procedures.

Dosage/duration picklists (once a day, twice a day, before food, 5 days, 2 weeks, etc.)
are seeded automatically — no step needed for those. They can be edited later at
**Catalog → Dosage/Duration**.

## After the wizard

All four catalogs stay editable forever under the **Catalog** dropdown in the header
(Drugs / Dosage-Duration / Lab Tests / Procedures), and doctors under **Setup → Users**
→ **Practitioners** (each doctor's schedule lives on their practitioner record, editable
any time).

## Add staff logins

From **Setup → Users** (`/admin/users/new`), create logins for **more admins**,
**doctors**, and **receptionists** — any existing admin can create another admin, there's
no separate "master" tier above it:

- Name, email, password + confirm password, mobile, address, emergency contact — full
  fields, same as your own account.
- **Role**: Admin, Doctor, or Receptionist.
- For a **doctor**, you choose:
  - **Create a new practitioner** (default) — fills in the practitioner record (name,
    qualification, specialization) from this same form, no separate step.
  - **Link to an existing practitioner** — if the practitioner record was already
    created in step 1 above (or separately) without a login yet, pick it from a list
    instead of creating a duplicate.
- Every module's view/edit permission is set here too — see
  [Roles & permissions](./07-roles-and-permissions.md).

An admin who also sees patients themselves can link their own login to a practitioner
record and switch into that doctor's view — see
[Account page](./05-account-page.md#also-a-doctor).

At this point the clinic is ready to use — see [Day-to-day operations](./04-day-to-day.md).
