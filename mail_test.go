package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
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

// TestIdentities: sending as another address of the mailbox, with its own
// name, Reply-To and signature.
func TestIdentities(t *testing.T) {
	t.Setenv("PETREL_ALLOW_PRIVATE_HOSTS", "true")
	ip, sp, _ := mailServer(t)
	c := newClient(t)
	c.PostForm("/signup", url.Values{"name": {"Ada"}, "email": {"ada@petrel.test"}, "password": {"a long enough password"}})
	c.PostForm("/settings/mailboxes?/connect", url.Values{
		"email": {"ada@example.test"}, "name": {"Ada Lovelace"}, "password": {"secret"}, "protocol": {"imap"},
		"host": {"127.0.0.1"}, "port": {strconv.Itoa(ip)}, "security": {"plain"},
		"smtpHost": {"127.0.0.1"}, "smtpPort": {strconv.Itoa(sp)}, "smtpSecurity": {"plain"},
	})
	var accounts []app.AccountView
	api(t, c, "GET", "/api/mail/accounts", nil, &accounts)
	if len(accounts) != 1 || len(accounts[0].Identities) != 1 || !accounts[0].Identities[0].Default || accounts[0].Identities[0].Name != "Ada Lovelace" {
		t.Fatalf("default identity: %+v", accounts)
	}
	acc := accounts[0].ID
	page := fmt.Sprintf("/settings/mailboxes/%d", acc)
	if p := c.Get(page); p.Status != 200 || !strings.Contains(p.Body, "ada@example.test") {
		t.Fatalf("mailbox page: %d", p.Status)
	}
	// The default keeps the mailbox's address whatever is posted.
	c.PostForm(page+"?/identity", url.Values{"id": {strconv.FormatInt(accounts[0].Identities[0].ID, 10)}, "name": {"Ada L."}, "email": {"other@example.test"}})
	if p := c.PostForm(page+"?/identity", url.Values{"name": {"Ada at work"}, "email": {"Ada.Work@Example.test"},
		"replyTo": {"team@example.test"}, "signature": {`<p><b>Ada</b> – Analytical Engines</p><script>x()</script>`}}); p.Status != 200 {
		t.Fatalf("add identity: %d %s", p.Status, p.Body)
	}
	if p := c.PostForm(page+"?/identity", url.Values{"email": {"not an address"}}); p.Status != 400 {
		t.Errorf("a bad address: %d", p.Status)
	}
	api(t, c, "GET", "/api/mail/accounts", nil, &accounts)
	ids := accounts[0].Identities
	if len(ids) != 2 || ids[0].Email != "ada@example.test" || ids[0].Name != "Ada L." || ids[1].Email != "ada.work@example.test" {
		t.Fatalf("identities: %+v", ids)
	}
	if strings.Contains(ids[1].Signature, "script") || !strings.Contains(ids[1].Signature, "<b>Ada</b>") {
		t.Errorf("signature not cleaned: %q", ids[1].Signature)
	}

	send := map[string]any{"accountId": acc, "identityId": ids[1].ID, "to": "ada@example.test", "subject": "From work", "html": "<p>Hi</p>" + ids[1].Signature, "text": "Hi"}
	if s := api(t, c, "POST", "/api/mail/send", send, nil); s != 200 {
		t.Fatalf("send: %d", s)
	}
	var in list
	api(t, c, "GET", "/api/mail/messages?mailbox=inbox&q=From+work", nil, &in)
	if len(in.Messages) != 1 {
		t.Fatalf("inbox: %+v", in)
	}
	var msg app.MessageView
	api(t, c, "GET", "/api/mail/message?id="+url.QueryEscape(in.Messages[0].ID), nil, &msg)
	if msg.From[0].Email != "ada.work@example.test" || msg.From[0].Name != "Ada at work" || len(msg.ReplyTo) != 1 || msg.ReplyTo[0].Email != "team@example.test" {
		t.Errorf("headers: from %+v, reply-to %+v", msg.From, msg.ReplyTo)
	}
	// Removing: not the default, but an alias.
	if p := c.PostForm(page+"?/remove", url.Values{"id": {strconv.FormatInt(ids[0].ID, 10)}}); p.Status != 400 {
		t.Errorf("removed the default: %d", p.Status)
	}
	if p := c.PostForm(page+"?/remove", url.Values{"id": {strconv.FormatInt(ids[1].ID, 10)}}); p.Status != 200 {
		t.Errorf("remove: %d", p.Status)
	}
	// The test server has no Sieve: the page says so, and filters are refused.
	if p := c.Get(page); !strings.Contains(p.Body, "doesn’t run filters") {
		t.Error("filters offered without Sieve")
	}
	rules := `[{"name":"News","enabled":true,"if":[{"field":"list","op":"contains","value":"news"}],"then":[{"type":"read"}]}]`
	if p := c.PostForm(page+"?/filters", url.Values{"rules": {rules}}); p.Status != 400 || !strings.Contains(p.Body, "no Sieve") {
		t.Errorf("filters without Sieve: %d %s", p.Status, p.Body)
	}
	if p := c.PostForm(page+"?/filters", url.Values{"rules": {`[{"name":"Bad","enabled":true,"if":[],"then":[]}]`}}); p.Status != 400 || !strings.Contains(p.Body, "Bad") {
		t.Errorf("an empty rule: %d", p.Status)
	}
}

