package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/sealed"

	"petrel/internal/mail"
)

// Petrel signs people in one of two ways, per installation:
//
//   - accounts (the default): Petrel accounts, each with any number of
//     connected mailboxes.
//   - direct: people sign in with their mailbox's address and password,
//     checked against the servers the admin configured, like Roundcube.
//     Petrel keeps the mailbox password (sealed) only while signed in.

// DirectServer is a domain's servers, for direct sign-in. Domain "*"
// matches every address.
type DirectServer struct {
	Domain string `json:"domain"`
	mail.Settings
}

// AuthMode reads PETREL_AUTH_MODE.
func AuthMode() string {
	if os.Getenv("PETREL_AUTH_MODE") == "direct" {
		return "direct"
	}
	return "accounts"
}

// Direct reports whether people sign in with their mailbox.
func (a *App) Direct() bool { return a.direct != nil }

func loadDirect() ([]DirectServer, error) {
	if AuthMode() != "direct" {
		return nil, nil
	}
	raw := os.Getenv("PETREL_DIRECT_SERVERS")
	if strings.HasPrefix(strings.TrimSpace(raw), "@") {
		b, err := os.ReadFile(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	var list []DirectServer
	if err := json.Unmarshal([]byte(raw), &list); err != nil || len(list) == 0 {
		return nil, fmt.Errorf("app: PETREL_AUTH_MODE=direct needs PETREL_DIRECT_SERVERS, a JSON list of servers (see README): %v", err)
	}
	return list, nil
}

func (a *App) directFor(email string) (mail.Settings, bool) {
	domain := strings.ToLower(email[strings.LastIndexByte(email, '@')+1:])
	for _, pass := range []bool{false, true} {
		for _, d := range a.direct {
			if (!pass && strings.EqualFold(d.Domain, domain)) || (pass && d.Domain == "*") {
				s := d.Settings
				s.Username = email
				return s, true
			}
		}
	}
	return mail.Settings{}, false
}

// DirectSignIn checks a mailbox's address and password with its server,
// then signs in as that mailbox, making the Petrel user the first time.
func (a *App) DirectSignIn(ctx *kit.Ctx, email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	s, ok := a.directFor(email)
	if !ok {
		return kit.FieldError("email", ctx.T("e.domain"))
	}
	c := mail.Credentials{Username: email, Password: password}
	if err := a.Mail.Test(ctx, s, c); err != nil {
		if errors.Is(err, mail.ErrAuth) {
			return kit.FormError(ctx.T("e.credentials"))
		}
		return kit.FormError(ctx.T("e.unreachable"))
	}
	local := email[:strings.IndexByte(email, '@')]
	u, err := a.Accounts.SignInWith(ctx, accounts.Identity{Provider: "mail", Subject: email, Email: email, EmailVerified: true, Name: local})
	if err != nil {
		return accounts.FormError(ctx, err)
	}
	return a.Mail.Upsert(ctx, u.ID, email, s, c)
}

// DirectSignOut forgets the mailbox password when a direct user signs out.
func (a *App) DirectSignOut(ctx context.Context, userID int64) {
	if !a.Direct() {
		return
	}
	list, _ := a.Mail.List(ctx, userID)
	for _, acc := range list {
		_ = a.Mail.Delete(ctx, userID, acc.ID)
	}
}

// Upsert saves a mailbox, or updates its settings and credentials if the
// user has it already.
func (m *MailAccounts) Upsert(ctx context.Context, userID int64, email string, s mail.Settings, c mail.Credentials) error {
	settings, _ := json.Marshal(s)
	secret, _ := json.Marshal(c)
	res, err := m.db.ExecContext(ctx, `UPDATE mail_accounts SET settings = ?, secret = ?, protocol = ?, updated_at = ? WHERE user_id = ? AND email = ?`,
		string(settings), sealed.String(secret), string(s.Protocol), time.Now().UnixMilli(), userID, strings.ToLower(email))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = m.Create(ctx, userID, email, "", s, c)
	return err
}

// saveCreds stores renewed credentials (an OAuth token) for an account.
func (m *MailAccounts) saveCreds(ctx context.Context, id int64, c mail.Credentials) error {
	secret, _ := json.Marshal(c)
	// updated_at stays: the cached connection keeps using the renewed token.
	_, err := m.db.ExecContext(ctx, `UPDATE mail_accounts SET secret = ? WHERE id = ?`, sealed.String(secret), id)
	return err
}

var _ = http.StatusOK
