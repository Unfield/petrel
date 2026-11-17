package mail

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"mime"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/emersion/go-message/charset" // decode every charset mail uses
	"github.com/emersion/go-message"
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
	var buf bytes.Buffer
	var mw *gomail.Writer
	var w *gomail.InlineWriter
	var err error
	if len(m.Attachments) > 0 {
		if mw, err = gomail.CreateWriter(&buf, h); err != nil {
			return nil, "", err
		}
		w, err = mw.CreateInline()
	} else {
		w, err = gomail.CreateInlineWriter(&buf, h)
	}
	if err != nil {
		return nil, "", err
	}
	text := m.Text
	if text == "" {
		text = preview("", m.HTML, 1<<20)
	}
	if err := writePart(w, "text/plain", text); err != nil {
		return nil, "", err
	}
	if m.HTML != "" {
		if err := writePart(w, "text/html", m.HTML); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
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
			return nil, "", err
		}
		if _, err := aw.Write(f.Data); err != nil {
			return nil, "", err
		}
		if err := aw.Close(); err != nil {
			return nil, "", err
		}
	}
	if mw != nil {
		if err := mw.Close(); err != nil {
			return nil, "", err
		}
	}
	return buf.Bytes(), id, nil
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
