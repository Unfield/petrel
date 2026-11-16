// Command devmail is a mail server for developing Petrel: IMAP and SMTP in
// memory, with two mailboxes full of sample mail. Nothing it holds is kept.
//
//	go run ./cmd/devmail
//
// Then connect a mailbox in Petrel (with PETREL_ALLOW_PRIVATE_HOSTS=true)
// as sam@petrel.test, password "petrel", IMAP 127.0.0.1:1143 and SMTP
// 127.0.0.1:1587, both unencrypted. Mail sent to either sample address
// arrives in its inbox, and a new message arrives a minute after start, to
// see live updates.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	gomail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-smtp"
)

var users = map[string]*imapmemserver.User{}

func main() {
	imapAddr := flag.String("imap", "127.0.0.1:1143", "IMAP address")
	smtpAddr := flag.String("smtp", "127.0.0.1:1587", "SMTP address")
	drip := flag.Duration("drip", time.Minute, "deliver a new sample message after this long (0 to turn off)")
	flag.Parse()

	mem := imapmemserver.New()
	for _, addr := range []string{"sam@petrel.test", "sam.taylor@northwind.test"} {
		u := imapmemserver.NewUser(addr, "petrel")
		for _, box := range []string{"INBOX", "Drafts", "Sent", "Archive", "Junk", "Trash"} {
			if err := u.Create(box, nil); err != nil {
				log.Fatal(err)
			}
		}
		mem.AddUser(u)
		users[addr] = u
	}
	seed()

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		InsecureAuth: true,
	})
	go func() { log.Fatal(srv.ListenAndServe(*imapAddr)) }()

	s := smtp.NewServer(backend{})
	s.Addr, s.Domain, s.AllowInsecureAuth = *smtpAddr, "localhost", true
	go func() { log.Fatal(s.ListenAndServe()) }()

	if *drip > 0 {
		go func() {
			time.Sleep(*drip)
			deliver("sam.taylor@northwind.test", "INBOX", sample{
				from: priya, to: []string{"sam.taylor@northwind.test"}, subject: "Lunch today?",
				text: "Hey Sam! A few of us are getting ramen at 12:30. Want to join? 🍜\n\nPriya", ago: 0, unseen: true,
			})
			log.Print("devmail: delivered a new message to sam.taylor@northwind.test")
		}()
	}
	log.Printf("devmail: IMAP on %s, SMTP on %s. Sign in as sam@petrel.test or sam.taylor@northwind.test, password \"petrel\"", *imapAddr, *smtpAddr)
	select {}
}

// The SMTP side delivers to the sample mailboxes' inboxes.
type backend struct{}

func (backend) NewSession(*smtp.Conn) (smtp.Session, error) { return &session{}, nil }

type session struct{ to []string }

func (s *session) Mail(string, *smtp.MailOptions) error { return nil }
func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.to = append(s.to, strings.ToLower(to))
	return nil
}
func (s *session) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	for _, to := range s.to {
		if u, ok := users[to]; ok {
			if _, err := u.Append("INBOX", literal(b), &imap.AppendOptions{Time: time.Now()}); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *session) Reset()        { s.to = nil }
func (s *session) Logout() error { return nil }

type literalReader struct {
	*bytes.Reader
	size int64
}

func (l literalReader) Size() int64 { return l.size }

func literal(b []byte) imap.LiteralReader { return literalReader{bytes.NewReader(b), int64(len(b))} }

type person struct{ name, email string }

var (
	sam   = person{"Sam Taylor", "sam@petrel.test"}
	samW  = person{"Sam Taylor", "sam.taylor@northwind.test"}
	anna  = person{"Anna Berg", "anna.berg@northwind.test"}
	jonas = person{"Jonas Weber", "jonas.weber@northwind.test"}
	mia   = person{"Mia Chen", "mia.chen@northwind.test"}
	david = person{"David Okafor", "david.okafor@northwind.test"}
	priya = person{"Priya Nair", "priya.nair@northwind.test"}
	lena  = person{"Lena Fischer", "lena.fischer@mailbox.test"}
	mom   = person{"Karen Taylor", "karen.taylor@mailbox.test"}
	omar  = person{"Omar Haddad", "omar@haddad.test"}
	cloud = person{"Cloudnest Billing", "billing@cloudnest.test"}
	sky   = person{"Skyline Travel", "bookings@skyline-travel.test"}
	byte_ = person{"The Weekly Byte", "hello@weeklybyte.test"}
	itsec = person{"Northwind IT", "it-security@northwind.test"}
	prize = person{"Prize Center", "win@totally-legit.test"}
)

type file struct {
	name, typ string
	body      []byte
	cid       string
}

type sample struct {
	from       person
	to         []string
	subject    string
	text, html string
	ago        time.Duration
	unseen     bool
	flagged    bool
	id         string
	replyTo    string // the Message-ID this answers
	refs       []string
	files      []file
}

var pdf = []byte("%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj 2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj 3 0 obj<</Type/Page/MediaBox[0 0 200 100]/Parent 2 0 R>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n")

// A 1×1 PNG, for the inline image.
var png = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x60, 0x68, 0xf8, 0xff, 0x1f, 0x00, 0x03, 0x82, 0x01, 0xff, 0x6e, 0x61, 0x0b, 0x8c, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}

