package mail

import (
	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

type Page struct {
	Accounts      []app.MailAccount `json:"accounts"`
	EmailVerified bool              `json:"emailVerified"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	u := accounts.Current(ctx)
	list, err := app.Current.Mail.List(ctx.Request.Context(), u.ID)
	return Page{Accounts: list, EmailVerified: u.EmailVerified}, err
}

var Actions = kit.Actions{
	"verify": func(ctx *kit.Ctx) (any, error) {
		if err := app.Current.Accounts.SendVerification(ctx); err != nil {
			return nil, err
		}
		return map[string]bool{"sent": true}, nil
	},
}
