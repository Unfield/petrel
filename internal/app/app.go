// Package app holds what the routes share: the database, user accounts,
// connected mail accounts, email and background jobs.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	_ "modernc.org/sqlite"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/jobs"
	"github.com/Unfield/Tern/mail"
	"github.com/Unfield/Tern/plugins/oauth"
)

// App is the running app's shared parts.
type App struct {
	DB       *sql.DB
	Accounts *accounts.Accounts
	Jobs     *jobs.Queue
	// OAuth is sign-in with GitHub and Google, when their keys are set.
	OAuth *oauth.Plugin
	// Providers are OAuth's providers, for the sign-in page's buttons.
	Providers []Provider
	Mail      *MailAccounts

	direct []DirectServer
	oauth  map[string]*MailProvider
}

// Provider is a sign-in button.
type Provider struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// Current is the app, for loads and actions.
var Current *App

// Open opens the database and sets up accounts, email and sign-in with
// other providers, from the environment.
func Open(logger *slog.Logger) (*App, error) {
	dbPath := env("DATABASE_PATH", "app.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	if err := setUpKeys(dbPath, logger); err != nil {
		return nil, err
	}
	q, err := jobs.New(db, jobs.Options{Logger: logger})
	if err != nil {
		return nil, err
	}
	acc, err := accounts.New(db, accounts.Options{
		Send:    sender(logger, q),
		BaseURL: os.Getenv("BASE_URL"),
		RPName:  env("APP_NAME", "Petrel"),
	})
	if err != nil {
		return nil, err
	}
	if err := acc.Migrate(context.Background()); err != nil {
		return nil, err
	}
	// PETREL_ALLOW_PRIVATE_HOSTS lets mailboxes on private addresses and any
	// port connect: for local test servers, never on a public instance.
	allowPrivate := os.Getenv("PETREL_ALLOW_PRIVATE_HOSTS") == "true"
	a := &App{DB: db, Accounts: acc, Jobs: q, Mail: NewMailAccounts(db, logger, allowPrivate), oauth: mailProviders()}
	a.Mail.oauth = a.oauth
	if a.direct, err = loadDirect(); err != nil {
		return nil, err
	}
	// Snoozed mail wakes on time; unsent uploads go after a day.
	if err := q.Schedule("wake-snoozed", "@every 1m", a.Mail.Wake); err != nil {
		return nil, err
	}
	if err := q.Schedule("drop-uploads", "@every 1h", func(ctx context.Context) error {
		a.Mail.DropUploads(ctx, 0, nil)
		return nil
	}); err != nil {
		return nil, err
	}
	if err := a.setUpOAuth(); err != nil {
		return nil, err
	}
	Current = a
	return a, nil
}

// sender delivers account emails: with SMTP_HOST, through SMTP in a
// background job, so they're retried; without it, to the log, link
// included, for development.
func sender(logger *slog.Logger, q *jobs.Queue) func(context.Context, accounts.Email) error {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return func(_ context.Context, e accounts.Email) error {
			logger.Info("email (set SMTP_HOST to send it)", "to", e.To, "subject", e.Subject, "link", e.Link)
			return nil
		}
	}
	port, err := strconv.Atoi(env("SMTP_PORT", "587"))
	if err != nil {
		port = 587
	}
	mailer := mail.New(mail.SMTP{Host: host, Port: port, Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD")},
		env("MAIL_FROM", "Petrel <noreply@localhost>"))
	return accounts.SendWith(mailer, q)
}

func (a *App) setUpOAuth() error {
	var providers []*oauth.Provider
	if id := os.Getenv("GITHUB_CLIENT_ID"); id != "" {
		providers = append(providers, oauth.GitHub(id, os.Getenv("GITHUB_CLIENT_SECRET")))
		a.Providers = append(a.Providers, Provider{Name: "github", Label: "GitHub"})
	}
	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		providers = append(providers, oauth.Google(id, os.Getenv("GOOGLE_CLIENT_SECRET")))
		a.Providers = append(a.Providers, Provider{Name: "google", Label: "Google"})
	}
	if len(providers) == 0 {
		return nil
	}
	secret := os.Getenv("OAUTH_SECRET")
	if len(secret) < 32 {
		return fmt.Errorf("app: set OAUTH_SECRET to at least 32 random characters to sign in with GitHub or Google")
	}
	a.OAuth = oauth.New(oauth.Options{
		Secret:    secret,
		Providers: providers,
		Accounts:  a.Accounts,
		URL:       os.Getenv("BASE_URL"),
	})
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
