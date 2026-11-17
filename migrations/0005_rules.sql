-- Each mailbox's filters and vacation reply, as Petrel shows them. The
-- mail server runs them as the Sieve script Petrel writes from these.
CREATE TABLE mail_rules (
	account_id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL,
	rules TEXT NOT NULL DEFAULT '[]',
	vacation TEXT NOT NULL DEFAULT '{}',
	updated_at INTEGER NOT NULL
);

-- +down
DROP TABLE mail_rules;
