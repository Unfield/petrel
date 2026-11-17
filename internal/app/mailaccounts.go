package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/realtime"
	"github.com/Unfield/Tern/sealed"

	"petrel/internal/mail"
)

// MailAccount is a mailbox connected to Petrel. Its server settings and
// encrypted credentials stay on the server and are never sent to the page.
type MailAccount struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Protocol    string `json:"protocol"`
	Color       string `json:"color"`
}

// Change tells a user's open tabs that an account's mail changed.
type Change struct {
	AccountID int64 `json:"accountId"`
}

// MailAccounts stores connected mailboxes (the mail_accounts table), keeps
// a connection open per mailbox in use, and watches them for new mail.
type MailAccounts struct {
	db     *sql.DB
	logger *slog.Logger
	opts   mail.Options

	// Hub carries Changes, by user ("user:<id>").
	Hub *realtime.Hub[Change]

	oauth map[string]*MailProvider

	mu        sync.Mutex
	providers map[int64]*cached
	watchers  map[int64]*watcher
}

type cached struct {
	p       mail.Provider
	version int64
	used    time.Time
}

type watcher struct {
	users  int
	cancel context.CancelFunc
}

// colors are given to accounts in turn, to tell them apart in lists.
var colors = []string{"#8b5cf6", "#0f9d8f", "#e8590c", "#3b82f6", "#e64980", "#2f9e44", "#f59f00"}

func NewMailAccounts(db *sql.DB, logger *slog.Logger, allowPrivate bool) *MailAccounts {
	m := &MailAccounts{
		db: db, logger: logger, Hub: realtime.NewHub[Change](),
		providers: map[int64]*cached{}, watchers: map[int64]*watcher{},
	}
	m.opts = mail.Options{AllowPrivate: allowPrivate, TLS: extraCAs(logger)}
	go m.closeIdle()
	return m
}

// AllowPrivate says whether servers on private addresses may be used.
func (m *MailAccounts) AllowPrivate() bool { return m.opts.AllowPrivate }

// List returns the user's mailboxes in the order they arranged them.
func (m *MailAccounts) List(ctx context.Context, userID int64) ([]MailAccount, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, email, display_name, protocol, color FROM mail_accounts WHERE user_id = ? ORDER BY position, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []MailAccount{}
	for rows.Next() {
		var a MailAccount
		if err := rows.Scan(&a.ID, &a.Email, &a.DisplayName, &a.Protocol, &a.Color); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// ErrNoAccount is returned for accounts that aren't the user's.
var ErrNoAccount = errors.New("app: no such mail account")

type stored struct {
	MailAccount
	settings mail.Settings
	creds    mail.Credentials
	version  int64
}

func (m *MailAccounts) get(ctx context.Context, userID, id int64) (*stored, error) {
	var s stored
	var settings string
	var secret sealed.String
	err := m.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, protocol, color, settings, secret, updated_at FROM mail_accounts WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&s.ID, &s.Email, &s.DisplayName, &s.Protocol, &s.Color, &settings, &secret, &s.version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoAccount
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(settings), &s.settings); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(secret), &s.creds); err != nil {
		return nil, err
	}
	return &s, nil
}

// Test signs in to the servers, so a typo shows before anything is saved.
func (m *MailAccounts) Test(ctx context.Context, s mail.Settings, c mail.Credentials) error {
	p, err := mail.Open(s, c, m.opts)
	if err != nil {
		return err
	}
	defer p.Close()
	_, err = p.Mailboxes(ctx)
	return err
}

