package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"petrel/internal/mail"
)

// Gmail and Outlook don't take passwords from new apps: they need OAuth,
// with a client registered at Google or Microsoft. Set
// PETREL_GOOGLE_CLIENT_ID/_SECRET or PETREL_MICROSOFT_CLIENT_ID/_SECRET,
// with <BASE_URL>/api/mail/oauth/callback as the redirect URL.

// MailProvider is a mail service Petrel can connect with OAuth.
type MailProvider struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	config   *oauth2.Config
	settings mail.Settings
}

func mailProviders() map[string]*MailProvider {
	base := strings.TrimSuffix(os.Getenv("BASE_URL"), "/")
	redirect := base + "/api/mail/oauth/callback"
	out := map[string]*MailProvider{}
	if id := os.Getenv("PETREL_GOOGLE_CLIENT_ID"); id != "" {
		out["google"] = &MailProvider{Name: "google", Label: "Gmail",
			config: &oauth2.Config{ClientID: id, ClientSecret: os.Getenv("PETREL_GOOGLE_CLIENT_SECRET"), RedirectURL: redirect,
				Scopes:   []string{"https://mail.google.com/", "openid", "email", "profile"},
				Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token"}},
			settings: mail.Settings{Protocol: mail.IMAP,
				Incoming: mail.Server{Host: "imap.gmail.com", Port: 993, Security: mail.TLS},
				SMTP:     mail.Server{Host: "smtp.gmail.com", Port: 465, Security: mail.TLS}}}
	}
	if id := os.Getenv("PETREL_MICROSOFT_CLIENT_ID"); id != "" {
		out["microsoft"] = &MailProvider{Name: "microsoft", Label: "Outlook",
			config: &oauth2.Config{ClientID: id, ClientSecret: os.Getenv("PETREL_MICROSOFT_CLIENT_SECRET"), RedirectURL: redirect,
				Scopes: []string{"https://outlook.office.com/IMAP.AccessAsUser.All", "https://outlook.office.com/SMTP.Send", "offline_access", "openid", "email", "profile"},
				Endpoint: oauth2.Endpoint{AuthURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
					TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token"}},
			settings: mail.Settings{Protocol: mail.IMAP,
				Incoming: mail.Server{Host: "outlook.office365.com", Port: 993, Security: mail.TLS},
				SMTP:     mail.Server{Host: "smtp.office365.com", Port: 587, Security: mail.StartTLS}}}
	}
	return out
}

// OAuthProviders lists the services set up for OAuth, for buttons.
func (a *App) OAuthProviders() []MailProvider {
	var out []MailProvider
	for _, n := range []string{"google", "microsoft"} {
		if p, ok := a.oauth[n]; ok {
			out = append(out, MailProvider{Name: p.Name, Label: p.Label})
		}
	}
	return out
}

// OAuthStart returns where to send the user to grant access, with the
// state and PKCE verifier to keep until they come back.
func (a *App) OAuthStart(provider string) (url, state, verifier string, err error) {
	p, ok := a.oauth[provider]
	if !ok {
		return "", "", "", errors.New("app: that provider isn't set up")
	}
	state, verifier = oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
	opts := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier)}
	if provider == "google" {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "consent"))
	}
	return p.config.AuthCodeURL(state, opts...), state, verifier, nil
}

// OAuthFinish trades the code for tokens and connects the mailbox.
func (a *App) OAuthFinish(ctx context.Context, userID int64, provider, code, verifier string) (string, error) {
	p, ok := a.oauth[provider]
	if !ok {
		return "", errors.New("app: that provider isn't set up")
	}
	tok, err := p.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", err
	}
	email, name := idTokenEmail(tok)
	if email == "" {
		return "", errors.New("app: the provider didn't share the mailbox's address")
	}
	s := p.settings
	s.Username = email
	c := mail.Credentials{Username: email, OAuth: &mail.OAuth{Provider: provider, AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, Expiry: tok.Expiry}}
	if err := a.Mail.Test(ctx, s, c); err != nil {
		return "", err
	}
	if err := a.Mail.Upsert(ctx, userID, email, s, c); err != nil {
		return "", err
	}
	if name != "" {
		_, _ = a.DB.ExecContext(ctx, `UPDATE mail_accounts SET display_name = ? WHERE user_id = ? AND email = ? AND display_name = ''`, name, userID, email)
	}
	return email, nil
}

// idTokenEmail reads the address from the ID token. It came straight
// from the provider's token endpoint over TLS, so its signature isn't
// checked again here.
func idTokenEmail(tok *oauth2.Token) (email, name string) {
	raw, _ := tok.Extra("id_token").(string)
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", ""
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ""
	}
	var claims struct {
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	_ = json.Unmarshal(b, &claims)
	if claims.Email == "" {
		claims.Email = claims.PreferredUsername
	}
	return strings.ToLower(claims.Email), claims.Name
}

// refresher renews an account's OAuth token and saves it.
func (m *MailAccounts) refresher(id int64, providers map[string]*MailProvider, c *mail.Credentials) func(ctx context.Context, o *mail.OAuth) error {
	return func(ctx context.Context, o *mail.OAuth) error {
		p, ok := providers[o.Provider]
		if !ok {
			return errors.New("app: OAuth provider not set up")
		}
		tok, err := p.config.TokenSource(ctx, &oauth2.Token{RefreshToken: o.RefreshToken, Expiry: time.Unix(0, 0)}).Token()
		if err != nil {
			return err
		}
		o.AccessToken, o.Expiry = tok.AccessToken, tok.Expiry
		if tok.RefreshToken != "" {
			o.RefreshToken = tok.RefreshToken
		}
		c.OAuth = o
		return m.saveCreds(ctx, id, *c)
	}
}
