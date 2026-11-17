-- The names and addresses a mailbox sends as, each with a signature. Every
-- mailbox has one default, made from the mailbox when first needed.
CREATE TABLE identities (
	id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL,
	account_id INTEGER NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	email TEXT NOT NULL,
	reply_to TEXT NOT NULL DEFAULT '',
	signature TEXT NOT NULL DEFAULT '',
	is_default INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);
CREATE INDEX identities_account ON identities (account_id);

-- +down
DROP TABLE identities;
