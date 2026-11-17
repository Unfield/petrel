package mail_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"

	"petrel/internal/mail"
	"petrel/internal/pgp"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestJMAPSealed sends a PGP/MIME message over JMAP (Email/import, then
// EmailSubmission) and checks it arrives intact.
func TestJMAPSealed(t *testing.T) {
	u := os.Getenv("PETREL_JMAP_URL")
	if u == "" {
		t.Skip("PETREL_JMAP_URL isn't set")
	}
	ctx := context.Background()
	user1, user2, pass := os.Getenv("PETREL_JMAP_USER1"), os.Getenv("PETREL_JMAP_USER2"), os.Getenv("PETREL_JMAP_PASSWORD")
	base, _ := url.Parse(u)
	opts := mail.Options{AllowPrivate: true, Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme, r.URL.Host = base.Scheme, base.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	open := func(user string) mail.Provider {
		p, err := mail.Open(mail.Settings{Protocol: mail.JMAP, Username: user, JMAPURL: u}, mail.Credentials{Username: user, Password: pass}, opts)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	sam, lena := open(user1), open(user2)
	defer sam.Close()
	defer lena.Close()

	priv, pub, _, err := pgp.Generate("Sam", user1, "pass phrase")
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := pgp.Unlock(priv, "pass phrase")
	samKey, _ := pgp.Read(pub)
	subject := "Sealed " + time.Now().Format("150405.000")
	err = sam.Send(ctx, mail.Outgoing{From: mail.Address{Email: user1}, To: []mail.Address{{Email: user2}}, Subject: subject, HTML: "<p>sealed</p>", Text: "sealed",
		Seal: func(e []byte) (string, map[string]string, []byte, error) { return pgp.SignMIME(e, signer) }})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	boxes, _ := lena.Mailboxes(ctx)
	inbox := ""
	for _, b := range boxes {
		if b.Role == mail.RoleInbox {
			inbox = b.ID
		}
	}
	var id string
	for i := 0; i < 40 && id == ""; i++ {
		page, _ := lena.Query(ctx, mail.Query{MailboxID: inbox, Limit: 50})
		for _, m := range page.Messages {
			if m.Subject == subject {
				id = m.ID
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if id == "" {
		t.Fatal("the sealed message didn't arrive")
	}
	r, err := lena.(mail.RawProvider).Raw(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(r)
	r.Close()
	o := pgp.Open(raw, nil, openpgp.EntityList{samKey})
	if o.Kind != "signed" || o.Signature == nil || o.Signature.Status != "good" || !strings.Contains(string(o.Inner), "sealed") {
		t.Errorf("signed over JMAP: %+v", o.Signature)
	}
	if m, _ := lena.Get(ctx, id); m == nil || !m.PGP {
		t.Error("not seen as PGP")
	}
	_ = lena.Destroy(ctx, []string{id})
}
