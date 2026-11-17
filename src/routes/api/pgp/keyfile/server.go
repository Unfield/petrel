// Package keyfile imports a public key attached to a message.
package keyfile

import (
	"io"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	ID   string `json:"id" validate:"required"`
	Part string `json:"part" validate:"required"`
}

type Out struct {
	Emails []string `json:"emails"`
}

func POST(ctx *kit.Ctx, in In) (Out, error) {
	account, id, ok := app.SplitID(in.ID)
	if !ok {
		return Out{}, kit.NotFound()
	}
	user := app.UserID(ctx)
	p, _, err := app.Current.Mail.Provider(ctx, user, account)
	if err != nil {
		return Out{}, app.MailError(ctx, err)
	}
	_, body, err := p.Attachment(ctx, id, in.Part)
	if err != nil {
		return Out{}, app.MailError(ctx, err)
	}
	defer body.Close()
	data, _ := io.ReadAll(io.LimitReader(body, 1<<20))
	emails, err := app.Current.Mail.AddPublic(ctx, user, string(data), "import")
	return Out{Emails: emails}, err
}
