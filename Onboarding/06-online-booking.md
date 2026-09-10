# Online booking

The clinic gets a public, no-login booking page at `/book` — share this link directly
with patients.

## Patient flow

1. Enter mobile number → request OTP (`/book/otp/request`).
2. Enter the code → verified (`/book/otp/verify`).
3. Once verified, the visitor sees any appointment already on file for that number, with
   self-service **reschedule** or **cancel**, or can book a new one: pick doctor → date →
   an open slot.

New bookings land with status **requested** — they are **not** an automatic confirmed
slot. Staff must **Confirm** them from the day view (**Appointments** in the header)
before they're a real commitment; **Reject** is the alternative if the slot doesn't
actually work.

## OTP delivery

Out of the box, the OTP code is just logged to the server console (fine for
development/testing). To actually send it as an SMS, wire up a real gateway — see
[Settings & integrations](./08-settings-and-integrations.md#sms).