// TestFolderTools: folders, emptying the trash, marking all as read, the
// original message and the print page.
func TestFolderTools(t *testing.T) {
	t.Setenv("PETREL_ALLOW_PRIVATE_HOSTS", "true")
	ip, sp, _ := mailServer(t)
	c := newClient(t)
	c.PostForm("/signup", url.Values{"name": {"Ada"}, "email": {"ada@petrel.test"}, "password": {"a long enough password"}})
	c.PostForm("/settings/mailboxes?/connect", url.Values{
		"email": {"ada@example.test"}, "password": {"secret"}, "protocol": {"imap"},
		"host": {"127.0.0.1"}, "port": {strconv.Itoa(ip)}, "security": {"plain"},
		"smtpHost": {"127.0.0.1"}, "smtpPort": {strconv.Itoa(sp)}, "smtpSecurity": {"plain"},
	})
	var accounts []app.AccountView
	api(t, c, "GET", "/api/mail/accounts", nil, &accounts)
	acc := accounts[0].ID
	box := func(role string) string {
		api(t, c, "GET", "/api/mail/accounts", nil, &accounts)
		for _, b := range accounts[0].Mailboxes {
			if b.Role == role {
				return b.ID
			}
		}
		return ""
	}
	for i := range 3 {
		api(t, c, "POST", "/api/mail/send", map[string]any{"accountId": acc, "to": "ada@example.test", "subject": fmt.Sprintf("Tool %d", i), "html": "<p>x</p>", "text": "x"}, nil)
	}

	// Folders: make one, rename it, refuse the standard ones, delete it.
	var out struct {
		Folder  string
		Changed int
	}
	if s := api(t, c, "POST", "/api/mail/folders", map[string]any{"op": "create", "account": acc, "name": "Projects"}, &out); s != 200 || out.Folder == "" {
		t.Fatalf("create: %d %+v", s, out)
	}
	if s := api(t, c, "POST", "/api/mail/folders", map[string]any{"op": "create", "account": acc, "name": "Projects"}, nil); s != 400 {
		t.Errorf("a second Projects: %d", s)
	}
	if s := api(t, c, "POST", "/api/mail/folders", map[string]any{"op": "rename", "folder": out.Folder, "name": "Clients"}, &out); s != 200 {
		t.Fatalf("rename: %d", s)
	}
	names := func() string {
		api(t, c, "GET", "/api/mail/accounts", nil, &accounts)
		var n []string
		for _, b := range accounts[0].Mailboxes {
			n = append(n, b.Name)
		}
		return strings.Join(n, ",")
	}
	if n := names(); !strings.Contains(n, "Clients") || strings.Contains(n, "Projects") {
		t.Errorf("after renaming: %s", n)
	}
	if s := api(t, c, "POST", "/api/mail/folders", map[string]any{"op": "delete", "folder": box("inbox")}, nil); s != 400 {
		t.Errorf("deleted the inbox: %d", s)
	}
	if s := api(t, c, "POST", "/api/mail/folders", map[string]any{"op": "delete", "folder": out.Folder}, nil); s != 200 || strings.Contains(names(), "Clients") {
		t.Errorf("delete: %d %s", s, names())
	}

	// Mark all as read.
	if s := api(t, c, "POST", "/api/mail/folders", map[string]any{"op": "readAll", "folder": box("inbox")}, &out); s != 200 || out.Changed < 3 {
		t.Errorf("read all: %d %+v", s, out)
	}
	var unread list
	api(t, c, "GET", "/api/mail/messages?mailbox=inbox&unread=true", nil, &unread)
	if unread.Total != 0 {
		t.Errorf("unread after reading all: %d", unread.Total)
	}

	// The original, and the print page.
	var in list
	api(t, c, "GET", "/api/mail/messages?mailbox=inbox", nil, &in)
	id := in.Messages[0].ID
	if p := c.Get("/api/mail/raw?id=" + url.QueryEscape(id)); p.Status != 200 || !strings.Contains(p.Body, "Subject: Tool") || !strings.HasPrefix(p.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("raw: %d %s", p.Status, p.Header.Get("Content-Type"))
	}
	if p := c.Get("/api/mail/raw?download=1&id=" + url.QueryEscape(id)); !strings.Contains(p.Header.Get("Content-Disposition"), "message.eml") {
		t.Errorf("download: %v", p.Header)
	}
	if p := c.Get("/mail/print?id=" + url.QueryEscape(id)); p.Status != 200 || !strings.Contains(p.Body, "Tool") {
		t.Errorf("print: %d", p.Status)
	}

	// Empty the trash, and only the trash.
	ids := []string{}
	for _, m := range in.Messages {
		ids = append(ids, m.ID)
	}
	api(t, c, "POST", "/api/mail/actions", map[string]any{"ids": ids, "action": "trash"}, nil)
	if s := api(t, c, "POST", "/api/mail/folders", map[string]any{"op": "empty", "folder": box("inbox")}, nil); s != 400 {
		t.Errorf("emptied the inbox: %d", s)
	}
	if s := api(t, c, "POST", "/api/mail/folders", map[string]any{"op": "empty", "folder": box("trash")}, &out); s != 200 || out.Changed != len(ids) {
		t.Errorf("empty: %d %+v", s, out)
	}
	var trash list
	api(t, c, "GET", "/api/mail/messages?mailbox=trash", nil, &trash)
	if trash.Total != 0 {
		t.Errorf("trash after emptying: %d", trash.Total)
	}
}

