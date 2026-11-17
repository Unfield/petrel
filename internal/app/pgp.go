package app

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/sealed"

	"petrel/internal/mail"
	"petrel/internal/pgp"
)

// OpenPGP runs on the server, like Roundcube's Enigma: private keys are
// kept locked with their passphrase (and sealed with Petrel's key), and
// unlocked into memory for a while when the user enters it. Encrypting to
// others needs only their public keys.

// OwnKey is one of the user's own keys.
type OwnKey struct {
	ID     int64   `json:"id"`
	Email  string  `json:"email"`
	Key    pgp.Key `json:"key"`
	Public string  `json:"public"`
}

// PublicKey is someone else's key.
type PublicKey struct {
	Email  string  `json:"email"`
	Key    pgp.Key `json:"key"`
	Source string  `json:"source"` // import, wkd, autocrypt
	Public string  `json:"public"`
}

// unlockFor is how long an unlocked key stays open without use.
const unlockFor = 30 * time.Minute

var unlocked = struct {
	sync.Mutex
	m map[int64]*unlockedKeys
}{m: map[int64]*unlockedKeys{}}

type unlockedKeys struct {
	keys  openpgp.EntityList
	until time.Time
}

// Unlocked returns the user's open private keys, renewing them.
func Unlocked(userID int64) openpgp.EntityList {
	unlocked.Lock()
	defer unlocked.Unlock()
	u, ok := unlocked.m[userID]
	if !ok || time.Now().After(u.until) {
		delete(unlocked.m, userID)
		return nil
	}
	u.until = time.Now().Add(unlockFor)
	return u.keys
}

// LockKeys closes the user's keys, as signing out does.
func LockKeys(userID int64) {
	unlocked.Lock()
	delete(unlocked.m, userID)
	unlocked.Unlock()
}

func (m *MailAccounts) ownKeys(ctx context.Context, userID int64, private bool) ([]OwnKey, []string, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, email, public, private FROM pgp_keys WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []OwnKey{}
	var privs []string
	for rows.Next() {
		var k OwnKey
		var priv sealed.String
		if err := rows.Scan(&k.ID, &k.Email, &k.Public, &priv); err != nil {
			return nil, nil, err
		}
		if d, _, err := pgp.Inspect(k.Public); err == nil {
			k.Key = d
		}
		out = append(out, k)
		privs = append(privs, string(priv))
	}
	if !private {
		privs = nil
	}
	return out, privs, rows.Err()
}

// OwnKeys lists the user's keys.
func (m *MailAccounts) OwnKeys(ctx context.Context, userID int64) ([]OwnKey, error) {
	k, _, err := m.ownKeys(ctx, userID, false)
	return k, err
}

