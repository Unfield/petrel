// Package draft saves a message being written to the account's Drafts.
package draft

import (
	"errors"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
	"petrel/internal/mail"
)

var Access = kit.SignedIn

type Out struct {
	// ID is the saved draft's global ID, or "" where the account can't keep
	// drafts (POP3): the browser keeps it then.
	ID string `json:"id"`
}

func POST(ctx *kit.Ctx, in app.Draft) (Out, error) {
	user := app.UserID(ctx)
	p, m, err := app.Current.Mail.Outgoing(ctx, user, in, false)
	if err != nil {
		return Out{}, err
	}
	replace := ""
	if a, id, ok := app.SplitID(in.DraftID); ok && a == in.AccountID {
		replace = id
	}
	id, err := p.SaveDraft(ctx, m, replace)
	if errors.Is(err, mail.ErrUnsupported) {
		return Out{}, nil
	}
	if err != nil {
		return Out{}, app.MailError(err)
	}
	if id == "" {
		return Out{}, nil
	}
	return Out{ID: app.GlobalID(in.AccountID, id)}, nil
}
