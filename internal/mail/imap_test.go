package mail

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-smtp"
)

type lit struct {
	*bytes.Reader
	n int64
}

func (l lit) Size() int64 { return l.n }

// testServer runs IMAP and SMTP in memory, with SMTP delivering to INBOX.
func testServer(t *testing.T) (Settings, Credentials, *imapmemserver.User) {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("ada@example.test", "secret")
	for _, box := range []string{"INBOX", "Sent", "Archive", "Trash"} {
		if err := u.Create(box, nil); err != nil {
			t.Fatal(err)
		}
	}
	mem.AddUser(u)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	sln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ss := smtp.NewServer(deliverTo{u})
	ss.Domain, ss.AllowInsecureAuth = "localhost", true
	go ss.Serve(sln)
	t.Cleanup(func() { ss.Close() })

	port := func(l net.Listener) int { return l.Addr().(*net.TCPAddr).Port }
	s := Settings{Protocol: IMAP, Username: "ada@example.test",
		Incoming: Server{Host: "127.0.0.1", Port: port(ln), Security: Plain},
		SMTP:     Server{Host: "127.0.0.1", Port: port(sln), Security: Plain}}
	return s, Credentials{Username: "ada@example.test", Password: "secret"}, u
}

type deliverTo struct{ u *imapmemserver.User }

func (d deliverTo) NewSession(*smtp.Conn) (smtp.Session, error) { return &sess{u: d.u}, nil }

type sess struct{ u *imapmemserver.User }

func (s *sess) Mail(string, *smtp.MailOptions) error { return nil }
func (s *sess) Rcpt(string, *smtp.RcptOptions) error { return nil }
func (s *sess) Reset()                               {}
func (s *sess) Logout() error                        { return nil }
func (s *sess) Data(r io.Reader) error {
	b, _ := io.ReadAll(r)
	_, err := s.u.Append("INBOX", lit{bytes.NewReader(b), int64(len(b))}, &imap.AppendOptions{})
	return err
}

