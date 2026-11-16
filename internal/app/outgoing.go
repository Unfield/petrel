package app

import (
	"net/http"
	gomail "net/mail"
	"strings"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/mail"
)

// Draft is a message being written, as the compose window sends it.
type Draft struct {
	AccountID   int64     `json:"accountId" validate:"required"`
	To          string    `json:"to"`
	Cc          string    `json:"cc"`
	Bcc         string    `json:"bcc"`
	Subject     string    `json:"subject" validate:"max=998"`
	HTML        string    `json:"html"`
	Text        string    `json:"text"`
	ReplyTo     string    `json:"replyTo"`     // the global ID of the message this answers
	Attachments []FileRef `json:"attachments"` // uploads, or parts of messages
	DraftID     string    `json:"draftId"`     // the saved draft this replaces
}

func parseList(s, field string) ([]mail.Address, error) {
	s = strings.TrimSpace(strings.Trim(strings.ReplaceAll(s, ";", ","), ", "))
	if s == "" {
		return nil, nil
	}
	list, err := gomail.ParseAddressList(s)
	if err != nil {
		return nil, kit.Error(http.StatusBadRequest, "Check the addresses in “"+field+"”.")
	}
	out := make([]mail.Address, 0, len(list))
	for _, a := range list {
		out = append(out, mail.Address{Name: a.Name, Email: a.Address})
	}
	return out, nil
}

// Outgoing turns a draft into a message to send or save, with its
// threading headers and attachments.
func (m *MailAccounts) Outgoing(ctx *kit.Ctx, userID int64, d Draft, strict bool) (mail.Provider, mail.Outgoing, error) {
	p, acc, err := m.Provider(ctx, userID, d.AccountID)
	if err != nil {
		return nil, mail.Outgoing{}, MailError(err)
	}
	out := mail.Outgoing{From: mail.Address{Name: acc.DisplayName, Email: acc.Email}, Subject: d.Subject, HTML: d.HTML, Text: d.Text}
	for _, f := range []struct {
		raw, name string
		into      *[]mail.Address
	}{{d.To, "To", &out.To}, {d.Cc, "Cc", &out.Cc}, {d.Bcc, "Bcc", &out.Bcc}} {
		list, err := parseList(f.raw, f.name)
		if err != nil && strict {
			return nil, out, err
		}
		*f.into = list
	}
	if a, id, ok := SplitID(d.ReplyTo); ok && a == d.AccountID {
		if orig, err := p.Get(ctx, id); err == nil && orig.MessageID != "" {
			out.InReplyTo = orig.MessageID
			out.References = append(append([]string{}, orig.References...), orig.MessageID)
		}
	}
	if out.Attachments, err = m.Files(ctx, userID, d.Attachments); err != nil {
		return nil, out, MailError(err)
	}
	return p, out, nil
}
