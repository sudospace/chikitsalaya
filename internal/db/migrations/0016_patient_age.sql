-- +goose Up
-- Front-desk staff registering a walk-in almost never know an exact date
-- of birth, and nothing in the app ever needed day-level precision — only
-- "age" gets shown or printed anywhere. Down migration recovers dob as
-- NULL for every row since a birth date can't be reconstructed from an
-- age.
ALTER TABLE patients ADD COLUMN age INT;
ALTER TABLE patients DROP COLUMN dob;

-- +goose Down
ALTER TABLE patients ADD COLUMN dob DATE;
ALTER TABLE patients DROP COLUMN age;
