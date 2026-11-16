-- POP3 has no flags: Petrel keeps seen and flagged for its messages here.
CREATE TABLE pop3_flags (
	account_id INTEGER NOT NULL,
	uidl TEXT NOT NULL,
	seen INTEGER NOT NULL DEFAULT 0,
	flagged INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (account_id, uidl)
);

-- +down
DROP TABLE pop3_flags;
