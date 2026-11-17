package mail

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"github.com/emersion/go-message/textproto"
	"html"
	"io"
	"mime"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // decode every charset mail uses
	gomail "github.com/emersion/go-message/mail"
)

// parsed is what Petrel reads from a raw message.
type parsed struct {
	header      gomail.Header
	text, html  string
	attachments []Attachment
	parts       map[string]*partRef
}

type partRef struct {
	att  Attachment
	body []byte
}

const maxPart = 25 << 20 // the largest part read into memory

// parse reads a raw message. A truncated message (a preview's first few
// kilobytes) parses as far as it goes.
func parse(r io.Reader, keepBodies bool) (*parsed, error) {
	e, err := message.Read(r)
	if e == nil {
		return nil, err
	}
	p := &parsed{header: gomail.Header{Header: e.Header}, parts: map[string]*partRef{}}
	n := 0
	_ = e.Walk(func(path []int, part *message.Entity, err error) error {
		if err != nil {
			return nil
		}
		ct, params, _ := part.Header.ContentType()
		if strings.HasPrefix(ct, "multipart/") {
			return nil
		}
		disp, dparams, _ := part.Header.ContentDisposition()
		name := dparams["filename"]
		if name == "" {
			name = params["name"]
		}
		isText := (ct == "text/plain" || ct == "text/html" || ct == "") && disp != "attachment" && name == ""
		body, _ := io.ReadAll(io.LimitReader(part.Body, maxPart))
		if isText {
			s := string(body)
			if !utf8.ValidString(s) {
				s = strings.ToValidUTF8(s, "�")
			}
			if ct == "text/html" && p.html == "" {
				p.html = s
			} else if ct != "text/html" && p.text == "" {
				p.text = s
			}
			return nil
		}
		if ct == "message/delivery-status" || ct == "application/pgp-signature" && name == "" {
			return nil
		}
		n++
		id := strconv.Itoa(n)
		cid := strings.Trim(part.Header.Get("Content-Id"), "<> ")
		if name == "" {
			name = fmt.Sprintf("attachment-%d", n)
			if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
				name += exts[0]
			}
		}
		a := Attachment{ID: id, Name: name, Type: ct, Size: int64(len(body)), CID: cid, Inline: disp == "inline" || (cid != "" && disp != "attachment")}
		p.attachments = append(p.attachments, a)
		if keepBodies {
			p.parts[id] = &partRef{att: a, body: body}
		}
		return nil
	})
	return p, nil
}

func addresses(h gomail.Header, key string) []Address {
	list, err := h.AddressList(key)
	if err != nil {
		// A malformed header: keep what can be read.
		raw := h.Get(key)
		if raw == "" {
			return []Address{}
		}
		return []Address{{Email: strings.TrimSpace(raw)}}
	}
	out := make([]Address, 0, len(list))
	for _, a := range list {
		out = append(out, Address{Name: a.Name, Email: a.Address})
	}
	return out
}

func msgIDs(h gomail.Header, key string) []string {
	ids, _ := h.MsgIDList(key)
	return ids
}

// threadOf picks a thread key from the headers: the conversation's first
// message, as References lists it. Replies in one conversation share it.
func threadOf(messageID, inReplyTo string, references []string) string {
	root := messageID
	if len(references) > 0 {
		root = references[0]
	} else if inReplyTo != "" {
		root = inReplyTo
	}
	root = strings.Trim(strings.ToLower(root), "<> ")
	if root == "" {
		return ""
	}
	sum := sha1.Sum([]byte(root))
	return hex.EncodeToString(sum[:8])
}

var (
	reTags   = regexp.MustCompile(`(?is)<(script|style|head|title)[^>]*>.*?</(script|style|head|title)>|<[^>]+>`)
	reSpaces = regexp.MustCompile(`\s+`)
	reQuote  = regexp.MustCompile(`(?m)^>.*$`)
)

// preview is a message's first words, as plain text.
func preview(text, htmlBody string, n int) string {
	s := text
	if strings.TrimSpace(s) == "" {
		s = html.UnescapeString(reTags.ReplaceAllString(htmlBody, " "))
	}
	s = reQuote.ReplaceAllString(s, "")
	s = strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) > n {
		r := []rune(s)
		s = string(r[:n])
	}
	return s
}

func hasAttachments(h gomail.Header) bool {
	ct, _, _ := h.ContentType()
	return ct == "multipart/mixed"
}

