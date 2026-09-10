# Onboarding & Operations Guide

Step-by-step instructions for setting up and running Chikitsalaya, organized by who's
doing the work. If you just want the app running, see the root [`README.md`](../README.md)
first — this folder picks up after that, at first login.

1. [First login & 2FA](./01-first-login-and-2fa.md) — every account, every time
2. [Admin setup](./03-admin-setup.md) — the one-time wizard: doctors, drugs, lab tests,
   procedures, users
3. [Day-to-day operations](./04-day-to-day.md) — patients, appointments, consultations,
   prescriptions, Quick Prescription, printing
4. [Account page](./05-account-page.md) — password, personal details, doctor schedule,
   acting as a doctor
5. [Online booking](./06-online-booking.md) — the public, no-login booking page
6. [Roles & permissions](./07-roles-and-permissions.md) — what each role can see/do, and
   how to restrict a specific login further
7. [Clinic settings & integrations](./08-settings-and-integrations.md) — logo,
   letterhead, fee schedule, SMS/email/payment

## Quick map: who does what

| Task | Who |
|---|---|
| Install/run the app | Operator (CLI, see root README) |
| Create the clinic + first admin login | Operator (CLI, `--seed-admin`) |
| Add doctors, drugs, lab tests, procedures | Admin |
| Add more staff (admin/doctor/receptionist logins) | Any admin |
| Book/manage appointments, run consultations, print prescriptions | Admin, doctor, receptionist |
| Update own password/personal info | Anyone, from **My Account** |
