-- +goose Up
-- A company's standard consultation prices, so reception picks from a
-- price list instead of typing an amount from memory every time. Custom
-- (a one-off amount) stays supported by simply not matching either —
-- appointments.fee_amount already accepts any value.
ALTER TABLE companies ADD COLUMN first_consultation_fee NUMERIC;
ALTER TABLE companies ADD COLUMN follow_up_fee NUMERIC;

-- +goose Down
ALTER TABLE companies DROP COLUMN first_consultation_fee;
ALTER TABLE companies DROP COLUMN follow_up_fee;