func seed() {
	h := time.Hour
	day := 24 * h
	w := "sam.taylor@northwind.test"
	p := "sam@petrel.test"

	// A conversation of three, with Sam's reply in Sent.
	deliver(w, "INBOX", sample{from: anna, to: []string{w, jonas.email, mia.email}, subject: "Q4 roadmap review — notes & next steps", ago: 26 * h, id: "roadmap-1@northwind.test",
		text:  "Hi team,\n\nThanks for a productive session today. I've attached the updated roadmap with the changes we discussed: mainly moving offline sync to January and pulling the onboarding revamp into December.\n\nCould everyone review their sections by Thursday?\n\nBest,\nAnna",
		files: []file{{name: "Roadmap-Q4.pdf", typ: "application/pdf", body: pdf}}})
	deliver(w, "Sent", sample{from: samW, to: []string{anna.email, jonas.email, mia.email}, subject: "Re: Q4 roadmap review — notes & next steps", ago: 5 * h, id: "roadmap-2@northwind.test",
		replyTo: "roadmap-1@northwind.test", refs: []string{"roadmap-1@northwind.test"},
		text: "Thanks Anna! Looks good from my side.\n\nOne question: does moving offline sync affect the beta timeline we promised to the pilot customers?\n\nSam"})
	deliver(w, "INBOX", sample{from: jonas, to: []string{w, anna.email, mia.email}, subject: "Re: Q4 roadmap review — notes & next steps", ago: 12 * time.Minute, unseen: true, id: "roadmap-3@northwind.test",
		replyTo: "roadmap-2@northwind.test", refs: []string{"roadmap-1@northwind.test", "roadmap-2@northwind.test"},
		text: "Good catch, Sam. I talked to the pilot group this morning: they're fine with a mid-January beta as long as we share a short status update every two weeks.\n\nI'll draft the first one and send it around tomorrow.\n\nJonas\n\n> One question: does moving offline sync affect the beta timeline\n> we promised to the pilot customers?"})

	deliver(w, "INBOX", sample{from: cloud, to: []string{w}, subject: "Your invoice INV-2048 from Cloudnest", ago: 3 * h,
		html:  `<div style="font-family:Arial,sans-serif;max-width:560px;margin:auto"><h2 style="color:#0f766e">Cloudnest</h2><p>Hi Sam,</p><p>Your invoice for November is ready. The amount of <b>€249.00</b> will be charged to the card on file on November 20.</p><table style="border-collapse:collapse;width:100%"><tr><td style="padding:6px;border-bottom:1px solid #ddd">Plan</td><td style="padding:6px;border-bottom:1px solid #ddd">Team (25 seats)</td></tr><tr><td style="padding:6px">Period</td><td style="padding:6px">Nov 1 – Nov 30</td></tr></table><p><img src="cid:logo@cloudnest" width="24" height="24" alt=""> Thanks for using Cloudnest!</p></div>`,
		files: []file{{name: "INV-2048.pdf", typ: "application/pdf", body: pdf}, {name: "logo.png", typ: "image/png", body: png, cid: "logo@cloudnest"}}})
	deliver(w, "INBOX", sample{from: mia, to: []string{w}, subject: "Design crit tomorrow at 10:00", ago: 25 * h, unseen: true,
		text: "Hi all,\n\nQuick reminder that we're doing the design crit for the new settings screens tomorrow at 10:00 in the Fjord room (and on video for remote folks).\n\nPlease bring one thing you'd change about the current density options. I'd love to hear opinions!\n\nMia"})
	deliver(w, "INBOX", sample{from: david, to: []string{w}, subject: "API rate limits for the mobile app", ago: 3 * day, flagged: true,
		text: "Hey Sam,\n\nWe're seeing 429s from the sync endpoint on app start for users with many accounts. The current limit is 60 req/min per token. Could we raise it or batch the calls? See https://example.com/dashboards/sync for the graphs.\n\nDavid"})
	deliver(w, "INBOX", sample{from: itsec, to: []string{w}, subject: "Security alert: new sign-in to your account", ago: 10 * day,
		text: "We noticed a new sign-in to your Northwind account from Safari on macOS in Berlin, Germany.\n\nIf this was you, you can ignore this message. If not, please reset your password and contact IT."})
	deliver(w, "Archive", sample{from: person{"EventHub", "events@eventhub.test"}, to: []string{w}, subject: "Conference badge pickup", ago: 28 * day,
		text: "Your badge for DevConf Berlin can be picked up at the registration desk from 08:00."})

	deliver(p, "INBOX", sample{from: lena, to: []string{p}, subject: "Wochenende in Hamburg?", ago: 64 * time.Minute, unseen: true,
		text: "Hey Sam!\n\nWir haben überlegt, Anfang Dezember für ein Wochenende nach Hamburg zu fahren. Hast du Lust mitzukommen? Wir könnten am Samstag auf den Weihnachtsmarkt und abends essen gehen. 🎄\n\nLiebe Grüße\nLena"})
	deliver(p, "INBOX", sample{from: sky, to: []string{p}, subject: "Flight confirmation: BER → LIS, Dec 12", ago: 5 * h, flagged: true,
		html:  `<table width="100%" style="background:#f4f6fb;font-family:Helvetica,Arial,sans-serif"><tr><td align="center" style="padding:24px"><table width="560" style="background:#fff;border-radius:12px"><tr><td><img src="https://images.example.com/skyline/header.png" width="560" height="140" alt="Skyline Travel"></td></tr><tr><td style="padding:24px"><h1 style="font-size:22px;color:#1d3557">Your trip is confirmed! ✈️</h1><p><b>Booking reference:</b> K7Q2XZ</p><p><b>Outbound:</b> Fri, Dec 12 · BER 07:35 → LIS 10:20<br><b>Return:</b> Tue, Dec 16 · LIS 18:05 → BER 22:40</p><p><a href="https://example.com/manage" style="background:#e63946;color:#fff;padding:10px 16px;border-radius:8px;text-decoration:none">Manage booking</a></p><img src="https://track.example.com/open.gif?u=sam" width="1" height="1" alt=""></td></tr></table></td></tr></table><script>alert("this never runs")</script>`,
		files: []file{{name: "E-ticket-K7Q2XZ.pdf", typ: "application/pdf", body: pdf}}})
	deliver(p, "INBOX", sample{from: byte_, to: []string{p}, subject: "The Weekly Byte — Issue #142: Tiny tools, big wins", ago: 28 * h,
		html: `<div style="max-width:600px;margin:auto;font-family:Georgia,serif;color:#222"><img src="https://cdn.example.com/weeklybyte/142.jpg" width="600" height="200" alt="The Weekly Byte"><h1>Tiny tools, big wins</h1><p>This week: why small CLI tools keep winning, a gentle intro to JMAP, and five keyboard-driven apps worth trying.</p><h3>1. The case for tiny tools</h3><p>Single-purpose utilities are easier to trust, easier to replace, and easier to compose…</p><h3>2. JMAP in 10 minutes</h3><p>The modern mail protocol replaces IMAP's stateful sessions with plain JSON over HTTP…</p><p><a href="https://example.com/issues/142">Read the full issue</a></p></div>`})
	deliver(p, "INBOX", sample{from: mom, to: []string{p}, subject: "Photos from Sunday 📷", ago: 2 * day, flagged: true,
		text:  "Hi sweetie,\n\nHere are the photos from Sunday's lunch. Grandpa loved the cake! Let me know if you want the originals, these are a bit smaller.\n\nLove,\nMom",
		files: []file{{name: "IMG_2041.png", typ: "image/png", body: png}, {name: "IMG_2042.png", typ: "image/png", body: png}}})
	deliver(p, "INBOX", sample{from: omar, to: []string{p}, subject: "Book club: December pick", ago: 8 * day,
		text: "Hi book people!\n\nVotes are in: December's pick is The Left Hand of Darkness. We'll meet on the 18th at my place. Bring snacks!\n\nOmar"})
	deliver(p, "Drafts", sample{from: sam, to: []string{mom.email}, subject: "Holiday plans", ago: 10 * h, text: "Hi Mom, quick idea for the holidays: what if we all meet in…"})
	deliver(p, "Junk", sample{from: prize, to: []string{p}, subject: "You have WON a €500 gift card!!!", ago: 15 * h, unseen: true,
		html: `<h1 style="color:red">CONGRATULATIONS!!!</h1><img src="http://totally-legit.test/claim.png"><p><a href="http://totally-legit.test/claim">Click here to claim your prize</a> before it expires!!!</p><form action="http://totally-legit.test/steal"><input name="card"></form>`})
}

