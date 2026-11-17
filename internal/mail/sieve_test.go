package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gomail "github.com/emersion/go-message/mail"
)

func TestScript(t *testing.T) {
	rules := []Rule{
		{Name: "Newsletters", Enabled: true, Any: true,
			If:   []Condition{{Field: "list", Op: "contains", Value: "news"}, {Field: "from", Op: "contains", Value: `"quoted" \ back`}},
			Then: []Action{{Type: "move", Value: "Reading/News"}, {Type: "read"}}},
		{Name: "Boss", Enabled: true, If: []Condition{{Field: "from", Op: "is", Value: "boss@example.com"}, {Field: "size", Op: "over", Value: "100"}},
			Then: []Action{{Type: "star"}, {Type: "label", Value: "boss"}, {Type: "forward", Value: "me@example.org"}, {Type: "stop"}}},
		{Name: "Off", Enabled: false, If: []Condition{{Field: "subject", Op: "contains", Value: "x"}}, Then: []Action{{Type: "delete"}}},
		{Name: "Invalid", Enabled: true, If: []Condition{{Field: "subject", Op: "over", Value: "x"}}, Then: []Action{{Type: "delete"}}},
	}
	got := Script(rules, Vacation{Enabled: true, From: "2026-12-20", Until: "2027-01-02", Subject: "Away", Text: "Back in January.", Days: 3},
		SieveOptions{Trash: "Trash", Me: []string{"me@example.com"}})
	for _, want := range []string{
		`require ["copy", "date", "fileinto", "imap4flags", "relational", "vacation"];`,
		`if allof (currentdate :value "ge" "date" "2026-12-20", currentdate :value "le" "date" "2027-01-02") {`,
		`vacation :days 3 :subject "Away" :addresses ["me@example.com"] "Back in January.";`,
		`if anyof (header :contains "list-id" "news", header :contains ["from", "sender"] "\"quoted\" \\ back") {`,
		"\taddflag [\"\\\\Seen\"];\n\tfileinto \"Reading/News\";\n\tremoveflag [\"\\\\Seen\"];",
		`if allof (header :is ["from", "sender"] "boss@example.com", size :over 100K) {`,
		"\taddflag [\"\\\\Flagged\", \"boss\"];\n\tredirect :copy \"me@example.org\";\n\tkeep;\n\tstop;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "# Off") || strings.Contains(got, "# Invalid") {
		t.Errorf("disabled or invalid rules written:\n%s", got)
	}
	if s := Script(nil, Vacation{}, SieveOptions{}); strings.Contains(s, "require") {
		t.Errorf("an empty script requires extensions: %s", s)
	}
}

