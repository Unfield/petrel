-- OpenPGP. The user's own keys: private keys locked with their passphrase,
-- and sealed again with Petrel's key.
CREATE TABLE pgp_keys (
	id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL,
	email TEXT NOT NULL,
	fingerprint TEXT NOT NULL,
	public TEXT NOT NULL,
	private TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	UNIQUE (user_id, fingerprint)
);

-- Other people's public keys, by address: imported, looked up (WKD), or
-- learned from their mail (Autocrypt).
CREATE TABLE pgp_public (
	user_id INTEGER NOT NULL,
	email TEXT NOT NULL,
	fingerprint TEXT NOT NULL,
	public TEXT NOT NULL,
	source TEXT NOT NULL,
	added_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, email)
);

-- +down
DROP TABLE pgp_keys;
DROP TABLE pgp_public;
