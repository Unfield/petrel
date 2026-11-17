// Package contacts is the address book.
package contacts

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

type Page struct {
	Saved   []app.Contact `json:"saved"`
	Learned []app.Contact `json:"learned"`
	Q       string        `json:"q"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	q := ctx.URL().Query().Get("q")
	user := app.UserID(ctx)
	saved, err := app.Current.Mail.Contacts(ctx, user, true, q, 2000)
	if err != nil {
		return Page{}, err
	}
	learned, err := app.Current.Mail.Contacts(ctx, user, false, q, 200)
	if err != nil {
		return Page{}, err
	}
	return Page{Saved: saved, Learned: learned, Q: q}, nil
}

type ContactForm struct {
	Was     string `form:"was" validate:"max=254"`
	Email   string `form:"email" validate:"required,email,max=254"`
	Name    string `form:"name" validate:"max=200"`
	Phone   string `form:"phone" validate:"max=50"`
	Company string `form:"company" validate:"max=200"`
	Notes   string `form:"notes" validate:"max=4000"`
}

type Remove struct {
	Email string `form:"email" validate:"required"`
}

type Done struct {
	Done string `json:"done"`
	N    int    `json:"n"`
}

var Actions = kit.Actions{
	"save": kit.FormAction(func(ctx *kit.Ctx, in ContactForm) (Done, error) {
		err := app.Current.Mail.SaveContact(ctx, app.UserID(ctx), app.Contact{Email: in.Email, Name: in.Name, Phone: in.Phone, Company: in.Company, Notes: in.Notes}, in.Was)
		return Done{Done: "saved"}, err
	}),
	"remove": kit.FormAction(func(ctx *kit.Ctx, in Remove) (Done, error) {
		return Done{Done: "removed"}, app.Current.Mail.DeleteContact(ctx, app.UserID(ctx), in.Email)
	}),
	// import takes a .vcf file.
	"import": func(ctx *kit.Ctx) (any, error) {
		f, _, err := ctx.Request.FormFile("file")
		if err != nil {
			return nil, kit.FormError(ctx.T("acc.c.noFile"))
		}
		defer f.Close()
		n, err := app.Current.Mail.ImportContacts(ctx, app.UserID(ctx), f)
		if err != nil {
			return nil, kit.FormError(ctx.T("acc.c.badFile"))
		}
		return Done{Done: "imported", N: n}, nil
	},
}
