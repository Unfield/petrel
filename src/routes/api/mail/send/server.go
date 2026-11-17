// Package send sends a message from one of the user's accounts.
package send

import (
	"context"
	"net/http"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type Out struct {
	Sent bool `json:"sent"`
}

func POST(ctx *kit.Ctx, in app.Draft) (Out, error) {
	user := app.UserID(ctx)
	p, m, err := app.Current.Mail.Outgoing(ctx, user, in, true)
	if err != nil {
		return Out{}, err
	}
	if len(m.To)+len(m.Cc)+len(m.Bcc) == 0 {
		return Out{}, kit.Error(http.StatusBadRequest, ctx.T("e.noRecipient"))
	}
	if err := p.Send(ctx, m); err != nil {
		return Out{}, app.MailError(ctx, err)
	}
	bg := context.WithoutCancel(ctx)
	if a, id, ok := app.SplitID(in.DraftID); ok && a == in.AccountID {
		_ = p.Destroy(bg, []string{id}) // sent: the draft goes
	}
	if a, id, ok := app.SplitID(in.ReplyTo); ok && a == in.AccountID {
		_ = p.SetKeywords(bg, []string{id}, []string{"$answered"}, nil)
	}
	app.Current.Mail.Remember(bg, user, append(append(m.To, m.Cc...), m.Bcc...), 3)
	app.Current.Mail.DropUploads(bg, user, in.Attachments)
	return Out{Sent: true}, nil
}
