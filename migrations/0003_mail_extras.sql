-- Files uploaded while composing, until the message is sent or a day passes.
CREATE TABLE uploads (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	size INTEGER NOT NULL,
	data BLOB NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE INDEX uploads_created ON uploads (created_at);

-- People the user writes to and hears from, for address suggestions.
CREATE TABLE contacts (
	user_id INTEGER NOT NULL,
	email TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	uses INTEGER NOT NULL DEFAULT 0,
	last_used INTEGER NOT NULL,
	PRIMARY KEY (user_id, email)
);

-- Each user's appearance and labels, so they follow them across browsers.
CREATE TABLE user_prefs (
	user_id INTEGER PRIMARY KEY,
	prefs TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);

-- Snoozed messages, moved back to the inbox when their time comes.
CREATE TABLE snoozes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	account_id INTEGER NOT NULL,
	message_id TEXT NOT NULL,
	until INTEGER NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE INDEX snoozes_until ON snoozes (until);

-- +down
DROP TABLE snoozes;
DROP TABLE user_prefs;
DROP TABLE contacts;
DROP TABLE uploads;