func appendRaw(t *testing.T, u *imapmemserver.User, box, raw string, flags ...imap.Flag) {
	t.Helper()
	b := []byte(strings.ReplaceAll(raw, "\n", "\r\n"))
	if _, err := u.Append(box, lit{bytes.NewReader(b), int64(len(b))}, &imap.AppendOptions{Flags: flags, Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestIMAP(t *testing.T) {
	ctx := context.Background()
	s, c, u := testServer(t)
	appendRaw(t, u, "INBOX", "From: Bob <bob@example.test>\nTo: ada@example.test\nSubject: Hello\nMessage-ID: <1@example.test>\nDate: Mon, 16 Nov 2026 10:00:00 +0000\n\nHi Ada, see https://example.com.\n")
	appendRaw(t, u, "INBOX", "From: Bob <bob@example.test>\nTo: ada@example.test\nSubject: Re: Hello\nMessage-ID: <2@example.test>\nIn-Reply-To: <1@example.test>\nReferences: <1@example.test>\nDate: Mon, 16 Nov 2026 11:00:00 +0000\nContent-Type: multipart/mixed; boundary=b\n\n--b\nContent-Type: text/html\n\n<p>Again <img src=\"https://tracker.example/x.gif\"><script>x()</script></p>\n--b\nContent-Type: application/pdf\nContent-Disposition: attachment; filename=a.pdf\n\n%PDF\n--b--\n", imap.FlagSeen)

	p, err := Open(s, c, Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	boxes, err := p.Mailboxes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	for _, b := range boxes {
		roles[b.Role] = b.ID
	}
	if roles[RoleInbox] == "" || roles[RoleSent] == "" || roles[RoleArchive] == "" || roles[RoleTrash] == "" {
		t.Fatalf("roles: %+v", boxes)
	}
	if boxes[0].Role != RoleInbox || boxes[0].Total != 2 || boxes[0].Unread != 1 {
		t.Errorf("inbox: %+v", boxes[0])
	}

	page, err := p.Query(ctx, Query{MailboxID: roles[RoleInbox], Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Messages) != 2 {
		t.Fatalf("query: %+v", page)
	}
	newest, oldest := page.Messages[0], page.Messages[1]
	if newest.Subject != "Re: Hello" || !newest.Seen || !newest.HasAttachment || newest.ThreadID != oldest.ThreadID {
		t.Errorf("newest: %+v", newest)
	}
	if oldest.Preview != "Hi Ada, see https://example.com." || oldest.From[0].Name != "Bob" {
		t.Errorf("oldest: %+v", oldest)
	}

	m, err := p.Get(ctx, newest.ID)
	if err != nil {
		t.Fatal(err)
	}
	body := Clean(m)
	if strings.Contains(body.HTML, "script") || !body.HasRemote || len(m.Attachments) != 1 || m.Attachments[0].Name != "a.pdf" {
		t.Errorf("message: %+v %+v", body, m.Attachments)
	}
	_, r, err := p.Attachment(ctx, newest.ID, m.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(r); !strings.HasPrefix(string(b), "%PDF") {
		t.Errorf("attachment: %q", b)
	}

	yes := true
	if err := p.SetFlags(ctx, []string{oldest.ID}, &yes, &yes); err != nil {
		t.Fatal(err)
	}
	if page, _ := p.Query(ctx, Query{MailboxID: roles[RoleInbox], Unread: true}); page.Total != 0 {
		t.Errorf("unread after marking read: %d", page.Total)
	}
	if page, _ := p.Query(ctx, Query{Flagged: true}); page.Total != 1 {
		t.Errorf("starred: %d", page.Total)
	}

	moved, err := p.Move(ctx, []string{oldest.ID}, roles[RoleArchive])
	if err != nil {
		t.Fatal(err)
	}
	archived, _ := p.Query(ctx, Query{MailboxID: roles[RoleArchive]})
	if archived.Total != 1 || moved[oldest.ID] != archived.Messages[0].ID {
		t.Errorf("archive after move: %+v, new IDs %v", archived, moved)
	}
	// Moving back by the new ID (undo) works.
	if _, err := p.Move(ctx, []string{moved[oldest.ID]}, roles[RoleInbox]); err != nil {
		t.Fatal(err)
	}
	if page, _ := p.Query(ctx, Query{MailboxID: roles[RoleInbox]}); page.Total != 2 {
		t.Errorf("inbox after moving back: %d", page.Total)
	}
	again, _ := p.Query(ctx, Query{MailboxID: roles[RoleInbox], Search: "hello"})
	if _, err := p.Move(ctx, []string{again.Messages[len(again.Messages)-1].ID}, roles[RoleArchive]); err != nil {
		t.Fatal(err)
	}
	if page, _ := p.Query(ctx, Query{MailboxID: roles[RoleInbox], Search: "again"}); page.Total != 1 {
		t.Errorf("search: %d", page.Total)
	}

	err = p.Send(ctx, Outgoing{From: Address{Name: "Ada", Email: "ada@example.test"}, To: []Address{{Email: "ada@example.test"}},
		Subject: "Note to self", HTML: "<p>Hello <b>me</b></p>", InReplyTo: "<1@example.test>"})
	if err != nil {
		t.Fatal(err)
	}
	if page, _ := p.Query(ctx, Query{MailboxID: roles[RoleSent]}); page.Total != 1 || page.Messages[0].Subject != "Note to self" {
		t.Errorf("sent copy: %+v", page)
	}
	inbox, _ := p.Query(ctx, Query{MailboxID: roles[RoleInbox], Search: "self"})
	if inbox.Total != 1 || inbox.Messages[0].Preview != "Hello me" {
		t.Fatalf("delivered: %+v", inbox)
	}
	if err := p.Destroy(ctx, []string{inbox.Messages[0].ID}); err != nil {
		t.Fatal(err)
	}
	if page, _ := p.Query(ctx, Query{MailboxID: roles[RoleInbox]}); page.Total != 1 {
		t.Errorf("after destroy: %d", page.Total)
	}

	// IDLE reports new mail.
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	got := make(chan struct{}, 1)
	go p.(Watcher).Watch(wctx, func() { got <- struct{}{} })
	time.Sleep(300 * time.Millisecond)
	appendRaw(t, u, "INBOX", "From: carol@example.test\nSubject: New\n\nhi\n")
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Error("IDLE didn't report the new message")
	}
}

func TestPrivateAddressesBlocked(t *testing.T) {
	s, c, _ := testServer(t)
	p, _ := Open(s, c, Options{})
	if _, err := p.Mailboxes(context.Background()); err == nil {
		t.Fatal("connected to a loopback address without AllowPrivate")
	}
}
