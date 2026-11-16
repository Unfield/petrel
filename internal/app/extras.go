package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/mail"
)

// ─── Uploads ─────────────────────────────────────────────

// MaxUpload is the largest file one can attach. Most servers refuse
// messages much over 25 MB.
const MaxUpload = 25 << 20

// Upload is a file waiting to be attached.
type Upload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// SaveUpload keeps a file for the user until it's sent.
func (m *MailAccounts) SaveUpload(ctx context.Context, userID int64, name, typ string, data []byte) (Upload, error) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	u := Upload{ID: hex.EncodeToString(b), Name: name, Type: typ, Size: int64(len(data))}
	_, err := m.db.ExecContext(ctx, `INSERT INTO uploads (id, user_id, name, type, size, data, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, userID, u.Name, u.Type, u.Size, data, time.Now().UnixMilli())
	return u, err
}

// FileRef is an attachment to send: an upload, or a part of a message
// (forwarding, or a draft's attachments).
type FileRef struct {
	Upload  string `json:"upload,omitempty"`
	Message string `json:"message,omitempty"`
	Part    string `json:"part,omitempty"`
}

// Files reads the attachments to send.
func (m *MailAccounts) Files(ctx context.Context, userID int64, refs []FileRef) ([]mail.File, error) {
	var out []mail.File
	var total int64
	for _, r := range refs {
		var f mail.File
		switch {
		case r.Upload != "":
			err := m.db.QueryRowContext(ctx, `SELECT name, type, data FROM uploads WHERE id = ? AND user_id = ?`, r.Upload, userID).Scan(&f.Name, &f.Type, &f.Data)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, kit.Error(http.StatusBadRequest, "An attachment expired: attach it again.")
			}
			if err != nil {
				return nil, err
			}
		case r.Message != "":
			account, id, ok := SplitID(r.Message)
			if !ok {
				return nil, kit.NotFound()
			}
			p, _, err := m.Provider(ctx, userID, account)
			if err != nil {
				return nil, err
			}
			att, body, err := p.Attachment(ctx, id, r.Part)
			if err != nil {
				return nil, err
			}
			f.Name, f.Type = att.Name, att.Type
			f.Data, err = io.ReadAll(io.LimitReader(body, MaxUpload+1))
			body.Close()
			if err != nil {
				return nil, err
			}
		default:
			continue
		}
		total += int64(len(f.Data))
		if total > MaxUpload {
			return nil, kit.Error(http.StatusRequestEntityTooLarge, "Attachments can be 25 MB together at most.")
		}
		out = append(out, f)
	}
	return out, nil
}

// DropUploads deletes uploads once sent, and any older than a day.
func (m *MailAccounts) DropUploads(ctx context.Context, userID int64, refs []FileRef) {
	for _, r := range refs {
		if r.Upload != "" {
			_, _ = m.db.ExecContext(ctx, `DELETE FROM uploads WHERE id = ? AND user_id = ?`, r.Upload, userID)
		}
	}
	_, _ = m.db.ExecContext(ctx, `DELETE FROM uploads WHERE created_at < ?`, time.Now().Add(-24*time.Hour).UnixMilli())
}

// ─── Contacts ────────────────────────────────────────────

// Remember notes addresses for suggestions; weight says how much this use
// counts (writing to someone counts more than hearing from them).
func (m *MailAccounts) Remember(ctx context.Context, userID int64, list []mail.Address, weight int) {
	now := time.Now().UnixMilli()
	for _, a := range list {
		email := strings.ToLower(strings.TrimSpace(a.Email))
		if email == "" || !strings.Contains(email, "@") || strings.Contains(email, "noreply") || strings.Contains(email, "no-reply") {
			continue
		}
		_, _ = m.db.ExecContext(ctx, `INSERT INTO contacts (user_id, email, name, uses, last_used) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (user_id, email) DO UPDATE SET uses = uses + excluded.uses,
			name = CASE WHEN excluded.name != '' THEN excluded.name ELSE name END,
			last_used = CASE WHEN excluded.uses > 0 THEN excluded.last_used ELSE last_used END`,
			userID, email, a.Name, weight, now)
	}
}

// Suggest finds contacts matching what was typed, the most used first.
func (m *MailAccounts) Suggest(ctx context.Context, userID int64, q string, limit int) ([]mail.Address, error) {
	like := "%" + strings.NewReplacer("%", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(q))) + "%"
	rows, err := m.db.QueryContext(ctx, `SELECT email, name FROM contacts WHERE user_id = ? AND (email LIKE ? OR lower(name) LIKE ?)
		ORDER BY uses DESC, last_used DESC LIMIT ?`, userID, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mail.Address{}
	for rows.Next() {
		var a mail.Address
		if err := rows.Scan(&a.Email, &a.Name); err == nil {
			out = append(out, a)
		}
	}
	return out, rows.Err()
}

// ─── Preferences ─────────────────────────────────────────

// Prefs returns the user's saved preferences as JSON, or "".
func (m *MailAccounts) Prefs(ctx context.Context, userID int64) string {
	var s string
	_ = m.db.QueryRowContext(ctx, `SELECT prefs FROM user_prefs WHERE user_id = ?`, userID).Scan(&s)
	return s
}

// SavePrefs stores the user's preferences.
func (m *MailAccounts) SavePrefs(ctx context.Context, userID int64, prefs string) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO user_prefs (user_id, prefs, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET prefs = excluded.prefs, updated_at = excluded.updated_at`, userID, prefs, time.Now().UnixMilli())
	return err
}