func deliver(user, box string, s sample) {
	u := users[user]
	raw, err := build(s)
	if err != nil {
		log.Fatal(err)
	}
	var flags []imap.Flag
	if !s.unseen {
		flags = append(flags, imap.FlagSeen)
	}
	if s.flagged {
		flags = append(flags, imap.FlagFlagged)
	}
	if box == "Drafts" {
		flags = append(flags, imap.FlagDraft)
	}
	if _, err := u.Append(box, literal(raw), &imap.AppendOptions{Flags: flags, Time: time.Now().Add(-s.ago)}); err != nil {
		log.Fatal(err)
	}
}

func build(s sample) ([]byte, error) {
	var h gomail.Header
	h.SetDate(time.Now().Add(-s.ago))
	h.SetSubject(s.subject)
	h.SetAddressList("From", []*gomail.Address{{Name: s.from.name, Address: s.from.email}})
	var to []*gomail.Address
	for _, t := range s.to {
		to = append(to, &gomail.Address{Address: t})
	}
	h.SetAddressList("To", to)
	if s.id != "" {
		h.SetMessageID(s.id)
	} else if err := h.GenerateMessageIDWithHostname("devmail.test"); err != nil {
		return nil, err
	}
	if s.replyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{s.replyTo})
	}
	if len(s.refs) > 0 {
		h.SetMsgIDList("References", s.refs)
	}
	var buf bytes.Buffer
	var inline *gomail.InlineWriter
	var mw *gomail.Writer
	var err error
	if len(s.files) > 0 {
		if mw, err = gomail.CreateWriter(&buf, h); err != nil {
			return nil, err
		}
		if inline, err = mw.CreateInline(); err != nil {
			return nil, err
		}
	} else if inline, err = gomail.CreateInlineWriter(&buf, h); err != nil {
		return nil, err
	}
	text := s.text
	if text == "" {
		// The plain-text alternative, as mail programs write it.
		text = strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(regexp.MustCompile(`(?s)<script.*?</script>|<[^>]+>`).ReplaceAllString(s.html, " "), " "))
	}
	for _, part := range []struct{ typ, body string }{{"text/plain", text}, {"text/html", s.html}} {
		if part.body == "" {
			continue
		}
		var ph gomail.InlineHeader
		ph.SetContentType(part.typ, map[string]string{"charset": "utf-8"})
		pw, err := inline.CreatePart(ph)
		if err != nil {
			return nil, err
		}
		io.WriteString(pw, part.body)
		pw.Close()
	}
	if err := inline.Close(); err != nil {
		return nil, err
	}
	for _, f := range s.files {
		var ah gomail.AttachmentHeader
		ah.SetContentType(f.typ, nil)
		ah.SetFilename(f.name)
		if f.cid != "" {
			ah.Set("Content-Id", "<"+f.cid+">")
			ah.Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", f.name))
		}
		aw, err := mw.CreateAttachment(ah)
		if err != nil {
			return nil, err
		}
		aw.Write(f.body)
		aw.Close()
	}
	if mw != nil {
		if err := mw.Close(); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

var _ = errors.New
