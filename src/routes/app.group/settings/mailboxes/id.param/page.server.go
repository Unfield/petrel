// Package mailbox is one mailbox's settings: the addresses it sends as and
// their signatures.
package mailbox

import (
	"strconv"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

type Page struct {
	Account    app.MailAccount `json:"account"`
	Identities []app.Identity  `json:"identities"`
}

func account(ctx *kit.Ctx) (*app.MailAccount, error) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		return nil, kit.NotFound()
	}
	return app.Current.Mail.Account(ctx, app.UserID(ctx), id)
}

func Load(ctx *kit.Ctx) (Page, error) {
	a, err := account(ctx)
	if err != nil {
		return Page{}, err
	}
	all, err := app.Current.Mail.Identities(ctx, app.UserID(ctx))
	if err != nil {
		return Page{}, err
	}
	p := Page{Account: *a, Identities: []app.Identity{}}
	for _, i := range all {
		if i.AccountID == a.ID {
			p.Identities = append(p.Identities, i)
		}
	}
	return p, nil
}

type IdentityForm struct {
	ID        int64  `form:"id"`
	Name      string `form:"name" validate:"max=200"`
	Email     string `form:"email" validate:"required,email,max=254"`
	ReplyTo   string `form:"replyTo" validate:"max=1000"`
	Signature string `form:"signature" validate:"max=20000"`
}

type Remove struct {
	ID int64 `form:"id" validate:"required"`
}

type Done struct {
	Done string `json:"done"`
	ID   int64  `json:"id"`
}

var Actions = kit.Actions{
	"identity": kit.FormAction(func(ctx *kit.Ctx, in IdentityForm) (Done, error) {
		a, err := account(ctx)
		if err != nil {
			return Done{}, err
		}
		id, err := app.Current.Mail.SaveIdentity(ctx, app.UserID(ctx), app.Identity{
			ID: in.ID, AccountID: a.ID, Name: in.Name, Email: in.Email, ReplyTo: in.ReplyTo, Signature: in.Signature,
		})
		if err != nil {
			return Done{}, err
		}
		return Done{Done: "saved", ID: id.ID}, nil
	}),
	"remove": kit.FormAction(func(ctx *kit.Ctx, in Remove) (Done, error) {
		if _, err := account(ctx); err != nil {
			return Done{}, err
		}
		if err := app.Current.Mail.DeleteIdentity(ctx, app.UserID(ctx), in.ID); err != nil {
			return Done{}, err
		}
		return Done{Done: "removed"}, nil
	}),
}