// build writes an outgoing message as MIME: text and HTML alternatives.
func build(m Outgoing, hostname string) ([]byte, string, error) {
	var h gomail.Header
	h.SetDate(time.Now())
	h.SetSubject(m.Subject)
	h.SetAddressList("From", toMail([]Address{m.From}))
	if len(m.ReplyTo) > 0 {
		h.SetAddressList("Reply-To", toMail(m.ReplyTo))
	}
	if len(m.To) > 0 {
		h.SetAddressList("To", toMail(m.To))
	}
	if len(m.Cc) > 0 {
		h.SetAddressList("Cc", toMail(m.Cc))
	}
	if err := h.GenerateMessageIDWithHostname(hostname); err != nil {
		return nil, "", err
	}
	id, _ := h.MessageID()
	if m.InReplyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{m.InReplyTo})
	}
	if len(m.References) > 0 {
		h.SetMsgIDList("References", m.References)
	}
	h.Set("User-Agent", "Petrel")
	for k, v := range m.Headers {
		h.Set(k, v)
	}
	var buf bytes.Buffer
	if m.Seal == nil {
		if err := writeContent(&buf, h, m); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), id, nil
	}
	// PGP/MIME: the content as an entity of its own, sealed, under the
	// message's header.
	var entity bytes.Buffer
	if err := writeContent(&entity, gomail.Header{}, m); err != nil {
		return nil, "", err
	}
	ct, params, body, err := m.Seal(entity.Bytes())
	if err != nil {
		return nil, "", err
	}
	h.SetContentType(ct, params)
	h.Set("MIME-Version", "1.0")
	if err := textproto.WriteHeader(&buf, h.Header.Header); err != nil {
		return nil, "", err
	}
	buf.Write(body)
	return buf.Bytes(), id, nil
}

// writeContent writes a message's text, HTML and attachments under h.
func writeContent(buf *bytes.Buffer, h gomail.Header, m Outgoing) error {
	var mw *gomail.Writer
	var w *gomail.InlineWriter
	var err error
	if len(m.Attachments) > 0 {
		if mw, err = gomail.CreateWriter(buf, h); err != nil {
			return err
		}
		w, err = mw.CreateInline()
	} else {
		w, err = gomail.CreateInlineWriter(buf, h)
	}
	if err != nil {
		return err
	}
	text := m.Text
	if text == "" {
		text = preview("", m.HTML, 1<<20)
	}
	if err := writePart(w, "text/plain", text); err != nil {
		return err
	}
	if m.HTML != "" {
		if err := writePart(w, "text/html", m.HTML); err != nil {
			return err
		}
	}
	if m.Calendar != nil {
		var ph gomail.InlineHeader
		ph.SetContentType("text/calendar", map[string]string{"charset": "utf-8", "method": m.CalendarMethod})
		// iCalendar folds its lines short: sent as it is, as calendars expect.
		ph.Set("Content-Transfer-Encoding", "8bit")
		pw, err := w.CreatePart(ph)
		if err != nil {
			return err
		}
		if _, err := pw.Write(m.Calendar); err != nil {
			return err
		}
		if err := pw.Close(); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	for _, f := range m.Attachments {
		var ah gomail.AttachmentHeader
		typ := f.Type
		if typ == "" {
			typ = "application/octet-stream"
		}
		ah.SetContentType(typ, nil)
		ah.SetFilename(f.Name)
		aw, err := mw.CreateAttachment(ah)
		if err != nil {
			return err
		}
		if _, err := aw.Write(f.Data); err != nil {
			return err
		}
		if err := aw.Close(); err != nil {
			return err
		}
	}
	if mw != nil {
		return mw.Close()
	}
	return nil
}

func writePart(w *gomail.InlineWriter, typ, body string) error {
	var ph gomail.InlineHeader
	ph.SetContentType(typ, map[string]string{"charset": "utf-8"})
	ph.Set("Content-Transfer-Encoding", "quoted-printable")
	pw, err := w.CreatePart(ph)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(pw, body); err != nil {
		return err
	}
	return pw.Close()
}

func toMail(list []Address) []*gomail.Address {
	out := make([]*gomail.Address, 0, len(list))
	for _, a := range list {
		out = append(out, &gomail.Address{Name: a.Name, Address: a.Email})
	}
	return out
}

func domainOf(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return strings.ToLower(email[i+1:])
	}
	return "localhost"
}

// ParseEntity reads a MIME entity, such as a decrypted PGP/MIME message's
// content, as a message's text, HTML and attachments.
func ParseEntity(entity []byte) (html, text string, atts []Attachment, err error) {
	pr, err := parse(bytes.NewReader(entity), false)
	if err != nil {
		return "", "", nil, err
	}
	if pr.attachments == nil {
		pr.attachments = []Attachment{}
	}
	return pr.html, pr.text, pr.attachments, nil
}

// EntityPart reads one attachment of a MIME entity.
func EntityPart(entity []byte, attID string) (Attachment, []byte, error) {
	pr, err := parse(bytes.NewReader(entity), true)
	if err != nil {
		return Attachment{}, nil, err
	}
	p := findPart(pr, attID)
	if p == nil {
		return Attachment{}, nil, ErrNotFound
	}
	return p.att, p.body, nil
}

// hasPGP reports whether a message is PGP/MIME (its parts give it away) or
// holds inline PGP.
func hasPGP(m *Message) bool {
	for _, a := range m.Attachments {
		switch strings.ToLower(a.Type) {
		case "application/pgp-encrypted", "application/pgp-signature":
			return true
		}
		if strings.EqualFold(a.Name, "encrypted.asc") {
			return true
		}
	}
	for _, s := range []string{m.Text, m.HTML} {
		if strings.Contains(s, "-----BEGIN PGP MESSAGE-----") || strings.Contains(s, "-----BEGIN PGP SIGNED MESSAGE-----") {
			return true
		}
	}
	return false
}
