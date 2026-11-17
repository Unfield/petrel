-- The address book: contacts the user saved, beside the ones learned from
-- mail, which only suggest addresses.
ALTER TABLE contacts ADD COLUMN saved INTEGER NOT NULL DEFAULT 0;
ALTER TABLE contacts ADD COLUMN phone TEXT NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN company TEXT NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN notes TEXT NOT NULL DEFAULT '';

-- +down
ALTER TABLE contacts DROP COLUMN saved;
ALTER TABLE contacts DROP COLUMN phone;
ALTER TABLE contacts DROP COLUMN company;
ALTER TABLE contacts DROP COLUMN notes;
