package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestRealServers checks IMAP, SMTP and POP3 over TLS against a real mail
// server (scripts/stalwart-test.sh sets one up). It trusts that server's
// own certificate, still checking it. Set PETREL_REAL_IMAP, PETREL_REAL_SMTP
// and PETREL_REAL_POP3 (host:port, implicit TLS), and PETREL_JMAP_USER1,
// PETREL_JMAP_USER2 and PETREL_JMAP_PASSWORD.
func TestRealServers(t *testing.T) {
	imapAddr, smtpAddr, pop3Addr := os.Getenv("PETREL_REAL_IMAP"), os.Getenv("PETREL_REAL_SMTP"), os.Getenv("PETREL_REAL_POP3")
	if imapAddr == "" || smtpAddr == "" {
		t.Skip("PETREL_REAL_IMAP and PETREL_REAL_SMTP aren't set")
	}
	ctx := context.Background()
	user1, user2, pass := os.Getenv("PETREL_JMAP_USER1"), os.Getenv("PETREL_JMAP_USER2"), os.Getenv("PETREL_JMAP_PASSWORD")
	server := func(a string) Server {
		host, port, _ := net.SplitHostPort(a)
		n, _ := strconv.Atoi(port)
		return Server{Host: host, Port: n, Security: TLS}
	}

	// Trust the test server's certificate, under the name it's issued to.
	conn, err := tls.Dial("tcp", imapAddr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	cert := conn.ConnectionState().PeerCertificates[0]
	conn.Close()
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	name := cert.Subject.CommonName
	if len(cert.DNSNames) > 0 {
		name = cert.DNSNames[0]
	}
	opts := Options{AllowPrivate: true, TLS: &tls.Config{RootCAs: pool, ServerName: name}}
	t.Logf("trusting the server's certificate for %q", name)

	settings := Settings{Protocol: IMAP, Incoming: server(imapAddr), SMTP: server(smtpAddr)}
	open := func(user string) Provider {
		s := settings
		s.Username = user
		p, err := Open(s, Credentials{Username: user, Password: pass}, opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { p.Close() })
		return p
	}
	sam, lena := open(user1), open(user2)
	checkQuota(t, sam)
	bad, _ := Open(settings, Credentials{Username: user1, Password: "wrong"}, opts)
	if _, err := bad.Mailboxes(ctx); err != ErrAuth {
		t.Errorf("a wrong password: %v", err)
	}
	strict, _ := Open(settings, Credentials{Username: user1, Password: pass}, Options{AllowPrivate: true})
	if _, err := strict.Mailboxes(ctx); err == nil {
		t.Error("connected without trusting the self-signed certificate")
	}

	roles := map[string]string{}
	boxes, err := lena.Mailboxes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range boxes {
		roles[b.Role] = b.ID
	}
	for _, r := range []string{RoleInbox, RoleSent, RoleDrafts, RoleTrash, RoleJunk} {
		if roles[r] == "" {
			t.Errorf("no %s mailbox: %+v", r, boxes)
		}
	}

	wctx, stop := context.WithCancel(ctx)
	defer stop()
	pushed := make(chan struct{}, 8)
	go lena.(Watcher).Watch(wctx, func() { pushed <- struct{}{} })
	time.Sleep(700 * time.Millisecond)

	subject := "Petrel IMAP test " + time.Now().Format("150405.000")
	if err := sam.Send(ctx, Outgoing{From: Address{Name: "Sam", Email: user1}, To: []Address{{Email: user2}}, Subject: subject,
		HTML: "<p>Over <i>IMAP</i></p>", Attachments: []File{{Name: "a.txt", Type: "text/plain", Data: []byte("abc")}}}); err != nil {
		t.Fatalf("send over SMTP: %v", err)
	}
	select {
	case <-pushed:
	case <-time.After(10 * time.Second):
		t.Error("IDLE didn't report the new message")
	}
	var got Summary
	for i := 0; i < 40 && got.ID == ""; i++ {
		page, err := lena.Query(ctx, Query{MailboxID: roles[RoleInbox], Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page.Messages {
			if m.Subject == subject {
				got = m
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if got.ID == "" || !got.HasAttachment || got.Seen || got.Preview != "Over IMAP" {
		t.Fatalf("received: %+v", got)
	}
	checkFolders(t, lena, got.ID)
	m, err := lena.Get(ctx, got.ID)
	if err != nil || !strings.Contains(m.HTML, "<i>IMAP</i>") || len(m.Attachments) != 1 {
		t.Fatalf("get: %v %+v", err, m)
	}
	_, r, err := lena.Attachment(ctx, got.ID, m.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(r); string(b) != "abc" {
		t.Errorf("attachment: %q", b)
	}
	sent := ""
	samBoxes, _ := sam.Mailboxes(ctx)
	for _, b := range samBoxes {
		if b.Role == RoleSent {
			sent = b.ID
		}
	}
	if page, _ := sam.Query(ctx, Query{MailboxID: sent, Limit: 20}); !has(page, subject) {
		t.Errorf("no copy in sam's Sent")
	}

	yes := true
	if err := lena.SetFlags(ctx, []string{got.ID}, &yes, &yes); err != nil {
		t.Fatal(err)
	}
	if err := lena.SetKeywords(ctx, []string{got.ID}, []string{"work"}, nil); err != nil {
		t.Fatal(err)
	}
	if page, _ := lena.Query(ctx, Query{MailboxID: roles[RoleInbox], Keyword: "work"}); !has(page, subject) {
		t.Errorf("label: %+v", page)
	}
	moved, err := lena.Move(ctx, []string{got.ID}, roles[RoleTrash])
	if err != nil || moved[got.ID] == "" {
		t.Fatalf("move: %v %v", err, moved)
	}
	if _, err := lena.Move(ctx, []string{moved[got.ID]}, roles[RoleInbox]); err != nil {
		t.Fatalf("move back: %v", err)
	}
	// Leave the mailbox as it was.
	defer func() {
		if page, err := lena.Query(ctx, Query{MailboxID: roles[RoleInbox], Limit: 100}); err == nil {
			for _, m := range page.Messages {
				if m.Subject == subject {
					_ = lena.Destroy(ctx, []string{m.ID})
				}
			}
		}
	}()
	id, err := lena.SaveDraft(ctx, Outgoing{From: Address{Email: user2}, Subject: "Draft " + subject, Text: "draft"}, "")
	if err != nil || id == "" {
		t.Fatalf("draft: %q %v", id, err)
	}
	if err := lena.Destroy(ctx, []string{id}); err != nil {
		t.Fatal(err)
	}

	if pop3Addr != "" {
		s := Settings{Protocol: POP3, Username: user2, Incoming: server(pop3Addr), SMTP: server(smtpAddr)}
		p, _ := Open(s, Credentials{Username: user2, Password: pass}, opts)
		page, err := p.Query(ctx, Query{Limit: 50})
		if err != nil {
			t.Fatalf("POP3: %v", err)
		}
		if !has(page, subject) {
			t.Errorf("POP3 doesn't see the message: %+v", page)
		}
	}
}

func has(p Page, subject string) bool {
	for _, m := range p.Messages {
		if m.Subject == subject {
			return true
		}
	}
	return false
}