// TestContacts: the address book, with vCards in and out.
func TestContacts(t *testing.T) {
	c := newClient(t)
	c.PostForm("/signup", url.Values{"name": {"Ada"}, "email": {"ada@petrel.test"}, "password": {"a long enough password"}})
	if p := c.PostForm("/contacts?/save", url.Values{"name": {"Charles Babbage"}, "email": {"Charles@Engine.example"}, "company": {"Difference, Inc."}}); p.Status != 200 {
		t.Fatalf("save: %d %s", p.Status, p.Body)
	}
	// Changing the address replaces the contact.
	c.PostForm("/contacts?/save", url.Values{"was": {"charles@engine.example"}, "name": {"Charles Babbage"}, "email": {"cb@engine.example"}, "phone": {"+44 20 1234"}})
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "people.vcf")
	io.WriteString(fw, "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Mary Somerville\r\nitem1.EMAIL;TYPE=INTERNET:mary@science.example\r\nNOTE:Line one\\nline two\r\nEND:VCARD\r\nBEGIN:VCARD\r\nVERSION:3.0\r\nFN:No Address\r\nEND:VCARD\r\n")
	mw.Close()
	req, _ := http.NewRequest("POST", c.URL("/contacts?/import"), &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", c.URL(""))
	if p := c.Do(req); p.Status != 200 {
		t.Fatalf("import: %d %s", p.Status, p.Body)
	}
	page := c.Get("/contacts")
	for _, want := range []string{"Charles Babbage", "cb@engine.example", "Mary Somerville"} {
		if !strings.Contains(page.Body, want) {
			t.Errorf("contacts page lacks %q", want)
		}
	}
	if strings.Contains(page.Body, "charles@engine.example") {
		t.Error("the old address stayed")
	}
	vcf := c.Get("/api/contacts/export")
	if !strings.Contains(vcf.Body, "FN:Mary Somerville") || !strings.Contains(vcf.Body, `NOTE:Line one\nline two`) || !strings.Contains(vcf.Body, "TEL:+44 20 1234") {
		t.Errorf("export: %s", vcf.Body)
	}
	c.PostForm("/contacts?/remove", url.Values{"email": {"mary@science.example"}})
	if strings.Contains(c.Get("/contacts").Body, "Mary Somerville") {
		t.Error("removed contact still there")
	}
	var people []map[string]string
	api(t, c, "GET", "/api/mail/contacts?q=babb", nil, &people)
	if len(people) != 1 || people[0]["email"] != "cb@engine.example" {
		t.Errorf("suggestions: %+v", people)
	}
}