// Create saves a mailbox for the user, its credentials sealed.
func (m *MailAccounts) Create(ctx context.Context, userID int64, email, name string, s mail.Settings, c mail.Credentials) (MailAccount, error) {
	settings, _ := json.Marshal(s)
	secret, _ := json.Marshal(c)
	var n int
	_ = m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mail_accounts WHERE user_id = ?`, userID).Scan(&n)
	a := MailAccount{Email: strings.ToLower(email), DisplayName: name, Protocol: string(s.Protocol), Color: colors[n%len(colors)]}
	now := time.Now().UnixMilli()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO mail_accounts (user_id, email, display_name, protocol, settings, secret, color, position, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, a.Email, a.DisplayName, a.Protocol, string(settings), sealed.String(secret), a.Color, n, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return MailAccount{}, errors.New(tr(ctx, "e.duplicate"))
		}
		return MailAccount{}, err
	}
	a.ID, _ = res.LastInsertId()
	return a, nil
}

// Delete disconnects a mailbox. Its mail stays on the server.
func (m *MailAccounts) Delete(ctx context.Context, userID, id int64) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM mail_accounts WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoAccount
	}
	_, _ = m.db.ExecContext(ctx, `DELETE FROM pop3_flags WHERE account_id = ?`, id)
	_, _ = m.db.ExecContext(ctx, `DELETE FROM identities WHERE account_id = ?`, id)
	_, _ = m.db.ExecContext(ctx, `DELETE FROM mail_rules WHERE account_id = ?`, id)
	forgetCalendars(id)
	m.mu.Lock()
	if c, ok := m.providers[id]; ok {
		c.p.Close()
		delete(m.providers, id)
	}
	if w, ok := m.watchers[id]; ok {
		w.cancel()
		delete(m.watchers, id)
	}
	m.mu.Unlock()
	return nil
}

// Provider returns the user's account and a connection to it, reused
// between requests.
func (m *MailAccounts) Provider(ctx context.Context, userID, id int64) (mail.Provider, *MailAccount, error) {
	s, err := m.get(ctx, userID, id)
	if err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.providers[id]; ok && c.version == s.version {
		c.used = time.Now()
		return c.p, &s.MailAccount, nil
	} else if ok {
		c.p.Close()
	}
	opts := m.opts
	opts.Flags = pop3Flags{db: m.db, account: id}
	if s.creds.OAuth != nil {
		creds := s.creds
		opts.Refresh = m.refresher(id, m.oauth, &creds)
	}
	p, err := mail.Open(s.settings, s.creds, opts)
	if err != nil {
		return nil, nil, err
	}
	m.providers[id] = &cached{p: p, version: s.version, used: time.Now()}
	return p, &s.MailAccount, nil
}

// closeIdle closes connections nobody used for ten minutes.
func (m *MailAccounts) closeIdle() {
	for range time.Tick(time.Minute) {
		m.mu.Lock()
		for id, c := range m.providers {
			if time.Since(c.used) > 10*time.Minute {
				c.p.Close()
				delete(m.providers, id)
			}
		}
		m.mu.Unlock()
	}
}

// Subscribe streams changes to the user's mail while ctx lasts, watching
// each of their accounts: IMAP with IDLE, the others by polling.
func (m *MailAccounts) Subscribe(ctx context.Context, userID int64) (<-chan Change, error) {
	ch, err := m.Hub.Subscribe(ctx, "user:"+strconv.FormatInt(userID, 10))
	if err != nil {
		return nil, err
	}
	accounts, err := m.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	for _, a := range accounts {
		w, ok := m.watchers[a.ID]
		if !ok {
			wctx, cancel := context.WithCancel(context.Background())
			w = &watcher{cancel: cancel}
			m.watchers[a.ID] = w
			go m.watch(wctx, userID, a.ID)
		}
		w.users++
	}
	m.mu.Unlock()
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, a := range accounts {
			if w, ok := m.watchers[a.ID]; ok {
				if w.users--; w.users <= 0 {
					w.cancel()
					delete(m.watchers, a.ID)
				}
			}
		}
	}()
	return ch, nil
}

func (m *MailAccounts) watch(ctx context.Context, userID, id int64) {
	topic := "user:" + strconv.FormatInt(userID, 10)
	publish := func() { _ = m.Hub.Publish(topic, Change{AccountID: id}) }
	p, _, err := m.Provider(ctx, userID, id)
	if err != nil {
		return
	}
	if w, ok := p.(mail.Watcher); ok {
		err := w.Watch(ctx, publish)
		if err == nil || ctx.Err() != nil {
			return
		}
		if !errors.Is(err, mail.ErrUnsupported) {
			m.logger.Warn("mail: watching stopped", "account", id, "err", err)
			return
		}
	}
	// No push: compare the account's state every 45 seconds.
	last := ""
	for {
		if p, _, err := m.Provider(ctx, userID, id); err == nil {
			if state, err := p.State(ctx); err == nil {
				if last != "" && state != last {
					publish()
				}
				last = state
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(45 * time.Second):
		}
	}
}

// pop3Flags keeps seen and flagged for a POP3 account.
type pop3Flags struct {
	db      *sql.DB
	account int64
}

func (f pop3Flags) Flags(ctx context.Context, uidls []string) (map[string][2]bool, error) {
	out := map[string][2]bool{}
	rows, err := f.db.QueryContext(ctx, `SELECT uidl, seen, flagged FROM pop3_flags WHERE account_id = ?`, f.account)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var u string
		var seen, flagged bool
		if err := rows.Scan(&u, &seen, &flagged); err == nil {
			out[u] = [2]bool{seen, flagged}
		}
	}
	return out, rows.Err()
}

func (f pop3Flags) SetFlags(ctx context.Context, uidls []string, seen, flagged *bool) error {
	for _, u := range uidls {
		if _, err := f.db.ExecContext(ctx,
			`INSERT INTO pop3_flags (account_id, uidl, seen, flagged) VALUES (?, ?, 0, 0) ON CONFLICT DO NOTHING`, f.account, u); err != nil {
			return err
		}
		if seen != nil {
			if _, err := f.db.ExecContext(ctx, `UPDATE pop3_flags SET seen = ? WHERE account_id = ? AND uidl = ?`, *seen, f.account, u); err != nil {
				return err
			}
		}
		if flagged != nil {
			if _, err := f.db.ExecContext(ctx, `UPDATE pop3_flags SET flagged = ? WHERE account_id = ? AND uidl = ?`, *flagged, f.account, u); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteUser removes everything Petrel keeps for a user whose account is
// deleted: their mailboxes and the passwords for them, contacts, settings,
// snoozes and uploads. Their mail stays on the mail servers.
func (m *MailAccounts) DeleteUser(ctx context.Context, userID int64) error {
	list, err := m.List(ctx, userID)
	if err != nil {
		return err
	}
	for _, a := range list {
		if err := m.Delete(ctx, userID, a.ID); err != nil && !errors.Is(err, ErrNoAccount) {
			return err
		}
	}
	for _, table := range []string{"uploads", "contacts", "user_prefs", "snoozes", "identities", "mail_rules", "pgp_keys", "pgp_public"} {
		if _, err := m.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE user_id = ?`, userID); err != nil {
			return err
		}
	}
	return nil
}
