// Package send sends a message from one of the user's accounts.
package send

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
	pmail "petrel/internal/mail"
)

var Access = kit.SignedIn

type In struct {
	AccountID int64  `json:"accountId" validate:"required"`
	To        string `json:"to"`
	Cc        string `json:"cc"`
	Bcc       string `json:"bcc"`
	Subject   string `json:"subject" validate:"max=998"`
	HTML      string `json:"html"`
	Text      string `json:"text"`
	// ReplyTo is the global ID of the message this answers, for threading.
	ReplyTo string `json:"replyTo"`
}

type Out struct {
	Sent bool `json:"sent"`
}

func parseList(s, field string) ([]pmail.Address, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ";", ","))
	if s == "" {
		return nil, nil
	}
	list, err := mail.ParseAddressList(s)
	if err != nil {
		return nil, kit.Error(http.StatusBadRequest, "Check the addresses in “"+field+"”.")
	}
	out := make([]pmail.Address, 0, len(list))
	for _, a := range list {
		out = append(out, pmail.Address{Name: a.Name, Email: a.Address})
	}
	return out, nil
}

func POST(ctx *kit.Ctx, in In) (Out, error) {
	user := app.UserID(ctx)
	p, acc, err := app.Current.Mail.Provider(ctx, user, in.AccountID)
	if err != nil {
		return Out{}, app.MailError(err)
	}
	m := pmail.Outgoing{From: pmail.Address{Name: acc.DisplayName, Email: acc.Email}, Subject: in.Subject, HTML: in.HTML, Text: in.Text}
	if m.To, err = parseList(in.To, "To"); err != nil {
		return Out{}, err
	}
	if m.Cc, err = parseList(in.Cc, "Cc"); err != nil {
		return Out{}, err
	}
	if m.Bcc, err = parseList(in.Bcc, "Bcc"); err != nil {
		return Out{}, err
	}
	if len(m.To)+len(m.Cc)+len(m.Bcc) == 0 {
		return Out{}, kit.Error(http.StatusBadRequest, "Add at least one recipient.")
	}
	var replied string
	if a, id, ok := app.SplitID(in.ReplyTo); ok && a == in.AccountID {
		if orig, err := p.Get(ctx, id); err == nil {
			if orig.MessageID != "" {
				m.InReplyTo = orig.MessageID
				m.References = append(append([]string{}, orig.References...), orig.MessageID)
			}
			replied = id
		}
	}
	if err := p.Send(ctx, m); err != nil {
		return Out{}, app.MailError(err)
	}
	_ = replied
	return Out{Sent: true}, nil
}
