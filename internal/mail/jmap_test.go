package mail

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestJMAP runs against a real JMAP server with two users. Set:
//
//	PETREL_JMAP_URL=http://127.0.0.1:8080/.well-known/jmap
//	PETREL_JMAP_USER1=sam@petrel.test  PETREL_JMAP_USER2=lena@petrel.test
//	PETREL_JMAP_PASSWORD=…  (the same for both)
//
// See docs/testing-jmap.md to set up Stalwart in Docker for it.
type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestJMAP(t *testing.T) {
	u := os.Getenv("PETREL_JMAP_URL")
	if u == "" {
		t.Skip("PETREL_JMAP_URL isn't set: no JMAP server to test against")
	}
	ctx := context.Background()
	user1, user2, pass := os.Getenv("PETREL_JMAP_USER1"), os.Getenv("PETREL_JMAP_USER2"), os.Getenv("PETREL_JMAP_PASSWORD")
	// The server announces https://<its hostname>/…; send those to u's host.
	base, _ := url.Parse(u)
	rewrite := roundTrip(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme, r.URL.Host = base.Scheme, base.Host
		return http.DefaultTransport.RoundTrip(r)
	})
	opts := Options{AllowPrivate: true, Transport: rewrite}
	open := func(user string) Provider {
		p, err := Open(Settings{Protocol: JMAP, Username: user, JMAPURL: u}, Credentials{Username: user, Password: pass}, opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { p.Close() })
		return p
	}
	sam, lena := open(user1), open(user2)

	bad, _ := Open(Settings{Protocol: JMAP, JMAPURL: u}, Credentials{Username: user1, Password: "wrong"}, opts)
	if _, err := bad.Mailboxes(ctx); err != ErrAuth {
		t.Errorf("a wrong password: %v", err)
	}

	boxes := func(p Provider) map[string]string {
		list, err := p.Mailboxes(ctx)
		if err != nil {
			t.Fatal(err)
		}
		roles := map[string]string{}
		for _, b := range list {
			if b.Role != "" {
				roles[b.Role] = b.ID
			}
		}
		return roles
	}
	lenaBoxes := boxes(lena)
	for _, r := range []string{RoleInbox, RoleSent, RoleDrafts, RoleTrash} {
		if lenaBoxes[r] == "" {
			t.Fatalf("lena has no %s mailbox: %v", r, lenaBoxes)
		}
	}
	samBoxes := boxes(sam)

	// Push: lena's watcher should hear about the message before it's queried.
	wctx, stop := context.WithCancel(ctx)
	defer stop()
	pushed := make(chan struct{}, 8)
	if w, ok := lena.(Watcher); ok {
		go w.Watch(wctx, func() { pushed <- struct{}{} })
	}
	time.Sleep(500 * time.Millisecond)
	stateBefore, err := lena.State(ctx)
	if err != nil {
		t.Fatal(err)
	}

	subject := "Petrel JMAP test " + time.Now().Format("150405.000")
	err = sam.Send(ctx, Outgoing{
		From: Address{Name: "Sam Taylor", Email: user1}, To: []Address{{Name: "Lena", Email: user2}},
		Subject: subject, HTML: "<p>Hello <b>Lena</b></p><script>bad()</script>", Text: "Hello Lena",
		Attachments: []File{{Name: "plan.txt", Type: "text/plain", Data: []byte("the plan")}},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	var got Summary
	for i := 0; i < 40; i++ { // delivery is quick but not instant
		page, err := lena.Query(ctx, Query{MailboxID: lenaBoxes[RoleInbox], Search: subject})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Messages) > 0 {
			got = page.Messages[0]
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if got.ID == "" {
		t.Fatal("the message didn't arrive in lena's inbox")
	}
	if got.Subject != subject || got.Seen || !got.HasAttachment || got.From[0].Email != user1 || got.Preview == "" {
		t.Errorf("summary: %+v", got)
	}
	select {
	case <-pushed:
	case <-time.After(5 * time.Second):
		t.Error("no push event for the new message")
	}
	if stateAfter, _ := lena.State(ctx); stateAfter == stateBefore {
		t.Error("the state didn't change")
	}

	m, err := lena.Get(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	body := Clean(m)
	if !strings.Contains(body.HTML, "<b>Lena</b>") || strings.Contains(body.HTML, "script") {
		t.Errorf("body: %q", body.HTML)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Name != "plan.txt" {
		t.Fatalf("attachments: %+v", m.Attachments)
	}
	_, r, err := lena.Attachment(ctx, got.ID, m.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(r); string(b) != "the plan" {
		t.Errorf("attachment: %q", b)
	}
	r.Close()

	// The copy in sam's Sent, and no draft left behind.
	if page, _ := sam.Query(ctx, Query{MailboxID: samBoxes[RoleSent], Search: subject}); page.Total != 1 {
		t.Errorf("sent: %+v", page)
	}
	if page, _ := sam.Query(ctx, Query{MailboxID: samBoxes[RoleDrafts], Search: subject}); page.Total != 0 {
		t.Errorf("drafts after sending: %+v", page)
	}

	// Flags, labels, search filters.
	yes := true
	if err := lena.SetFlags(ctx, []string{got.ID}, &yes, &yes); err != nil {
		t.Fatal(err)
	}
	if err := lena.SetKeywords(ctx, []string{got.ID}, []string{"work"}, nil); err != nil {
		t.Fatal(err)
	}
	page, _ := lena.Query(ctx, Query{MailboxID: lenaBoxes[RoleInbox], Keyword: "work"})
	if page.Total != 1 || !page.Messages[0].Seen || !page.Messages[0].Flagged || len(page.Messages[0].Keywords) != 1 {
		t.Errorf("flags and labels: %+v", page)
	}
	if page, _ := lena.Query(ctx, Query{Flagged: true}); page.Total < 1 {
		t.Errorf("starred: %+v", page)
	}
	if page, _ := lena.Query(ctx, Query{MailboxID: lenaBoxes[RoleInbox], HasAttachment: true, Search: subject}); page.Total != 1 {
		t.Errorf("has attachment: %+v", page)
	}

	// A mailbox of its own, moving there and back.
	snoozed, err := lena.EnsureMailbox(ctx, "Snoozed")
	if err != nil || snoozed == "" {
		t.Fatalf("ensure: %q %v", snoozed, err)
	}
	if again, _ := lena.EnsureMailbox(ctx, "Snoozed"); again != snoozed {
		t.Errorf("ensure twice: %q %q", snoozed, again)
	}
	if _, err := lena.Move(ctx, []string{got.ID}, snoozed); err != nil {
		t.Fatal(err)
	}
	if page, _ := lena.Query(ctx, Query{MailboxID: snoozed}); page.Total != 1 {
		t.Errorf("snoozed: %+v", page)
	}
	if _, err := lena.Move(ctx, []string{got.ID}, lenaBoxes[RoleInbox]); err != nil {
		t.Fatal(err)
	}

	// Drafts: save, replace, delete.
	d := Outgoing{From: Address{Email: user2}, To: []Address{{Email: user1}}, Subject: "Draft " + subject, HTML: "<p>v1</p>",
		Attachments: []File{{Name: "notes.txt", Type: "text/plain", Data: []byte("notes")}}}
	id1, err := lena.SaveDraft(ctx, d, "")
	if err != nil || id1 == "" {
		t.Fatalf("draft: %q %v", id1, err)
	}
	d.HTML = "<p>v2</p>"
	id2, err := lena.SaveDraft(ctx, d, id1)
	if err != nil {
		t.Fatal(err)
	}
	// (Search would lag: the server indexes text in the background.)
	drafts, _ := lena.Query(ctx, Query{MailboxID: lenaBoxes[RoleDrafts], Limit: 100})
	found := map[string]bool{}
	for _, m := range drafts.Messages {
		found[m.ID] = true
	}
	if !found[id2] || found[id1] {
		t.Errorf("drafts: want %s and not %s: %+v", id2, id1, drafts)
	}
	if dm, _ := lena.Get(ctx, id2); dm == nil || !strings.Contains(dm.HTML, "v2") || len(dm.Attachments) != 1 {
		t.Errorf("draft content: %+v", dm)
	}
	if err := lena.Destroy(ctx, []string{id2, got.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := lena.Get(ctx, got.ID); err != ErrNotFound {
		t.Errorf("after destroy: %v", err)
	}
}