func (m *MailAccounts) storeOwn(ctx context.Context, userID int64, email, public, private string, k pgp.Key) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO pgp_keys (user_id, email, fingerprint, public, private, created_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, fingerprint) DO UPDATE SET public = excluded.public, private = excluded.private, email = excluded.email`,
		userID, strings.ToLower(email), k.Fingerprint, public, sealed.String(private), time.Now().UnixMilli())
	return err
}

// GenerateKey makes a key for one of the user's addresses.
func (m *MailAccounts) GenerateKey(ctx context.Context, userID int64, name, email, passphrase string) (*OwnKey, error) {
	if len(passphrase) < 8 {
		return nil, kit.FieldError("passphrase", tr(ctx, "e.pgpPassphrase"))
	}
	if !m.ownsAddress(ctx, userID, email) {
		return nil, kit.FieldError("email", tr(ctx, "e.pgpNotYours"))
	}
	priv, pub, k, err := pgp.Generate(name, email, passphrase)
	if err != nil {
		return nil, err
	}
	if err := m.storeOwn(ctx, userID, email, pub, priv, k); err != nil {
		return nil, err
	}
	LockKeys(userID) // the new key opens with the next unlock
	return &OwnKey{Email: email, Key: k, Public: pub}, nil
}

// ImportKey stores an existing private key, locking it with a new
// passphrase (or its own).
func (m *MailAccounts) ImportKey(ctx context.Context, userID int64, armored, passphrase, newPassphrase string) (*OwnKey, error) {
	k, pub, err := pgp.Inspect(armored)
	if err != nil || !k.Private {
		return nil, kit.FieldError("key", tr(ctx, "e.pgpBadPrivate"))
	}
	if newPassphrase == "" {
		newPassphrase = passphrase
	}
	if len(newPassphrase) < 8 {
		return nil, kit.FieldError("passphrase", tr(ctx, "e.pgpPassphrase"))
	}
	priv, err := pgp.Relock(armored, passphrase, newPassphrase)
	if err != nil {
		return nil, kit.FieldError("passphrase", tr(ctx, "e.pgpWrong"))
	}
	email := ""
	for _, e := range k.Emails {
		if m.ownsAddress(ctx, userID, e) {
			email = e
			break
		}
	}
	if email == "" {
		return nil, kit.FieldError("key", tr(ctx, "e.pgpNotYours"))
	}
	if err := m.storeOwn(ctx, userID, email, pub, priv, k); err != nil {
		return nil, err
	}
	LockKeys(userID)
	return &OwnKey{Email: email, Key: k, Public: pub}, nil
}

// DeleteKey deletes one of the user's keys.
func (m *MailAccounts) DeleteKey(ctx context.Context, userID, id int64) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM pgp_keys WHERE id = ? AND user_id = ?`, id, userID)
	LockKeys(userID)
	return err
}

// ownsAddress: the address is one of the user's mailboxes or identities.
func (m *MailAccounts) ownsAddress(ctx context.Context, userID int64, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	ids, _ := m.Identities(ctx, userID)
	for _, id := range ids {
		if id.Email == email {
			return true
		}
	}
	return false
}

// UnlockKeys opens every key of the user's that the passphrase opens.
func (m *MailAccounts) UnlockKeys(ctx context.Context, userID int64, passphrase string) (int, error) {
	_, privs, err := m.ownKeys(ctx, userID, true)
	if err != nil {
		return 0, err
	}
	var open openpgp.EntityList
	for _, p := range privs {
		if e, err := pgp.Unlock(p, passphrase); err == nil {
			open = append(open, e)
		}
	}
	if len(open) == 0 {
		if len(privs) == 0 {
			return 0, kit.Error(http.StatusBadRequest, tr(ctx, "e.pgpNoKeys"))
		}
		return 0, kit.Error(http.StatusBadRequest, tr(ctx, "e.pgpWrong"))
	}
	unlocked.Lock()
	unlocked.m[userID] = &unlockedKeys{keys: open, until: time.Now().Add(unlockFor)}
	unlocked.Unlock()
	return len(open), nil
}

// Publics lists the public keys the user has for others.
func (m *MailAccounts) Publics(ctx context.Context, userID int64) ([]PublicKey, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT email, public, source FROM pgp_public WHERE user_id = ? ORDER BY email`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublicKey{}
	for rows.Next() {
		var p PublicKey
		if err := rows.Scan(&p.Email, &p.Public, &p.Source); err != nil {
			return nil, err
		}
		if d, _, err := pgp.Inspect(p.Public); err == nil {
			p.Key = d
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddPublic keeps someone's public key for each address in it. Autocrypt
// keys don't replace keys the user chose.
func (m *MailAccounts) AddPublic(ctx context.Context, userID int64, armored, source string) ([]string, error) {
	k, pub, err := pgp.Inspect(armored)
	if err != nil {
		return nil, kit.Error(http.StatusBadRequest, tr(ctx, "e.pgpBadKey"))
	}
	if !k.CanEncrypt {
		return nil, kit.Error(http.StatusBadRequest, tr(ctx, "e.pgpCantEncrypt"))
	}
	q := `INSERT INTO pgp_public (user_id, email, fingerprint, public, source, added_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, email) DO UPDATE SET fingerprint = excluded.fingerprint, public = excluded.public, source = excluded.source, added_at = excluded.added_at`
	if source == "autocrypt" {
		q += ` WHERE pgp_public.source = 'autocrypt'`
	}
	for _, e := range k.Emails {
		if _, err := m.db.ExecContext(ctx, q, userID, e, k.Fingerprint, pub, source, time.Now().UnixMilli()); err != nil {
			return nil, err
		}
	}
	return k.Emails, nil
}

// DeletePublic forgets someone's key.
func (m *MailAccounts) DeletePublic(ctx context.Context, userID int64, email string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM pgp_public WHERE user_id = ? AND email = ?`, userID, strings.ToLower(email))
	return err
}

