package mail

import (
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"petrel/internal/devpop3"
)

type memFlags struct {
	mu sync.Mutex
	m  map[string][2]bool
}

func (f *memFlags) Flags(_ context.Context, _ []string) (map[string][2]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string][2]bool{}
	for k, v := range f.m {
		out[k] = v
	}
	return out, nil
}

func (f *memFlags) SetFlags(_ context.Context, uidls []string, seen, flagged *bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range uidls {
		v := f.m[u]
		if seen != nil {
			v[0] = *seen
		}
		if flagged != nil {
			v[1] = *flagged
		}
		f.m[u] = v
	}
	return nil
}

func TestPOP3(t *testing.T) {
	ctx := context.Background()
	s, c, u := testServer(t)
	appendRaw(t, u, "INBOX", "From: Bob <bob@example.test>\nSubject: First\nMessage-ID: <a@x>\nDate: Mon, 16 Nov 2026 10:00:00 +0000\n\nline one\n.starts with a dot\n")
	appendRaw(t, u, "INBOX", "From: Carol <carol@example.test>\nSubject: Second\nDate: Mon, 16 Nov 2026 11:00:00 +0000\nContent-Type: multipart/mixed; boundary=b\n\n--b\nContent-Type: text/plain\n\nhello\n--b\nContent-Type: text/plain\nContent-Disposition: attachment; filename=notes.txt\n\nnotes\n--b--\n")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go devpop3.Serve(ln, addr(s.Incoming))
	s.Protocol = POP3
	s.Incoming = Server{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Security: Plain}

	flags := &memFlags{m: map[string][2]bool{}}
	p, err := Open(s, c, Options{AllowPrivate: true, Flags: flags})
	if err != nil {
		t.Fatal(err)
	}
	boxes, err := p.Mailboxes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(boxes) != 1 || boxes[0].Role != RoleInbox || boxes[0].Total != 2 || boxes[0].Unread != 2 {
		t.Fatalf("boxes: %+v", boxes)
	}
	page, err := p.Query(ctx, Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Messages[0].Subject != "Second" || !page.Messages[0].HasAttachment || page.Messages[1].Preview != "line one .starts with a dot" {
		t.Fatalf("query: %+v", page)
	}
	m, err := p.Get(ctx, page.Messages[0].ID)
	if err != nil || len(m.Attachments) != 1 {
		t.Fatalf("get: %v %+v", err, m)
	}
	_, r, err := p.Attachment(ctx, m.ID, m.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(r); strings.TrimSpace(string(b)) != "notes" {
		t.Errorf("attachment: %q", b)
	}
	yes := true
	if err := p.SetFlags(ctx, []string{m.ID}, &yes, &yes); err != nil {
		t.Fatal(err)
	}
	if page, _ := p.Query(ctx, Query{Unread: true}); page.Total != 1 || page.Messages[0].Subject != "First" {
		t.Errorf("unread: %+v", page)
	}
	if page, _ := p.Query(ctx, Query{Search: "carol"}); page.Total != 1 {
		t.Errorf("search: %+v", page)
	}
	if err := p.Destroy(ctx, []string{page.Messages[1].ID}); err != nil {
		t.Fatal(err)
	}
	if page, _ := p.Query(ctx, Query{}); page.Total != 1 || page.Messages[0].Subject != "Second" {
		t.Errorf("after delete: %+v", page)
	}
	if _, err := p.Move(ctx, []string{m.ID}, "x"); err != ErrUnsupported {
		t.Errorf("move: %v", err)
	}
}

func TestIMAPDraftsAndLabels(t *testing.T) {
	ctx := context.Background()
	s, c, u := testServer(t)
	if err := u.Create("Drafts", nil); err != nil {
		t.Fatal(err)
	}
	p, _ := Open(s, c, Options{AllowPrivate: true})
	defer p.Close()
	m := Outgoing{From: Address{Email: "ada@example.test"}, To: []Address{{Email: "bob@example.test"}}, Subject: "Plan", HTML: "<p>v1</p>",
		Attachments: []File{{Name: "a.txt", Type: "text/plain", Data: []byte("attached")}}}
	id, err := p.SaveDraft(ctx, m, "")
	if err != nil || id == "" {
		t.Fatalf("save: %q %v", id, err)
	}
	m.HTML = "<p>v2</p>"
	id2, err := p.SaveDraft(ctx, m, id)
	if err != nil {
		t.Fatal(err)
	}
	boxes, _ := p.Mailboxes(ctx)
	var drafts string
	for _, b := range boxes {
		if b.Role == RoleDrafts {
			drafts = b.ID
		}
	}
	page, _ := p.Query(ctx, Query{MailboxID: drafts})
	if page.Total != 1 || page.Messages[0].ID != id2 || !page.Messages[0].HasAttachment {
		t.Fatalf("drafts: %+v", page)
	}
	got, _ := p.Get(ctx, id2)
	if !strings.Contains(got.HTML, "v2") || len(got.Attachments) != 1 || got.Attachments[0].Name != "a.txt" {
		t.Errorf("draft: %q %+v", got.HTML, got.Attachments)
	}

	if err := p.SetKeywords(ctx, []string{id2}, []string{"work"}, nil); err != nil {
		t.Fatal(err)
	}
	if page, _ := p.Query(ctx, Query{MailboxID: drafts, Keyword: "work"}); page.Total != 1 || len(page.Messages[0].Keywords) != 1 || page.Messages[0].Keywords[0] != "work" {
		t.Errorf("labelled: %+v", page)
	}
	if err := p.SetKeywords(ctx, []string{id2}, nil, []string{"work"}); err != nil {
		t.Fatal(err)
	}
	if page, _ := p.Query(ctx, Query{MailboxID: drafts, Keyword: "work"}); page.Total != 0 {
		t.Errorf("unlabelled: %+v", page)
	}

	snoozed, err := p.EnsureMailbox(ctx, "Snoozed")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := p.EnsureMailbox(ctx, "Snoozed")
	if snoozed == "" || snoozed != again {
		t.Errorf("ensure: %q %q", snoozed, again)
	}
}
