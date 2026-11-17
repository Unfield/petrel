// Package mail talks to mail servers. Every protocol (JMAP, IMAP, POP3)
// implements Provider, a small interface shaped after JMAP's model:
// mailboxes with roles, messages with keywords, and queries that page.
// The rest of Petrel only ever sees this interface.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"time"
)

// Protocol is how Petrel reads a mailbox.
type Protocol string

const (
	JMAP Protocol = "jmap"
	IMAP Protocol = "imap"
	POP3 Protocol = "pop3"
)

// Security is how a connection to a server is protected.
type Security string

const (
	TLS      Security = "tls"      // TLS from the start (993, 995, 465)
	StartTLS Security = "starttls" // upgraded with STARTTLS (143, 587)
	Plain    Security = "plain"    // unencrypted: only for local test servers
)

// Server is a host to connect to.
type Server struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Security Security `json:"security"`
}

// Settings say where a mailbox lives. They are stored as JSON beside the
// account; the credentials are stored apart, encrypted.
type Settings struct {
	Protocol Protocol `json:"protocol"`
	Username string   `json:"username"`
	// Incoming is the IMAP or POP3 server.
	Incoming Server `json:"incoming"`
	// SMTP sends mail for IMAP and POP3 accounts.
	SMTP Server `json:"smtp"`
	// JMAPURL is the JMAP session resource, such as
	// https://example.com/.well-known/jmap.
	JMAPURL string `json:"jmapUrl,omitempty"`
	// Sieve is the ManageSieve server for filters, when it isn't the
	// incoming server's host on port 4190.
	Sieve *Server `json:"sieve,omitempty"`
}

// Credentials sign in to the servers: a password, or an OAuth token for
// providers that require one (Gmail, Outlook).
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	OAuth    *OAuth `json:"oauth,omitempty"`
}

// OAuth is an access token and what renews it.
type OAuth struct {
	Provider     string    `json:"provider"`
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	Expiry       time.Time `json:"expiry"`
}

// Roles of special mailboxes (RFC 8621 and RFC 6154).
const (
	RoleInbox   = "inbox"
	RoleSent    = "sent"
	RoleDrafts  = "drafts"
	RoleTrash   = "trash"
	RoleJunk    = "junk"
	RoleArchive = "archive"
)

// Mailbox is a folder.
type Mailbox struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
	Total  int    `json:"total"`
	Unread int    `json:"unread"`
	// Path is the full name on the server, such as "Work/Clients", which
	// Sieve's fileinto uses.
	Path string `json:"path,omitempty"`
}

// Address is a person or a mailbox.
type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// Summary is what the message list shows.
type Summary struct {
	ID            string    `json:"id"`
	MailboxID     string    `json:"mailboxId"`
	ThreadID      string    `json:"threadId"`
	MessageID     string    `json:"-"`
	From          []Address `json:"from"`
	To            []Address `json:"to"`
	Cc            []Address `json:"cc"`
	Subject       string    `json:"subject"`
	Preview       string    `json:"preview"`
	Date          time.Time `json:"date"`
	Seen          bool      `json:"seen"`
	Flagged       bool      `json:"flagged"`
	Answered      bool      `json:"answered"`
	HasAttachment bool      `json:"hasAttachment"`
	// Keywords are the message's labels: IMAP keywords, JMAP keywords
	// that aren't system ones ($seen, $flagged…).
	Keywords []string `json:"keywords"`
}

// Attachment is a file in a message, or an inline image.
type Attachment struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	CID    string `json:"cid,omitempty"`
	Inline bool   `json:"inline"`
}

// Message is a whole message, for reading it.
type Message struct {
	Summary
	ReplyTo     []Address    `json:"replyTo"`
	Bcc         []Address    `json:"bcc"`
	HTML        string       `json:"-"`
	Text        string       `json:"-"`
	Attachments []Attachment `json:"attachments"`
	InReplyTo   string       `json:"-"`
	References  []string     `json:"-"`
	// Unsubscribe is how to leave the mailing list it came from, if any.
	Unsubscribe *Unsubscribe `json:"unsubscribe,omitempty"`
	// Autocrypt is the sender's Autocrypt header, with their OpenPGP key.
	Autocrypt string `json:"-"`
	// PGP: the message is PGP/MIME encrypted or signed, or holds inline
	// PGP, so Petrel reads its original to open it.
	PGP bool `json:"-"`
}

// Unsubscribe is a message's List-Unsubscribe (RFC 2369): a web address,
// an email address, or both. OneClick means the web address takes a POST
// and needs no page (RFC 8058).
type Unsubscribe struct {
	URL      string `json:"url,omitempty"`
	Mailto   string `json:"mailto,omitempty"`
	OneClick bool   `json:"oneClick,omitempty"`
}

// Query selects messages. MailboxID "" with Flagged set means flagged
// messages in every mailbox but trash and junk ("Starred").
type Query struct {
	MailboxID     string
	Search        string
	Unread        bool
	Flagged       bool
	HasAttachment bool
	Keyword       string
	Since         time.Time
	Offset        int
	Limit         int
}

// Page is a page of a query's results, newest first.
type Page struct {
	Messages []Summary `json:"messages"`
	Total    int       `json:"total"`
}