// keyring is every public key the user has, their own included, for
// checking signatures.
func (m *MailAccounts) keyring(ctx context.Context, userID int64) openpgp.EntityList {
	var list openpgp.EntityList
	add := func(armored string) {
		if e, err := pgp.Read(armored); err == nil {
			list = append(list, e)
		}
	}
	if own, err := m.OwnKeys(ctx, userID); err == nil {
		for _, k := range own {
			add(k.Public)
		}
	}
	if pubs, err := m.Publics(ctx, userID); err == nil {
		for _, p := range pubs {
			add(p.Public)
		}
	}
	return list
}

// keyFor is the public key for an address: the user's own, or someone's.
func (m *MailAccounts) keyFor(ctx context.Context, userID int64, email string) *openpgp.Entity {
	email = strings.ToLower(strings.TrimSpace(email))
	var armored string
	err := m.db.QueryRowContext(ctx, `SELECT public FROM pgp_keys WHERE user_id = ? AND email = ? ORDER BY id DESC LIMIT 1`, userID, email).Scan(&armored)
	if errors.Is(err, sql.ErrNoRows) {
		err = m.db.QueryRowContext(ctx, `SELECT public FROM pgp_public WHERE user_id = ? AND email = ?`, userID, email).Scan(&armored)
	}
	if err != nil {
		return nil
	}
	e, err := pgp.Read(armored)
	if err != nil {
		return nil
	}
	return e
}

// HasKeys says which addresses Petrel can encrypt to.
func (m *MailAccounts) HasKeys(ctx context.Context, userID int64, emails []string) map[string]bool {
	out := map[string]bool{}
	for _, e := range emails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			out[e] = m.keyFor(ctx, userID, e) != nil
		}
	}
	return out
}

// zbase32 is WKD's encoding of the hashed local part.
func zbase32(b []byte) string {
	const alphabet = "ybndrfg8ejkmcpqxot1uwisza345h769"
	var out strings.Builder
	var buf, bits uint
	for _, c := range b {
		buf = buf<<8 | uint(c)
		bits += 8
		for bits >= 5 {
			out.WriteByte(alphabet[(buf>>(bits-5))&31])
			bits -= 5
		}
	}
	if bits > 0 {
		out.WriteByte(alphabet[(buf<<(5-bits))&31])
	}
	return out.String()
}

// LookupWKD asks the address's domain for its key (Web Key Directory),
// through the same guard as everything Petrel fetches, and keeps it.
func (m *MailAccounts) LookupWKD(ctx context.Context, userID int64, email string) ([]string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" || strings.ContainsAny(domain, "/:?#") {
		return nil, kit.Error(http.StatusBadRequest, tr(ctx, "e.identityEmail"))
	}
	h := sha1.Sum([]byte(local))
	hash := zbase32(h[:])
	for _, u := range []string{
		"https://openpgpkey." + domain + "/.well-known/openpgpkey/" + domain + "/hu/" + hash + "?l=" + url.QueryEscape(local),
		"https://" + domain + "/.well-known/openpgpkey/hu/" + hash + "?l=" + url.QueryEscape(local),
	} {
		body, err := mail.Fetch(ctx, u, m.opts, 256<<10)
		if err != nil {
			continue
		}
		if emails, err := m.AddPublic(ctx, userID, string(body), "wkd"); err == nil {
			return emails, nil
		}
	}
	return nil, kit.Error(http.StatusNotFound, tr(ctx, "e.pgpNotFound"))
}

