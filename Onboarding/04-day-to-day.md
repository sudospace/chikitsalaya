# Day-to-day operations

For admin, doctor, and receptionist logins (scoped by
[permissions](./07-roles-and-permissions.md) where relevant).

## Patients

**Patients** in the header — search or create.

Fields: first/last name, **age**, sex, phone, address. Opening a patient shows their
detail page and a full **history timeline** — every past visit, newest first, with
diagnoses/prescriptions/orders from each.

## Appointments

**Appointments** in the header — a day view scoped to the logged-in doctor's own
schedule (or the whole clinic, for admin/receptionist). To book: pick a patient →
doctor → date → an open slot generated from that doctor's weekly schedule.

Per-appointment actions: **check-in**, **complete**, **cancel**, **no-show**, and for
anything that came in through [online booking](./06-online-booking.md), **confirm** or
**reject**. A **fee** can be set on any appointment — pick First consultation /
Follow-up (pre-filled from the clinic's [fee schedule](./08-settings-and-integrations.md#fee-schedule))
or type a custom amount, plus a paid/unpaid flag. This is capture only — there's no
invoicing or ledger.

## Consultations

From a checked-in appointment, **Start consultation** opens the workspace
(`/encounters/{id}`):

- **Vitals** — height/weight/BP/pulse/etc.; BMI is computed automatically.
- **Diagnoses** — ICD-10 search-as-you-type, add/remove any number.
- **Prescriptions** — pick from the drug catalog (live search) or type free-text; dosage
  and duration fields offer a dropdown of existing picklist values on focus, and
  **quantity auto-calculates** from dosage × duration (editable afterward if needed).
- **Lab / procedure orders** — order from catalog, record results/comments, remove if
  ordered in error.

**Complete consultation** locks the encounter (no further edits) and **auto-dispenses**
any prescribed stock from inventory.

## Print

Any completed consultation has a **Print** link (`/encounters/{id}/print`) — a
letterhead-branded, browser-printable prescription: patient/visit info, vitals (if
recorded), diagnoses, the Rx table, lab/procedure orders, advice/next-review, and a
signature line. Opens as a plain standalone page meant to be printed directly from the
browser (`Ctrl/Cmd+P`).

## Quick Prescription

**Quick Rx** in the header (`/quick-prescription`) — for writing a prescription without
walking the full appointment → check-in → consultation flow. One screen:

1. **Patient** — toggle *Existing* (live search, pick from results) or *New* (name, age,
   sex, phone entered right there).
2. **Doctor** — a practitioner picker, pre-selected to your own linked practitioner if
   you're logged in as a doctor.
3. **Notes / diagnosis**, an optional **plan** and **next review date**.
4. **Medications** — same drug/dosage/duration/quantity/remark rows as the consultation
   workspace, with **+ Add line** to add more.
5. Optional **lab tests** and **procedures** sections, same live-search pattern.

Submitting this actually persists everything, the same as a real visit: it creates the
patient if new, an appointment (marked completed — **Follow-up** if the patient already
has visit history, **Consultation** if not), the encounter, every prescription/lab/
procedure line, completes the encounter (dispensing stock), and redirects straight to
the print view. It shows up in that patient's history exactly like a normal visit.
