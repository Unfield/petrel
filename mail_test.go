package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/kit/kittest"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-smtp"

	"petrel/internal/app"
)

// mailServer runs IMAP and SMTP in memory for ada@example.test.
func mailServer(t *testing.T) (imapPort, smtpPort int, u *imapmemserver.User) {
	t.Helper()
	mem := imapmemserver.New()
	u = imapmemserver.NewUser("ada@example.test", "secret")
	for _, box := range []string{"INBOX", "Drafts", "Sent", "Archive", "Trash"} {
		if err := u.Create(box, nil); err != nil {
			t.Fatal(err)
		}
	}
	mem.AddUser(u)
	srv := imapserver.New(&imapserver.Options{
		NewSession:   func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) { return mem.NewSession(), nil, nil },
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		InsecureAuth: true,
	})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go srv.Serve(ln)
	sln, _ := net.Listen("tcp", "127.0.0.1:0")
	ss := smtp.NewServer(inbox{u})
	ss.Domain = "localhost"
	go ss.Serve(sln)
	t.Cleanup(func() { srv.Close(); ss.Close() })
	return ln.Addr().(*net.TCPAddr).Port, sln.Addr().(*net.TCPAddr).Port, u
}

type inbox struct{ u *imapmemserver.User }

func (i inbox) NewSession(*smtp.Conn) (smtp.Session, error) { return &delivery{u: i.u}, nil }

type delivery struct{ u *imapmemserver.User }

func (d *delivery) Mail(string, *smtp.MailOptions) error { return nil }
func (d *delivery) Rcpt(string, *smtp.RcptOptions) error { return nil }
func (d *delivery) Reset()                               {}
func (d *delivery) Logout() error                        { return nil }
func (d *delivery) Data(r io.Reader) error {
	b, _ := io.ReadAll(r)
	_, err := d.u.Append("INBOX", lit{bytes.NewReader(b), int64(len(b))}, &imap.AppendOptions{Time: time.Now()})
	return err
}

type lit struct {
	*bytes.Reader
	n int64
}

func (l lit) Size() int64 { return l.n }

func api(t *testing.T, c *kittest.Client, method, path string, body any, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.URL(path), r)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	page := c.Do(req)
	if out != nil && page.Status < 300 {
		if err := json.Unmarshal([]byte(page.Body), out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, path, err, page.Body)
		}
	}
	if page.Status >= 300 {
		t.Logf("%s %s: %d %s", method, path, page.Status, page.Body)
	}
	return page.Status
}

type list struct {
	Messages []struct {
		ID, Subject, MailboxRole string
		HasAttachment            bool
		Keywords                 []string
	}
	Total int
}