// PGPView is what a message's OpenPGP says.
type PGPView struct {
	Encrypted bool `json:"encrypted"`
	Decrypted bool `json:"decrypted"`
	// Locked: the user's keys must be unlocked to read it.
	Locked    bool           `json:"locked"`
	Error     string         `json:"error,omitempty"`
	Signature *pgp.Signature `json:"signature,omitempty"`
}

// openPGP reads a message's PGP: decrypting it into v if it can, checking
// signatures, and learning the sender's Autocrypt key.
func (m *MailAccounts) openPGP(ctx context.Context, userID int64, p mail.Provider, id string, msg *mail.Message, v *MessageView, global string) {
	if msg.Autocrypt != "" && len(msg.From) > 0 {
		if email, pub, err := pgp.FromAutocrypt(msg.Autocrypt); err == nil && strings.EqualFold(email, msg.From[0].Email) {
			_, _ = m.AddPublic(ctx, userID, pub, "autocrypt")
		}
	}
	if !msg.PGP {
		return
	}
	keyring := m.keyring(ctx, userID)
	private := Unlocked(userID)
	view := &PGPView{}
	v.PGP = view

	// Inline PGP in the text.
	if strings.Contains(msg.Text, "-----BEGIN PGP MESSAGE-----") {
		view.Encrypted = true
		if len(private) == 0 {
			view.Locked = true
			return
		}
		plain, sig, err := pgp.Decrypt([]byte(msg.Text), private, keyring)
		if err != nil {
			view.Error = tr(ctx, "e.pgpCantRead")
			return
		}
		view.Decrypted, view.Signature = true, sig
		v.Body = mail.Body{HTML: `<div class="petrel-plain">` + strings.ReplaceAll(html.EscapeString(string(plain)), "\n", "<br>") + `</div>`, Plain: true}
		return
	}
	if strings.Contains(msg.Text, "-----BEGIN PGP SIGNED MESSAGE-----") {
		if text, sig, ok := pgp.VerifyClearsigned([]byte(msg.Text), keyring); ok {
			view.Signature = sig
			v.Body = mail.Body{HTML: `<div class="petrel-plain">` + strings.ReplaceAll(html.EscapeString(string(text)), "\n", "<br>") + `</div>`, Plain: true}
		}
		return
	}

	// PGP/MIME: from the original.
	rp, ok := p.(mail.RawProvider)
	if !ok {
		return
	}
	r, err := rp.Raw(ctx, id)
	if err != nil {
		return
	}
	raw, _ := io.ReadAll(io.LimitReader(r, 50<<20))
	r.Close()
	o := pgp.Open(raw, private, keyring)
	switch o.Kind {
	case "encrypted":
		view.Encrypted = true
		if o.Inner == nil {
			if errors.Is(o.Err, pgp.ErrNoKey) && len(private) == 0 {
				view.Locked = true
			} else {
				view.Error = tr(ctx, "e.pgpCantRead")
			}
			// Hide the ciphertext attachment.
			v.Attachments = []AttachmentView{}
			v.Body = mail.Body{HTML: "", Plain: true}
			return
		}
		view.Decrypted, view.Signature = true, o.Signature
		html, text, atts, err := mail.ParseEntity(o.Inner)
		if err != nil {
			view.Error = tr(ctx, "e.pgpCantRead")
			return
		}
		inner := &mail.Message{HTML: html, Text: text, Attachments: atts}
		v.Body = mail.Clean(inner)
		v.Attachments = []AttachmentView{}
		for _, a := range atts {
			v.Attachments = append(v.Attachments, AttachmentView{Attachment: a,
				URL: "/api/mail/attachment?pgp=1&id=" + url.QueryEscape(global) + "&part=" + url.QueryEscape(a.ID)})
		}
	case "signed":
		view.Signature = o.Signature
	}
	// The signature itself isn't for reading.
	kept := v.Attachments[:0]
	for _, a := range v.Attachments {
		if !strings.EqualFold(a.Type, "application/pgp-signature") && !strings.EqualFold(a.Type, "application/pgp-encrypted") {
			kept = append(kept, a)
		}
	}
	v.Attachments = kept
}

