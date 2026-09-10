# Account page

Every logged-in user has **My Account** in the profile menu (top right, under your
email) — `/account`. It's split into tabs down the left:

## Credentials

- Your email (read-only) and role — shown as plain text, e.g. *Doctor* or, for an admin
  also acting as a doctor, *Admin + Doctor*.
- **What I can access** — a read-only list of every module (Drugs, Users, Lab Tests,
  etc.) and whether your login has *No access*, *View only*, or *Full access* to each —
  set by whoever created your login; see [Roles & permissions](./07-roles-and-permissions.md).
- **Change password** — current password + new password + confirm.

## Also a doctor?

Only shown to admin logins. Two states:

- **Not linked yet**: pick any practitioner and link your login to it.
- **Linked**: shows which practitioner you're linked to, plus a button to switch into
  **doctor view** — your dashboard scopes to that practitioner's own appointments, and
  the header shows the extra "+ Doctor" role label. **Exit Doctor View** from the
  profile menu switches back. This does not remove any admin privileges — it's an
  additional session-level view, not a role change.

## Personal details

Full name, phone, address, emergency contact name/phone. **Name is locked once set** —
after your first save, the name field can't be changed here (ask another admin if it was
entered wrong).

## Schedule (doctors only)

Shown if you're a doctor (or an admin currently acting as one). Add/remove your own
weekly schedule blocks — day of week, time range, slot length, room — the same schedule
that drives the slots patients/staff pick when booking an appointment with you.