// ─── Snooze ──────────────────────────────────────────────

// SnoozedName is the folder snoozed messages wait in.
const SnoozedName = "Snoozed"

// Snooze moves messages to the Snoozed folder until a time.
func (m *MailAccounts) Snooze(ctx context.Context, userID, account int64, ids []string, until time.Time) (map[string]string, error) {
	p, _, err := m.Provider(ctx, userID, account)
	if err != nil {
		return nil, err
	}
	box, err := p.EnsureMailbox(ctx, SnoozedName)
	if err != nil {
		return nil, err
	}
	moved, err := p.Move(ctx, ids, box)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	for _, id := range ids {
		if n, ok := moved[id]; ok {
			id = n
		}
		if _, err := m.db.ExecContext(ctx, `INSERT INTO snoozes (user_id, account_id, message_id, until, created_at) VALUES (?, ?, ?, ?, ?)`,
			userID, account, id, until.UnixMilli(), now); err != nil {
			return nil, err
		}
	}
	m.forgetBoxes(account)
	return moved, nil
}

// Unsnooze forgets snoozes of messages moved out of Snoozed by hand.
func (m *MailAccounts) Unsnooze(ctx context.Context, userID, account int64, ids []string) {
	for _, id := range ids {
		_, _ = m.db.ExecContext(ctx, `DELETE FROM snoozes WHERE user_id = ? AND account_id = ? AND message_id = ?`, userID, account, id)
	}
}

// Wake returns snoozed messages whose time came to the inbox, unread.
// It runs every minute (see app.go).
func (m *MailAccounts) Wake(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, `SELECT id, user_id, account_id, message_id FROM snoozes WHERE until <= ?`, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	type due struct {
		id, user, account int64
		msg               string
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.user, &d.account, &d.msg); err == nil {
			list = append(list, d)
		}
	}
	rows.Close()
	for _, d := range list {
		p, _, err := m.Provider(ctx, d.user, d.account)
		if err == nil {
			var inbox string
			if inbox, err = m.RoleBox(ctx, d.user, d.account, mail.RoleInbox); err == nil {
				var moved map[string]string
				if moved, err = p.Move(ctx, []string{d.msg}, inbox); err == nil {
					id := d.msg
					if n, ok := moved[id]; ok {
						id = n
					}
					f := false
					_ = p.SetFlags(ctx, []string{id}, &f, nil)
				}
			}
		}
		if err != nil && !errors.Is(err, mail.ErrNotFound) && !errors.Is(err, ErrNoAccount) {
			m.logger.Warn("mail: waking a snoozed message", "account", d.account, "err", err)
			continue // try again next minute
		}
		_, _ = m.db.ExecContext(ctx, `DELETE FROM snoozes WHERE id = ?`, d.id)
		m.forgetBoxes(d.account)
		_ = m.Hub.Publish("user:"+strconv.FormatInt(d.user, 10), Change{AccountID: d.account})
	}
	return nil
}

func (m *MailAccounts) forgetBoxes(account int64) {
	boxCache.Lock()
	delete(boxCache.m, account)
	boxCache.Unlock()
}
