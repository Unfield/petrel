-- Mailboxes connected to Petrel: one per sign-in in direct mode, any number
-- per user in accounts mode.
CREATE TABLE mail_accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	email TEXT NOT NULL,
	display_name TEXT NOT NULL DEFAULT '',
	protocol TEXT NOT NULL CHECK (protocol IN ('jmap', 'imap', 'pop3')),
	-- Server settings as JSON: hosts, ports, TLS mode or the JMAP session URL.
	settings TEXT NOT NULL DEFAULT '{}',
	-- Credentials, encrypted with AES-GCM. Never stored in plain text.
	secret BLOB,
	color TEXT NOT NULL DEFAULT '',
	position INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE (user_id, email)
);

-- +down
DROP TABLE mail_accounts;