// Outgoing is a message to send.
type Outgoing struct {
	From        Address
	ReplyTo     []Address
	To          []Address
	Cc          []Address
	Bcc         []Address
	Subject     string
	Text        string
	HTML        string
	InReplyTo   string
	References  []string
	Attachments []File
	// Headers are more header fields, such as Autocrypt.
	Headers map[string]string
	// Seal, if set, wraps the message's content (a MIME entity) before
	// sending, for PGP/MIME: it returns the outer Content-Type and body.
	Seal Seal
}

// Seal wraps a MIME entity.
type Seal func(entity []byte) (contentType string, params map[string]string, body []byte, err error)

// File is an attachment to send.
type File struct {
	Name string
	Type string
	Data []byte
}

// Provider reads and changes one mail account.
type Provider interface {
	Mailboxes(ctx context.Context) ([]Mailbox, error)
	Query(ctx context.Context, q Query) (Page, error)
	Get(ctx context.Context, id string) (*Message, error)
	Attachment(ctx context.Context, messageID, attachmentID string) (Attachment, io.ReadCloser, error)
	SetFlags(ctx context.Context, ids []string, seen, flagged *bool) error
	// Move returns the messages' new IDs where moving changes them (IMAP
	// gives a message a new UID in each mailbox), by old ID.
	Move(ctx context.Context, ids []string, mailboxID string) (map[string]string, error)
	Destroy(ctx context.Context, ids []string) error
	Send(ctx context.Context, m Outgoing) error
	// SaveDraft files a draft in the Drafts mailbox, replacing the draft
	// with ID replace if given, and returns the new draft's ID.
	SaveDraft(ctx context.Context, m Outgoing, replace string) (string, error)
	// SetKeywords adds and removes labels.
	SetKeywords(ctx context.Context, ids []string, add, remove []string) error
	// EnsureMailbox returns the ID of a mailbox with this name, creating it
	// if needed (such as "Snoozed").
	EnsureMailbox(ctx context.Context, name string) (string, error)
	// State changes whenever the account's messages change; watchers
	// compare it to notice new mail on servers without push.
	State(ctx context.Context) (string, error)
	Close() error
}

// Watcher is a Provider that can wait for changes itself (IMAP IDLE).
type Watcher interface {
	// Watch calls changed whenever the inbox changes, until ctx is done.
	Watch(ctx context.Context, changed func()) error
}

var (
	// ErrUnsupported is returned for what a protocol can't do, such as
	// moving messages in POP3.
	ErrUnsupported = errors.New("mail: not supported by this account's protocol")
	// ErrNotFound is returned for messages that are gone.
	ErrNotFound = errors.New("mail: message not found")
	// ErrAuth is returned when the server refuses the credentials.
	ErrAuth = errors.New("mail: the server refused the username or password")
)

// Options configure providers.
type Options struct {
	// AllowPrivate lets connections reach private and loopback addresses
	// and any port: for local test servers only.
	AllowPrivate bool
	// Flags keeps seen and flagged for POP3, which has neither.
	Flags FlagStore
	// Refresh renews an expired OAuth token in place and saves it.
	Refresh func(ctx context.Context, o *OAuth) error
	// Transport replaces how JMAP requests are made: for tests that reach
	// a server under another address than the one it announces.
	Transport http.RoundTripper
	// TLS, if set, is the base TLS configuration (for tests that trust a
	// test server's own certificate). The server name is still checked.
	TLS *tls.Config
}

// token returns a current OAuth access token, renewing it first if it
// expires within a minute.
func token(ctx context.Context, c *Credentials, o Options) (string, error) {
	if c.OAuth == nil {
		return "", nil
	}
	if time.Until(c.OAuth.Expiry) < time.Minute && o.Refresh != nil {
		if err := o.Refresh(ctx, c.OAuth); err != nil {
			return "", ErrAuth
		}
	}
	return c.OAuth.AccessToken, nil
}

// FlagStore keeps POP3 messages' flags in Petrel's database.
type FlagStore interface {
	Flags(ctx context.Context, uidls []string) (map[string][2]bool, error)
	SetFlags(ctx context.Context, uidls []string, seen, flagged *bool) error
}

// Open connects nothing yet: providers dial on first use.
func Open(s Settings, c Credentials, o Options) (Provider, error) {
	switch s.Protocol {
	case IMAP:
		return newIMAP(s, c, o), nil
	case JMAP:
		return newJMAP(s, c, o), nil
	case POP3:
		return newPOP3(s, c, o), nil
	}
	return nil, errors.New("mail: unknown protocol " + string(s.Protocol))
}

// Quota is how much storage a mailbox uses, in bytes. Limit is 0 when the
// server reports usage without a limit.
type Quota struct {
	Used  int64 `json:"used"`
	Limit int64 `json:"limit"`
}

// QuotaProvider is a provider whose server can say how full the mailbox
// is: IMAP with QUOTA (RFC 9208), JMAP with Quota (RFC 9425). Quota
// returns ErrUnsupported when the server can't.
type QuotaProvider interface {
	Quota(ctx context.Context) (*Quota, error)
}
