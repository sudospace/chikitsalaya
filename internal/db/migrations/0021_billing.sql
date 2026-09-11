-- +goose Up
-- Itemized GST invoicing + a real multi-payment ledger, replacing the
-- lump appointments.fee_amount/fee_paid capture for anything post-visit
-- (that column stays — it's the pre-visit *quoted* fee; an invoice is the
-- post-visit *actual itemized bill*, the two coexist).
ALTER TABLE drugs ADD COLUMN price NUMERIC;
ALTER TABLE clinic_settings ADD COLUMN gstin TEXT NOT NULL DEFAULT '';
ALTER TABLE clinic_settings ADD COLUMN gst_registered BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE invoices (
    id               BIGSERIAL PRIMARY KEY,
    invoice_number   TEXT NOT NULL UNIQUE,
    patient_id       BIGINT NOT NULL REFERENCES patients(id),
    practitioner_id  BIGINT NOT NULL REFERENCES practitioners(id),
    appointment_id   BIGINT REFERENCES appointments(id),
    encounter_id     BIGINT REFERENCES encounters(id),
    status           TEXT NOT NULL DEFAULT 'unpaid'
                       CHECK (status IN ('unpaid', 'partially_paid', 'paid', 'cancelled')),
    subtotal         NUMERIC NOT NULL DEFAULT 0,
    discount_amount  NUMERIC NOT NULL DEFAULT 0,
    cgst_amount      NUMERIC NOT NULL DEFAULT 0,
    sgst_amount      NUMERIC NOT NULL DEFAULT 0,
    total            NUMERIC NOT NULL DEFAULT 0,
    notes            TEXT NOT NULL DEFAULT '',
    issued_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by       BIGINT REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_invoices_patient      ON invoices (patient_id, issued_at);
CREATE INDEX idx_invoices_practitioner ON invoices (practitioner_id, issued_at);
CREATE INDEX idx_invoices_encounter    ON invoices (encounter_id);
CREATE INDEX idx_invoices_status       ON invoices (status);

-- unit_price/tax_rate here are the permanent historical record -- never
-- re-derived from catalog/live prices. The *_order_id FKs are for
-- traceability back to the clinical order only, not for pricing.
CREATE TABLE invoice_line_items (
    id                    BIGSERIAL PRIMARY KEY,
    invoice_id            BIGINT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    kind                  TEXT NOT NULL CHECK (kind IN ('consultation', 'lab', 'procedure', 'drug', 'other')),
    description           TEXT NOT NULL,
    hsn_sac_code          TEXT NOT NULL DEFAULT '',
    quantity              NUMERIC NOT NULL DEFAULT 1,
    unit_price            NUMERIC NOT NULL DEFAULT 0,
    amount                NUMERIC NOT NULL DEFAULT 0,
    tax_rate              NUMERIC NOT NULL DEFAULT 0,
    tax_amount            NUMERIC NOT NULL DEFAULT 0,
    lab_order_id          BIGINT REFERENCES lab_orders(id),
    procedure_order_id    BIGINT REFERENCES procedure_orders(id),
    prescription_item_id  BIGINT REFERENCES prescription_items(id),
    sort_order            INT NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_invoice_line_items_invoice ON invoice_line_items (invoice_id);

CREATE TABLE payments (
    id            BIGSERIAL PRIMARY KEY,
    invoice_id    BIGINT NOT NULL REFERENCES invoices(id),
    amount        NUMERIC NOT NULL CHECK (amount > 0),
    method        TEXT NOT NULL CHECK (method IN ('cash', 'card', 'upi', 'bank_transfer', 'cheque', 'other')),
    reference_no  TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    paid_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    recorded_by   BIGINT REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_invoice ON payments (invoice_id);
CREATE INDEX idx_payments_paid_at ON payments (paid_at);

-- +goose Down
DROP TABLE payments;
DROP TABLE invoice_line_items;
DROP TABLE invoices;
ALTER TABLE clinic_settings DROP COLUMN gst_registered;
ALTER TABLE clinic_settings DROP COLUMN gstin;
ALTER TABLE drugs DROP COLUMN price;