// DecryptedPart reads an attachment inside an encrypted message.
func (m *MailAccounts) DecryptedPart(ctx context.Context, userID int64, message, part string) (mail.Attachment, []byte, error) {
	r, err := m.Raw(ctx, userID, message)
	if err != nil {
		return mail.Attachment{}, nil, err
	}
	raw, _ := io.ReadAll(io.LimitReader(r, 50<<20))
	r.Close()
	o := pgp.Open(raw, Unlocked(userID), m.keyring(ctx, userID))
	if o.Inner == nil {
		return mail.Attachment{}, nil, kit.Error(http.StatusForbidden, tr(ctx, "e.pgpLocked"))
	}
	a, body, err := mail.EntityPart(o.Inner, part)
	if err != nil {
		return mail.Attachment{}, nil, kit.NotFound()
	}
	return a, body, nil
}

// seal makes an outgoing message encrypted and/or signed, and gives it an
// Autocrypt header when the sender has a key.
func (m *MailAccounts) seal(ctx context.Context, userID int64, out *mail.Outgoing, encrypt, sign bool) error {
	own := m.keyFor(ctx, userID, out.From.Email)
	var armored string
	_ = m.db.QueryRowContext(ctx, `SELECT public FROM pgp_keys WHERE user_id = ? AND email = ? ORDER BY id DESC LIMIT 1`, userID, strings.ToLower(out.From.Email)).Scan(&armored)
	if armored != "" {
		if data, err := pgp.AutocryptKeyData(armored); err == nil {
			if out.Headers == nil {
				out.Headers = map[string]string{}
			}
			out.Headers["Autocrypt"] = "addr=" + out.From.Email + "; keydata=" + data
		}
	}
	if !encrypt && !sign {
		return nil
	}
	var signer *openpgp.Entity
	if sign {
		keys := Unlocked(userID)
		if len(keys) == 0 {
			return kit.Error(http.StatusForbidden, tr(ctx, "e.pgpLocked"))
		}
		signer = keys[0]
		for _, k := range keys {
			for _, id := range k.Identities {
				if id.UserId != nil && strings.EqualFold(id.UserId.Email, out.From.Email) {
					signer = k
				}
			}
		}
	}
	if !encrypt {
		out.Seal = func(entity []byte) (string, map[string]string, []byte, error) { return pgp.SignMIME(entity, signer) }
		return nil
	}
	var to openpgp.EntityList
	var missing []string
	for _, list := range [][]mail.Address{out.To, out.Cc, out.Bcc} {
		for _, a := range list {
			if k := m.keyFor(ctx, userID, a.Email); k != nil {
				to = append(to, k)
			} else {
				missing = append(missing, a.Email)
			}
		}
	}
	if len(missing) > 0 {
		return kit.Error(http.StatusBadRequest, tr(ctx, "e.pgpMissing", map[string]any{"emails": strings.Join(missing, ", ")}))
	}
	if own != nil {
		to = append(to, own) // so the copy in Sent can be read
	}
	out.Seal = func(entity []byte) (string, map[string]string, []byte, error) {
		return pgp.EncryptMIME(entity, to, signer)
	}
	return nil
}