func TestMailEndToEnd(t *testing.T) {
	t.Setenv("PETREL_ALLOW_PRIVATE_HOSTS", "true")
	ip, sp, _ := mailServer(t)
	c := newClient(t)
	c.PostForm("/signup", url.Values{"name": {"Ada"}, "email": {"ada@petrel.test"}, "password": {"a long enough password"}})

	page := c.PostForm("/settings/mailboxes?/connect", url.Values{
		"email": {"ada@example.test"}, "name": {"Ada"}, "password": {"secret"}, "protocol": {"imap"},
		"host": {"127.0.0.1"}, "port": {strconv.Itoa(ip)}, "security": {"plain"},
		"smtpHost": {"127.0.0.1"}, "smtpPort": {strconv.Itoa(sp)}, "smtpSecurity": {"plain"},
	})
	if page.Status != 200 || !strings.Contains(page.Body, "connected") {
		t.Fatalf("connect: %d %s", page.Status, page.Body)
	}
	if page := c.PostForm("/settings/mailboxes?/connect", url.Values{
		"email": {"wrong@example.test"}, "password": {"nope"}, "protocol": {"imap"},
		"host": {"127.0.0.1"}, "port": {strconv.Itoa(ip)}, "security": {"plain"},
		"smtpHost": {"127.0.0.1"}, "smtpPort": {strconv.Itoa(sp)}, "smtpSecurity": {"plain"},
	}); page.Status != 400 || !strings.Contains(page.Body, "refused") {
		t.Errorf("a wrong password: %d %s", page.Status, page.Body)
	}

	var accounts []app.AccountView
	api(t, c, "GET", "/api/mail/accounts", nil, &accounts)
	if len(accounts) != 1 || len(accounts[0].Mailboxes) != 5 {
		t.Fatalf("accounts: %+v", accounts)
	}
	acc := accounts[0].ID

	// Upload a file and send it to ourselves.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "plan.txt")
	fw.Write([]byte("the plan"))
	mw.Close()
	req, _ := http.NewRequest("POST", c.URL("/api/mail/upload"), &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	up := c.Do(req)
	var upload app.Upload
	if err := json.Unmarshal([]byte(up.Body), &upload); err != nil || upload.ID == "" {
		t.Fatalf("upload: %d %s", up.Status, up.Body)
	}
	draft := map[string]any{"accountId": acc, "to": "Ada <ada@example.test>", "subject": "With a file", "html": "<p>See <b>attached</b></p>",
		"attachments": []map[string]string{{"upload": upload.ID}}}
	var saved struct{ ID string }
	if api(t, c, "POST", "/api/mail/draft", draft, &saved); saved.ID == "" {
		t.Fatal("no draft ID")
	}
	draft["draftId"] = saved.ID
	if s := api(t, c, "POST", "/api/mail/send", draft, nil); s != 200 {
		t.Fatalf("send: %d", s)
	}
	var in list
	api(t, c, "GET", "/api/mail/messages?mailbox=inbox", nil, &in)
	if in.Total != 1 || in.Messages[0].Subject != "With a file" || !in.Messages[0].HasAttachment {
		t.Fatalf("inbox: %+v", in)
	}
	var drafts list
	api(t, c, "GET", "/api/mail/messages?mailbox=drafts", nil, &drafts)
	if drafts.Total != 0 {
		t.Errorf("the draft stayed after sending: %+v", drafts)
	}
	var sent list
	api(t, c, "GET", "/api/mail/messages?mailbox=sent", nil, &sent)
	if sent.Total != 1 {
		t.Errorf("sent: %+v", sent)
	}
	var msg app.MessageView
	api(t, c, "GET", "/api/mail/message?id="+url.QueryEscape(in.Messages[0].ID), nil, &msg)
	if len(msg.Attachments) != 1 || msg.Attachments[0].Name != "plan.txt" {
		t.Fatalf("message: %+v", msg.Attachments)
	}
	if file := c.Get(msg.Attachments[0].URL); file.Body != "the plan" || !strings.Contains(file.Header.Get("Content-Disposition"), "plan.txt") {
		t.Errorf("download: %q %v", file.Body, file.Header)
	}

	// Forwarding attaches the original's file again.
	fwd := map[string]any{"accountId": acc, "to": "ada@example.test", "subject": "Fwd", "text": "fyi",
		"attachments": []map[string]string{{"message": msg.ID, "part": msg.Attachments[0].ID}}}
	if s := api(t, c, "POST", "/api/mail/send", fwd, nil); s != 200 {
		t.Fatalf("forward: %d", s)
	}

	// Contacts learned from sending and reading.
	var people []map[string]string
	api(t, c, "GET", "/api/mail/contacts?q=ad", nil, &people)
	if len(people) == 0 || people[0]["email"] != "ada@example.test" {
		t.Errorf("contacts: %+v", people)
	}

	// Labels.
	id := in.Messages[0].ID
	api(t, c, "POST", "/api/mail/actions", map[string]any{"ids": []string{id}, "action": "label", "label": "Project X"}, nil)
	var labelled list
	api(t, c, "GET", "/api/mail/messages?mailbox=label:project-x", nil, &labelled)
	if labelled.Total != 1 || labelled.Messages[0].Keywords[0] != "project-x" {
		t.Errorf("label: %+v", labelled)
	}

	// Snooze, then wake it as the scheduler would.
	var snoozed struct{ Moved map[string]string }
	api(t, c, "POST", "/api/mail/actions", map[string]any{"ids": []string{id}, "action": "snooze", "until": time.Now().Add(time.Hour).UnixMilli()}, &snoozed)
	var waiting list
	api(t, c, "GET", "/api/mail/messages?mailbox=snoozed", nil, &waiting)
	if waiting.Total != 1 || waiting.Messages[0].MailboxRole != "snoozed" {
		t.Fatalf("snoozed: %+v", waiting)
	}
	if _, err := app.Current.DB.Exec(`UPDATE snoozes SET until = 0`); err != nil {
		t.Fatal(err)
	}
	if err := app.Current.Mail.Wake(t.Context()); err != nil {
		t.Fatal(err)
	}
	var woke list
	api(t, c, "GET", "/api/mail/messages?mailbox=inbox&unread=true", nil, &woke)
	back := false
	for _, m := range woke.Messages {
		back = back || m.Subject == "With a file"
	}
	if !back {
		t.Errorf("not back in the inbox, unread, after waking: %+v", woke)
	}

	// Preferences follow the user.
	api(t, c, "POST", "/api/mail/prefs", map[string]any{"mode": "dark", "labels": []map[string]string{{"name": "Project X", "color": "#ff0000"}}}, nil)
	if page := c.Get("/mail"); !strings.Contains(page.Body, `data-mode="dark"`) {
		t.Error("the saved preferences don't apply")
	}
}

func TestDirectSignIn(t *testing.T) {
	ip, sp, _ := mailServer(t)
	t.Setenv("PETREL_ALLOW_PRIVATE_HOSTS", "true")
	t.Setenv("PETREL_AUTH_MODE", "direct")
	t.Setenv("PETREL_DIRECT_SERVERS", `[{"domain":"example.test","protocol":"imap",
		"incoming":{"host":"127.0.0.1","port":`+strconv.Itoa(ip)+`,"security":"plain"},
		"smtp":{"host":"127.0.0.1","port":`+strconv.Itoa(sp)+`,"security":"plain"}}]`)
	c := newClient(t)
	if page := c.Get("/signup"); page.Status/100 != 3 {
		t.Errorf("sign-up is open in direct mode: %d", page.Status)
	}
	if page := c.Get("/"); page.Status/100 != 3 || page.Location() != "/login" {
		t.Errorf("home in direct mode: %d %s", page.Status, page.Location())
	}
	if page := c.PostForm("/login?/mailbox", url.Values{"email": {"ada@example.test"}, "password": {"wrong"}}); page.Status != 400 {
		t.Errorf("a wrong password: %d", page.Status)
	}
	// In German, from the browser's language.
	req, _ := http.NewRequest("POST", c.URL("/login?/mailbox"), strings.NewReader(url.Values{"email": {"ada@example.test"}, "password": {"wrong"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.URL(""))
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	if page := c.Do(req); page.Status != 400 || !strings.Contains(page.Body, "Passwort stimmen nicht") {
		t.Errorf("a wrong password in German: %d %s", page.Status, page.Body)
	}
	if page := c.Get("/login"); !strings.Contains(page.Body, "Sign in to your mail") {
		t.Errorf("the login page in English: %s", page.Body)
	}
	if page := c.PostForm("/login?/mailbox", url.Values{"email": {"bob@elsewhere.test"}, "password": {"x"}}); page.Status != 400 {
		t.Errorf("another domain: %d", page.Status)
	}
	if page := c.PostForm("/login?/mailbox", url.Values{"email": {"ada@example.test"}, "password": {"secret"}}); page.Status != 303 {
		t.Fatalf("sign-in: %d %s", page.Status, page.Body)
	}
	if page := c.Get("/"); page.Status/100 != 3 || page.Location() != "/mail" {
		t.Errorf("home when signed in: %d %s", page.Status, page.Location())
	}
	var accounts []app.AccountView
	api(t, c, "GET", "/api/mail/accounts", nil, &accounts)
	if len(accounts) != 1 || accounts[0].Email != "ada@example.test" || accounts[0].Error != "" {
		t.Fatalf("accounts: %+v", accounts)
	}
	// No team was made at sign-in; settings make one rather than asking.
	for range 2 {
		if page := c.Get("/settings"); page.Status != 200 {
			t.Fatalf("settings without a team: %d %s", page.Status, page.Location())
		}
	}
	for path, want := range map[string]string{"/settings": "Konto löschen", "/settings/security": "Angemeldete Geräte", "/settings/mailboxes": "Postfächer"} {
		req, _ := http.NewRequest("GET", c.URL(path), nil)
		req.Header.Set("Accept-Language", "de")
		if page := c.Do(req); page.Status != 200 || !strings.Contains(page.Body, want) {
			t.Errorf("%s in German: %d, no %q", path, page.Status, want)
		}
	}
	// Teams are off unless PETREL_TEAMS=true.
	if page := c.Get("/settings/team"); page.Status != 404 {
		t.Errorf("team page with teams off: %d", page.Status)
	}
	if page := c.Get("/settings/security"); !strings.Contains(page.Body, "mailbox’s password") || strings.Contains(page.Body, "Passkeys") {
		t.Errorf("security page in direct mode")
	}
	c.PostForm("/logout", url.Values{})
	if n, _ := app.Current.Mail.List(t.Context(), 1); len(n) != 0 {
		t.Errorf("the mailbox password stayed after signing out: %+v", n)
	}
}

// TestSignInLimit: Petrel mustn't be a way to guess mailbox passwords.
func TestSignInLimit(t *testing.T) {
	ip, sp, _ := mailServer(t)
	t.Setenv("PETREL_ALLOW_PRIVATE_HOSTS", "true")
	t.Setenv("PETREL_AUTH_MODE", "direct")
	t.Setenv("PETREL_DIRECT_SERVERS", `[{"domain":"example.test","protocol":"imap",
		"incoming":{"host":"127.0.0.1","port":`+strconv.Itoa(ip)+`,"security":"plain"},
		"smtp":{"host":"127.0.0.1","port":`+strconv.Itoa(sp)+`,"security":"plain"}}]`)
	c := newClient(t)
	guess := url.Values{"email": {"limit@example.test"}, "password": {"guess"}}
	for i := range 5 {
		if page := c.PostForm("/login?/mailbox", guess); page.Status != 400 || !strings.Contains(page.Body, "isn’t right") {
			t.Fatalf("guess %d: %d %s", i, page.Status, page.Body)
		}
	}
	if page := c.PostForm("/login?/mailbox", guess); page.Status != 400 || !strings.Contains(page.Body, "Too many") || page.Header.Get("Retry-After") == "" {
		t.Errorf("the sixth guess: %d %v %s", page.Status, page.Header, page.Body)
	}
}

// TestDeleteAccount: deleting the account deletes its mailboxes and their
// passwords, which Tern doesn't know about.
func TestDeleteAccount(t *testing.T) {
	t.Setenv("PETREL_ALLOW_PRIVATE_HOSTS", "true")
	t.Setenv("PETREL_TEAMS", "true")
	ip, sp, _ := mailServer(t)
	c := newClient(t)
	c.PostForm("/signup", url.Values{"name": {"Ada"}, "email": {"ada@petrel.test"}, "password": {"a long enough password"}})
	if page := c.Get("/settings/team"); page.Status != 200 || !strings.Contains(page.Body, "Ada’s team") {
		t.Errorf("team page with teams on: %d", page.Status)
	}
	if page := c.PostForm("/settings/mailboxes?/connect", url.Values{
		"email": {"ada@example.test"}, "password": {"secret"}, "protocol": {"imap"},
		"host": {"127.0.0.1"}, "port": {strconv.Itoa(ip)}, "security": {"plain"},
		"smtpHost": {"127.0.0.1"}, "smtpPort": {strconv.Itoa(sp)}, "smtpSecurity": {"plain"},
	}); page.Status != 200 {
		t.Fatalf("connect: %d %s", page.Status, page.Body)
	}
	var n int
	app.Current.DB.QueryRow(`SELECT COUNT(*) FROM mail_accounts`).Scan(&n)
	if n != 1 {
		t.Fatalf("mailboxes before: %d", n)
	}
	if page := c.PostForm("/settings?/delete", url.Values{"form": {"delete"}}); page.Status/100 != 3 {
		t.Fatalf("delete: %d %s", page.Status, page.Body)
	}
	app.Current.DB.QueryRow(`SELECT COUNT(*) FROM mail_accounts`).Scan(&n)
	if n != 0 {
		t.Errorf("the mailbox and its password stayed after deleting the account")
	}
}
