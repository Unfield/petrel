package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/knadh/go-pop3"
)

// pop3Provider reads the server's single mailbox. POP3 has no flags,
// folders or search, so Petrel keeps seen and flagged in its own
// database, searches the page it fetched, and can't move messages.
type pop3Provider struct {
	s  Settings
	c  Credentials
	o  Options
	mu sync.Mutex // POP3 servers lock the mailbox per session
}

func newPOP3(s Settings, c Credentials, o Options) *pop3Provider {
	return &pop3Provider{s: s, c: c, o: o}
}

const pop3Inbox = "INBOX"

type guardedDialer struct {
	ctx          context.Context
	allowPrivate bool
}

func (d guardedDialer) Dial(network, address string) (net.Conn, error) {
	return dialer(d.allowPrivate).DialContext(d.ctx, network, address)
}

func (p *pop3Provider) session(ctx context.Context, fn func(c *pop3.Conn) error) error {
	if p.s.Incoming.Security == StartTLS {
		return fmt.Errorf("mail: POP3 with STARTTLS isn't supported yet: use port 995 with TLS")
	}
	if p.s.Incoming.Security == Plain && !p.o.AllowPrivate {
		return fmt.Errorf("mail: unencrypted connections are only allowed for local test servers")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	client := pop3.New(pop3.Opt{
		Host: p.s.Incoming.Host, Port: p.s.Incoming.Port, TLSEnabled: p.s.Incoming.Security == TLS,
		DialTimeout: 15 * time.Second, Dialer: guardedDialer{ctx: ctx, allowPrivate: p.o.AllowPrivate},
	})
	conn, err := client.NewConn()
	if err != nil {
		return fmt.Errorf("mail: connecting to %s: %w", p.s.Incoming.Host, err)
	}
	defer conn.Quit()
	if err := conn.Auth(p.c.Username, p.c.Password); err != nil {
		return ErrAuth
	}
	return fn(conn)
}

func encUIDL(u string) string { return base64.RawURLEncoding.EncodeToString([]byte(u)) }
func decUIDL(id string) string {
	b, _ := base64.RawURLEncoding.DecodeString(id)
	return string(b)
}

func (p *pop3Provider) Mailboxes(ctx context.Context) ([]Mailbox, error) {
	var box Mailbox
	err := p.session(ctx, func(c *pop3.Conn) error {
		ids, err := c.Uidl(0)
		if err != nil {
			return err
		}
		uidls := make([]string, 0, len(ids))
		for _, id := range ids {
			uidls = append(uidls, id.UID)
		}
		flags, _ := p.flags(ctx, uidls)
		box = Mailbox{ID: encBox(pop3Inbox), Name: "Inbox", Role: RoleInbox, Total: len(ids)}
		for _, u := range uidls {
			if !flags[u][0] {
				box.Unread++
			}
		}
		return nil
	})
	return []Mailbox{box}, err
}

func (p *pop3Provider) flags(ctx context.Context, uidls []string) (map[string][2]bool, error) {
	if p.o.Flags == nil {
		return map[string][2]bool{}, nil
	}
	return p.o.Flags.Flags(ctx, uidls)
}

func (p *pop3Provider) Query(ctx context.Context, q Query) (Page, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	page := Page{Messages: []Summary{}}
	if q.MailboxID != "" && q.MailboxID != encBox(pop3Inbox) {
		return page, nil
	}
	err := p.session(ctx, func(c *pop3.Conn) error {
		ids, err := c.Uidl(0)
		if err != nil {
			return err
		}
		slices.Reverse(ids) // the newest last on the server
		uidls := make([]string, 0, len(ids))
		for _, id := range ids {
			uidls = append(uidls, id.UID)
		}
		flags, _ := p.flags(ctx, uidls)
		filtered := q.Unread || q.Flagged || q.HasAttachment || q.Search != "" || !q.Since.IsZero()
		for i, id := range ids {
			f := flags[id.UID]
			if (q.Unread && f[0]) || (q.Flagged && !f[1]) {
				continue
			}
			if !filtered && (i < q.Offset || i >= q.Offset+q.Limit) {
				continue
			}
			// TOP gives the header and the first lines of the body.
			e, err := c.Top(id.ID, 40)
			if err != nil {
				continue
			}
			var raw bytes.Buffer
			_ = e.WriteTo(&raw)
			pr, _ := parse(&raw, false)
			s := p.summary(id.UID, pr, f)
			if q.HasAttachment && !s.HasAttachment || !q.Since.IsZero() && s.Date.Before(q.Since) || !matchText(s, q.Search) {
				continue
			}
			page.Messages = append(page.Messages, s)
			if filtered && len(page.Messages) >= q.Offset+q.Limit {
				break
			}
		}
		page.Total = len(ids)
		if filtered {
			page.Total = len(page.Messages)
			if q.Offset < len(page.Messages) {
				page.Messages = page.Messages[q.Offset:]
			} else {
				page.Messages = []Summary{}
			}
		}
		return nil
	})
	return page, err
}

func matchText(s Summary, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	hay := strings.ToLower(s.Subject + " " + s.Preview)
	for _, a := range s.From {
		hay += " " + strings.ToLower(a.Name+" "+a.Email)
	}
	for _, w := range strings.Fields(q) {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

func (p *pop3Provider) summary(uidl string, pr *parsed, f [2]bool) Summary {
	s := Summary{ID: encUIDL(uidl), MailboxID: encBox(pop3Inbox), From: []Address{}, To: []Address{}, Cc: []Address{}, Seen: f[0], Flagged: f[1]}
	if pr == nil {
		s.ThreadID = s.ID
		return s
	}
	h := pr.header
	s.From, s.To, s.Cc = addresses(h, "From"), addresses(h, "To"), addresses(h, "Cc")
	s.Subject, _ = h.Subject()
	s.Date, _ = h.Date()
	s.MessageID, _ = h.MessageID()
	s.Preview = preview(pr.text, pr.html, 200)
	s.HasAttachment = hasAttachments(h)
	inReplyTo := ""
	if ids := msgIDs(h, "In-Reply-To"); len(ids) > 0 {
		inReplyTo = ids[0]
	}
	s.ThreadID = threadOf(s.MessageID, inReplyTo, msgIDs(h, "References"))
	if s.ThreadID == "" {
		s.ThreadID = s.ID
	}
	return s
}

// find returns the message number for a UIDL: numbers change between
// sessions, UIDLs don't.
func find(c *pop3.Conn, uidl string) (int, error) {
	ids, err := c.Uidl(0)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if id.UID == uidl {
			return id.ID, nil
		}
	}
	return 0, ErrNotFound
}

func (p *pop3Provider) raw(ctx context.Context, id string) (uidl string, pr *parsed, keep bool, err error) {
	uidl = decUIDL(id)
	err = p.session(ctx, func(c *pop3.Conn) error {
		n, err := find(c, uidl)
		if err != nil {
			return err
		}
		buf, err := c.RetrRaw(n)
		if err != nil {
			return err
		}
		pr, err = parse(bytes.NewReader(buf.Bytes()), true)
		return err
	})
	return uidl, pr, true, err
}

func (p *pop3Provider) Get(ctx context.Context, id string) (*Message, error) {
	uidl, pr, _, err := p.raw(ctx, id)
	if err != nil {
		return nil, err
	}
	flags, _ := p.flags(ctx, []string{uidl})
	m := &Message{Summary: p.summary(uidl, pr, flags[uidl]), HTML: pr.html, Text: pr.text, Attachments: pr.attachments}
	m.ReplyTo, m.Bcc = addresses(pr.header, "Reply-To"), addresses(pr.header, "Bcc")
	m.References = msgIDs(pr.header, "References")
	if ids := msgIDs(pr.header, "In-Reply-To"); len(ids) > 0 {
		m.InReplyTo = ids[0]
	}
	if m.Attachments == nil {
		m.Attachments = []Attachment{}
	}
	return m, nil
}

func (p *pop3Provider) Attachment(ctx context.Context, id, attID string) (Attachment, io.ReadCloser, error) {
	_, pr, _, err := p.raw(ctx, id)
	if err != nil {
		return Attachment{}, nil, err
	}
	part := findPart(pr, attID)
	if part == nil {
		return Attachment{}, nil, ErrNotFound
	}
	return part.att, io.NopCloser(bytes.NewReader(part.body)), nil
}

func (p *pop3Provider) SetFlags(ctx context.Context, ids []string, seen, flagged *bool) error {
	if p.o.Flags == nil {
		return ErrUnsupported
	}
	uidls := make([]string, 0, len(ids))
	for _, id := range ids {
		uidls = append(uidls, decUIDL(id))
	}
	return p.o.Flags.SetFlags(ctx, uidls, seen, flagged)
}

func (p *pop3Provider) Move(context.Context, []string, string) (map[string]string, error) {
	return nil, ErrUnsupported
}

func (p *pop3Provider) Destroy(ctx context.Context, ids []string) error {
	return p.session(ctx, func(c *pop3.Conn) error {
		all, err := c.Uidl(0)
		if err != nil {
			return err
		}
		var nums []int
		for _, id := range ids {
			u := decUIDL(id)
			for _, a := range all {
				if a.UID == u {
					nums = append(nums, a.ID)
				}
			}
		}
		if len(nums) == 0 {
			return nil
		}
		return c.Dele(nums...) // takes effect at QUIT
	})
}

func (p *pop3Provider) Send(ctx context.Context, m Outgoing) error {
	_, err := sendSMTP(ctx, p.s.SMTP, p.c, p.o.AllowPrivate, m)
	return err
}

func (p *pop3Provider) State(ctx context.Context) (state string, err error) {
	err = p.session(ctx, func(c *pop3.Conn) error {
		n, size, err := c.Stat()
		state = fmt.Sprint(n, size)
		return err
	})
	return state, err
}

func (p *pop3Provider) Close() error { return nil }