// TestPGP: making a key, sending encrypted and signed mail, and reading it.
func TestPGP(t *testing.T) {
	t.Setenv("PETREL_ALLOW_PRIVATE_HOSTS", "true")
	ip, sp, _ := mailServer(t)
	c := newClient(t)
	c.PostForm("/signup", url.Values{"name": {"Ada"}, "email": {"ada@petrel.test"}, "password": {"a long enough password"}})
	c.PostForm("/settings/mailboxes?/connect", url.Values{
		"email": {"ada@example.test"}, "name": {"Ada"}, "password": {"secret"}, "protocol": {"imap"},
		"host": {"127.0.0.1"}, "port": {strconv.Itoa(ip)}, "security": {"plain"},
		"smtpHost": {"127.0.0.1"}, "smtpPort": {strconv.Itoa(sp)}, "smtpSecurity": {"plain"},
	})
	var accounts []app.AccountView
	api(t, c, "GET", "/api/mail/accounts", nil, &accounts)
	acc := accounts[0].ID

	if p := c.PostForm("/settings/encryption?/generate", url.Values{"email": {"someone@else.test"}, "passphrase": {"correct horse"}, "again": {"correct horse"}}); p.Status != 400 {
		t.Errorf("a key for someone else's address: %d", p.Status)
	}
	if p := c.PostForm("/settings/encryption?/generate", url.Values{"email": {"ada@example.test"}, "name": {"Ada"}, "passphrase": {"correct horse"}, "again": {"correct horse"}}); p.Status != 200 {
		t.Fatalf("generate: %d %s", p.Status, p.Body)
	}
	var status struct {
		Keys     []string
		Unlocked bool
	}
	api(t, c, "GET", "/api/pgp/status", nil, &status)
	if len(status.Keys) != 1 || status.Unlocked {
		t.Fatalf("status: %+v", status)
	}
	if s := api(t, c, "POST", "/api/pgp/unlock", map[string]string{"passphrase": "wrong"}, nil); s != 400 {
		t.Errorf("a wrong passphrase: %d", s)
	}

	// No key for a stranger: refused rather than sent in the clear.
	if s := api(t, c, "POST", "/api/mail/send", map[string]any{"accountId": acc, "to": "stranger@nowhere.test", "subject": "x", "html": "x", "encrypt": true}, nil); s != 400 {
		t.Errorf("encrypting to a stranger: %d", s)
	}
	// Signing needs the key unlocked.
	if s := api(t, c, "POST", "/api/mail/send", map[string]any{"accountId": acc, "to": "ada@example.test", "subject": "x", "html": "x", "sign": true}, nil); s != 403 {
		t.Errorf("signing while locked: %d", s)
	}
	if s := api(t, c, "POST", "/api/pgp/unlock", map[string]string{"passphrase": "correct horse"}, nil); s != 200 {
		t.Fatal("unlock")
	}
	send := map[string]any{"accountId": acc, "to": "ada@example.test", "subject": "Secret plan", "html": "<p>The <b>plan</b> is on.</p>", "text": "The plan is on.", "encrypt": true, "sign": true}
	if s := api(t, c, "POST", "/api/mail/send", send, nil); s != 200 {
		t.Fatalf("send encrypted: %d", s)
	}
	api(t, c, "POST", "/api/mail/send", map[string]any{"accountId": acc, "to": "ada@example.test", "subject": "Signed note", "html": "<p>Signed.</p>", "text": "Signed.", "sign": true}, nil)

	var in list
	api(t, c, "GET", "/api/mail/messages?mailbox=inbox", nil, &in)
	ids := map[string]string{}
	for _, m := range in.Messages {
		ids[m.Subject] = m.ID
	}
	raw := c.Get("/api/mail/raw?id=" + url.QueryEscape(ids["Secret plan"]))
	if !strings.Contains(raw.Body, "multipart/encrypted") || strings.Contains(raw.Body, "plan</b> is on") || !strings.Contains(raw.Body, "Autocrypt: addr=ada@example.test") {
		t.Errorf("not encrypted on the wire: %.600s", raw.Body)
	}
	type view struct {
		Body struct{ HTML string }
		PGP  *app.PGPView
	}
	var v view
	api(t, c, "GET", "/api/mail/message?id="+url.QueryEscape(ids["Secret plan"]), nil, &v)
	if v.PGP == nil || !v.PGP.Decrypted || !strings.Contains(v.Body.HTML, "<b>plan</b>") || v.PGP.Signature == nil || v.PGP.Signature.Status != "good" {
		t.Errorf("decrypted: %+v %q", v.PGP, v.Body.HTML)
	}
	api(t, c, "POST", "/api/pgp/lock", map[string]any{}, nil)
	v = view{}
	api(t, c, "GET", "/api/mail/message?id="+url.QueryEscape(ids["Secret plan"]), nil, &v)
	if v.PGP == nil || !v.PGP.Locked || strings.Contains(v.Body.HTML, "plan") {
		t.Errorf("locked: %+v %q", v.PGP, v.Body.HTML)
	}
	v = view{}
	api(t, c, "GET", "/api/mail/message?id="+url.QueryEscape(ids["Signed note"]), nil, &v)
	if v.PGP == nil || v.PGP.Signature == nil || v.PGP.Signature.Status != "good" || !strings.Contains(v.Body.HTML, "Signed.") {
		t.Errorf("signed: %+v", v.PGP)
	}
}