// TestRealSieve runs filters and a vacation reply on a real server (see
// scripts/stalwart-test.sh): PETREL_REAL_SIEVE is its ManageSieve address,
// with PETREL_REAL_IMAP, PETREL_REAL_SMTP and PETREL_JMAP_* as for
// TestRealServers and TestJMAP.
func TestRealSieve(t *testing.T) {
	sieveAddr, imapAddr, smtpAddr, jmapURL := os.Getenv("PETREL_REAL_SIEVE"), os.Getenv("PETREL_REAL_IMAP"), os.Getenv("PETREL_REAL_SMTP"), os.Getenv("PETREL_JMAP_URL")
	if sieveAddr == "" || imapAddr == "" || jmapURL == "" {
		t.Skip("PETREL_REAL_SIEVE, PETREL_REAL_IMAP and PETREL_JMAP_URL aren't set")
	}
	ctx := context.Background()
	user1, user2, pass := os.Getenv("PETREL_JMAP_USER1"), os.Getenv("PETREL_JMAP_USER2"), os.Getenv("PETREL_JMAP_PASSWORD")
	server := func(a string, sec Security) Server {
		host, port, _ := net.SplitHostPort(a)
		n, _ := strconv.Atoi(port)
		return Server{Host: host, Port: n, Security: sec}
	}
	conn, err := tls.Dial("tcp", imapAddr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	cert := conn.ConnectionState().PeerCertificates[0]
	conn.Close()
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	opts := Options{AllowPrivate: true, TLS: &tls.Config{RootCAs: pool, ServerName: cert.DNSNames[0]}}
	sv := server(sieveAddr, StartTLS)
	sam, err := Open(Settings{Protocol: IMAP, Username: user1, Incoming: server(imapAddr, TLS), SMTP: server(smtpAddr, TLS), Sieve: &sv},
		Credentials{Username: user1, Password: pass}, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer sam.Close()
	base, _ := url.Parse(jmapURL)
	jopts := Options{AllowPrivate: true, Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme, r.URL.Host = base.Scheme, base.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	lena, err := Open(Settings{Protocol: JMAP, Username: user2, JMAPURL: jmapURL}, Credentials{Username: user2, Password: pass}, jopts)
	if err != nil {
		t.Fatal(err)
	}
	defer lena.Close()

	// IMAP over ManageSieve: a filter on sam's mail.
	sp := sam.(SieveProvider)
	st, err := sp.SieveStatus(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !contains(st.Extensions, "fileinto") || !contains(st.Extensions, "vacation") {
		t.Errorf("extensions: %v", st.Extensions)
	}
	if err := sp.PutSieve(ctx, "if true { nonsense; }"); err == nil {
		t.Error("an invalid script was accepted")
	} else if _, ok := err.(*ErrSieve); !ok {
		t.Errorf("an invalid script: %v", err)
	}
	box, err := sam.EnsureMailbox(ctx, "Filtered")
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Format("150405.000")
	script := Script([]Rule{{Name: "Test", Enabled: true, If: []Condition{{Field: "subject", Op: "contains", Value: "Sieve " + stamp}},
		Then: []Action{{Type: "move", Value: "Filtered"}, {Type: "label", Value: "sieved"}, {Type: "read"}}}}, Vacation{}, SieveOptions{})
	if err := sp.PutSieve(ctx, script); err != nil {
		t.Fatalf("put: %v", err)
	}
	if st, _ := sp.SieveStatus(ctx); st.Active != ScriptName {
		t.Errorf("active script: %q", st.Active)
	}
	if err := lena.Send(ctx, Outgoing{From: Address{Email: user2}, To: []Address{{Email: user1}}, Subject: "Sieve " + stamp, Text: "filter me"}); err != nil {
		t.Fatal(err)
	}
	var got Summary
	for i := 0; i < 40 && got.ID == ""; i++ {
		page, _ := sam.Query(ctx, Query{MailboxID: box, Limit: 50})
		for _, m := range page.Messages {
			if m.Subject == "Sieve "+stamp {
				got = m
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if got.ID == "" {
		t.Fatal("the filter didn't file the message")
	}
	if !got.Seen || !contains(got.Keywords, "sieved") {
		t.Errorf("the filter's flags: %+v", got)
	}
	_ = sam.Destroy(ctx, []string{got.ID})

	// JMAP: Sieve, and the server's own vacation reply.
	if err := lena.(SieveProvider).PutSieve(ctx, Script(nil, Vacation{}, SieveOptions{})); err != nil {
		t.Fatalf("JMAP put: %v", err)
	}
	if st, err := lena.(SieveProvider).SieveStatus(ctx); err != nil || st.Active != ScriptName {
		t.Errorf("JMAP status: %+v %v", st, err)
	}
	vp := lena.(VacationProvider)
	if err := vp.SetVacation(ctx, Vacation{Enabled: true, Subject: "Away " + stamp, Text: "Lena is away."}); err != nil {
		t.Fatalf("vacation: %v", err)
	}
	defer vp.SetVacation(ctx, Vacation{})
	if err := sam.Send(ctx, Outgoing{From: Address{Email: user1}, To: []Address{{Email: user2}}, Subject: "Are you there " + stamp, Text: "?"}); err != nil {
		t.Fatal(err)
	}
	boxes, _ := sam.Mailboxes(ctx)
	inbox := ""
	for _, b := range boxes {
		if b.Role == RoleInbox {
			inbox = b.ID
		}
	}
	var reply Summary
	for i := 0; i < 40 && reply.ID == ""; i++ {
		page, _ := sam.Query(ctx, Query{MailboxID: inbox, Limit: 50})
		for _, m := range page.Messages {
			if m.Subject == "Away "+stamp {
				reply = m
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if reply.ID == "" {
		t.Fatal("no vacation reply")
	}
	_ = sam.Destroy(ctx, []string{reply.ID})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func TestUnsubscribe(t *testing.T) {
	var h gomail.Header
	h.Set("List-Unsubscribe", "<mailto:leave@list.example?subject=unsubscribe>, <https://list.example/u/123>")
	h.Set("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	if u := unsubscribeOf(h); u == nil || u.URL != "https://list.example/u/123" || u.Mailto != "mailto:leave@list.example?subject=unsubscribe" || !u.OneClick {
		t.Errorf("both: %+v", u)
	}
	h.Set("List-Unsubscribe", "<http://list.example/u>")
	if u := unsubscribeOf(h); u == nil || u.OneClick {
		t.Errorf("one click over http: %+v", u)
	}
	h.Set("List-Unsubscribe", "<javascript:alert(1)>")
	if u := unsubscribeOf(h); u != nil {
		t.Errorf("javascript: %+v", u)
	}
}
