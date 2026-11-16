// Package message reads one message, cleaned to show.
package message

import (
	"net/url"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
	"petrel/internal/mail"
)

var Access = kit.SignedIn

type In struct {
	ID string `query:"id" validate:"required"`
}

func GET(ctx *kit.Ctx, in In) (app.MessageView, error) {
	account, id, ok := app.SplitID(in.ID)
	if !ok {
		return app.MessageView{}, kit.NotFound()
	}
	user := app.UserID(ctx)
	p, _, err := app.Current.Mail.Provider(ctx, user, account)
	if err != nil {
		return app.MessageView{}, app.MailError(err)
	}
	m, err := p.Get(ctx, id)
	if err != nil {
		return app.MessageView{}, app.MailError(err)
	}
	boxes, _ := app.Current.Mail.Boxes(ctx, user, account, false)
	app.Current.Mail.Remember(ctx, user, m.From, 1)
	v := app.MessageView{
		SummaryView: app.View(account, m.Summary, boxes),
		ReplyTo:     m.ReplyTo, Bcc: m.Bcc, Body: mail.Clean(m), Attachments: []app.AttachmentView{},
	}
	for _, a := range m.Attachments {
		v.Attachments = append(v.Attachments, app.AttachmentView{
			Attachment: a,
			URL:        "/api/mail/attachment?id=" + url.QueryEscape(in.ID) + "&part=" + url.QueryEscape(a.ID),
		})
	}
	return v, nil
}