// TestInvitation: an invitation shows in the message, and answering it
// mails the organizer an iTIP reply.
func TestInvitation(t *testing.T) {
	t.Setenv("PETREL_ALLOW_PRIVATE_HOSTS", "true")
	ip, sp, u := mailServer(t)
	c := newClient(t)
	c.PostForm("/signup", url.Values{"name": {"Ada"}, "email": {"ada@petrel.test"}, "password": {"a long enough password"}})
	c.PostForm("/settings/mailboxes?/connect", url.Values{
		"email": {"ada@example.test"}, "name": {"Ada"}, "password": {"secret"}, "protocol": {"imap"},
		"host": {"127.0.0.1"}, "port": {strconv.Itoa(ip)}, "security": {"plain"},
		"smtpHost": {"127.0.0.1"}, "smtpPort": {strconv.Itoa(sp)}, "smtpSecurity": {"plain"},
	})
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:review-1@example.com\r\nDTSTAMP:20261117T100000Z\r\nDTSTART:20261120T140000Z\r\nDTEND:20261120T150000Z\r\nSUMMARY:Design review\r\nORGANIZER;CN=Lena:mailto:lena@example.com\r\nATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:ada@example.test\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	raw := "From: Lena <lena@example.com>\r\nTo: ada@example.test\r\nSubject: Invitation: Design review\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain\r\n\r\nYou're invited.\r\n--b\r\nContent-Type: text/calendar; method=REQUEST; charset=utf-8\r\nContent-Disposition: attachment; filename=invite.ics\r\n\r\n" + ics + "--b--\r\n"
	if _, err := u.Append("INBOX", lit{bytes.NewReader([]byte(raw)), int64(len(raw))}, &imap.AppendOptions{Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var in list
	api(t, c, "GET", "/api/mail/messages?mailbox=inbox", nil, &in)
	id := in.Messages[0].ID
	var v struct {
		Invite *app.InviteView
	}
	api(t, c, "GET", "/api/mail/message?id="+url.QueryEscape(id), nil, &v)
	if v.Invite == nil || v.Invite.Method != "REQUEST" || v.Invite.Event.Title != "Design review" || v.Invite.Status != "NEEDS-ACTION" {
		t.Fatalf("invite: %+v", v.Invite)
	}
	if s := api(t, c, "POST", "/api/cal/respond", map[string]string{"id": id, "part": v.Invite.Part, "status": "accepted"}, nil); s != 200 {
		t.Fatalf("respond: %d", s)
	}
	// The test server delivers everything to Ada: the reply is there.
	api(t, c, "GET", "/api/mail/messages?mailbox=inbox", nil, &in)
	var reply string
	for _, m := range in.Messages {
		if strings.HasPrefix(m.Subject, "Accepted: Design review") {
			reply = m.ID
		}
	}
	if reply == "" {
		t.Fatalf("no reply: %+v", in.Messages)
	}
	r := c.Get("/api/mail/raw?id=" + url.QueryEscape(reply))
	for _, want := range []string{`To: "Lena" <lena@example.com>`, "text/calendar", "method=REPLY", "METHOD:REPLY", "PARTSTAT=ACCEPTED", "review-1@example.com"} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("reply lacks %q", want)
		}
	}
}
