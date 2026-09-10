-- +goose Up
-- Standard dosage/duration picklist presets (common prescription shorthand
-- like BID/TID/QID plus hour/day/week/month durations). Existing companies
-- get the new set added alongside whatever they already have (nothing
-- removed, since prescription_items may already reference an old row by
-- id); new companies get only this set going forward (see
-- internal/opd/company/company.go's defaultLookupValues).
INSERT INTO lookup_values (company_id, category, value, sort_order)
SELECT c.id, m.category, m.value, m.sort_order
FROM companies c
CROSS JOIN (VALUES
    ('dosage', '1-0-0', 1), ('dosage', '0-1-0', 2), ('dosage', '0-0-1', 3),
    ('dosage', '1-0-1', 4), ('dosage', '1-1-1', 5), ('dosage', '1-1-1-1', 6),
    ('dosage', 'Once Daily', 7), ('dosage', 'Once Bedtime', 8), ('dosage', 'BID', 9),
    ('dosage', 'TID', 10), ('dosage', 'QID', 11), ('dosage', '5 times a day', 12),
    ('duration', '1 Hour', 1), ('duration', '2 Hour', 2), ('duration', '3 Hour', 3),
    ('duration', '4 Hour', 4), ('duration', '5 Hour', 5), ('duration', '6 Hour', 6),
    ('duration', '7 Hour', 7), ('duration', '8 Hour', 8), ('duration', '9 Hour', 9),
    ('duration', '10 Hour', 10), ('duration', '11 Hour', 11), ('duration', '12 Hour', 12),
    ('duration', '1 Day', 13), ('duration', '2 Day', 14), ('duration', '3 Day', 15),
    ('duration', '4 Day', 16), ('duration', '5 Day', 17), ('duration', '6 Day', 18),
    ('duration', '1 Week', 19), ('duration', '2 Week', 20), ('duration', '3 Week', 21),
    ('duration', '4 Week', 22), ('duration', '5 Week', 23), ('duration', '1 Month', 24),
    ('duration', '2 Month', 25), ('duration', '3 Month', 26)
) AS m(category, value, sort_order)
ON CONFLICT (company_id, category, value) DO NOTHING;

-- +goose Down
-- Not reversible in a targeted way (can't distinguish these from
-- independently-added rows with the same values) — down migration is a
-- no-op.
