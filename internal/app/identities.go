package app

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"

	pmail "petrel/internal/mail"
)

// Identity is a name and address a mailbox sends as, with its signature.
// Whether the mail server accepts sending as an address is up to it: most
// allow their aliases and refuse the rest.
type Identity struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"accountId"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	ReplyTo   string `json:"replyTo"`
	Signature string `json:"signature"` // sanitized HTML
	Default   bool   `json:"default"`
}

const maxSignature = 20000

// Identities returns the user's identities, defaults first, making the
// default of each mailbox that has none yet.
func (m *MailAccounts) Identities(ctx context.Context, userID int64) ([]Identity, error) {
	if _, err := m.db.ExecContext(ctx, `INSERT INTO identities (user_id, account_id, name, email, is_default, created_at)
		SELECT a.user_id, a.id, a.display_name, a.email, 1, ? FROM mail_accounts a
		WHERE a.user_id = ? AND NOT EXISTS (SELECT 1 FROM identities i WHERE i.account_id = a.id AND i.is_default = 1)`,
		time.Now().UnixMilli(), userID); err != nil {
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx, `SELECT id, account_id, name, email, reply_to, signature, is_default FROM identities
		WHERE user_id = ? AND account_id IN (SELECT id FROM mail_accounts WHERE user_id = ?)
		ORDER BY account_id, is_default DESC, id`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Identity{}
	for rows.Next() {
		var i Identity
		if err := rows.Scan(&i.ID, &i.AccountID, &i.Name, &i.Email, &i.ReplyTo, &i.Signature, &i.Default); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Identity returns one of the user's identities.
func (m *MailAccounts) Identity(ctx context.Context, userID, id int64) (*Identity, error) {
	var i Identity
	err := m.db.QueryRowContext(ctx, `SELECT id, account_id, name, email, reply_to, signature, is_default FROM identities WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&i.ID, &i.AccountID, &i.Name, &i.Email, &i.ReplyTo, &i.Signature, &i.Default)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, kit.NotFound()
	}
	return &i, err
}

// SaveIdentity adds an identity to one of the user's mailboxes (ID 0) or
// changes one, checking its addresses and cleaning its signature.
func (m *MailAccounts) SaveIdentity(ctx context.Context, userID int64, in Identity) (*Identity, error) {
	if _, err := m.get(ctx, userID, in.AccountID); err != nil {
		return nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.ReplyTo = strings.TrimSpace(in.ReplyTo)
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email {
		return nil, kit.FieldError("email", tr(ctx, "e.identityEmail"))
	}
	if in.ReplyTo != "" {
		if _, err := mail.ParseAddressList(in.ReplyTo); err != nil {
			return nil, kit.FieldError("replyTo", tr(ctx, "e.addresses", map[string]any{"field": "Reply-To"}))
		}
	}
	if len(in.Signature) > maxSignature {
		return nil, kit.FieldError("signature", tr(ctx, "e.signatureLong"))
	}
	in.Signature = pmail.CleanFragment(in.Signature)
	if in.ID == 0 {
		res, err := m.db.ExecContext(ctx, `INSERT INTO identities (user_id, account_id, name, email, reply_to, signature, is_default, created_at) VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
			userID, in.AccountID, in.Name, in.Email, in.ReplyTo, in.Signature, time.Now().UnixMilli())
		if err != nil {
			return nil, err
		}
		in.ID, _ = res.LastInsertId()
		return &in, nil
	}
	res, err := m.db.ExecContext(ctx, `UPDATE identities SET name = ?, email = CASE WHEN is_default = 1 THEN email ELSE ? END, reply_to = ?, signature = ? WHERE id = ? AND user_id = ? AND account_id = ?`,
		in.Name, in.Email, in.ReplyTo, in.Signature, in.ID, userID, in.AccountID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, kit.NotFound()
	}
	return &in, nil
}

// DeleteIdentity removes an identity that isn't a mailbox's default.
func (m *MailAccounts) DeleteIdentity(ctx context.Context, userID, id int64) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM identities WHERE id = ? AND user_id = ? AND is_default = 0`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return kit.FormError(tr(ctx, "e.identityDefault"))
	}
	return nil
}

// Account returns one of the user's mailboxes.
func (m *MailAccounts) Account(ctx context.Context, userID, id int64) (*MailAccount, error) {
	list, err := m.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, kit.NotFound()
}
