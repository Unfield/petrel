// Package encryption is OpenPGP: the user's keys, and other people's.
package encryption

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

type Page struct {
	Own       []app.OwnKey    `json:"own"`
	Public    []app.PublicKey `json:"public"`
	Addresses []app.Identity  `json:"addresses"`
	Unlocked  bool            `json:"unlocked"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	m, user := app.Current.Mail, app.UserID(ctx)
	own, err := m.OwnKeys(ctx, user)
	if err != nil {
		return Page{}, err
	}
	pub, err := m.Publics(ctx, user)
	if err != nil {
		return Page{}, err
	}
	ids, err := m.Identities(ctx, user)
	if err != nil {
		return Page{}, err
	}
	return Page{Own: own, Public: pub, Addresses: ids, Unlocked: len(app.Unlocked(user)) > 0}, nil
}

type Generate struct {
	Email      string `form:"email" validate:"required,email"`
	Name       string `form:"name" validate:"max=200"`
	Passphrase string `form:"passphrase" validate:"required,max=1000"`
	Again      string `form:"again" validate:"required,max=1000"`
}

type ImportPrivate struct {
	Key        string `form:"key" validate:"required,max=200000"`
	Passphrase string `form:"passphrase" validate:"max=1000"`
	New        string `form:"new" validate:"max=1000"`
}

type ImportPublic struct {
	Key string `form:"key" validate:"required,max=200000"`
}

type Lookup struct {
	Email string `form:"email" validate:"required,email"`
}

type ID struct {
	ID    int64  `form:"id"`
	Email string `form:"email"`
}

type Done struct {
	Done   string   `json:"done"`
	Emails []string `json:"emails,omitempty"`
}

var Actions = kit.Actions{
	"generate": kit.FormAction(func(ctx *kit.Ctx, in Generate) (Done, error) {
		if in.Passphrase != in.Again {
			return Done{}, kit.FieldError("again", ctx.T("e.pgpAgain"))
		}
		_, err := app.Current.Mail.GenerateKey(ctx, app.UserID(ctx), in.Name, in.Email, in.Passphrase)
		return Done{Done: "generated"}, err
	}),
	"import": kit.FormAction(func(ctx *kit.Ctx, in ImportPrivate) (Done, error) {
		_, err := app.Current.Mail.ImportKey(ctx, app.UserID(ctx), in.Key, in.Passphrase, in.New)
		return Done{Done: "imported"}, err
	}),
	"delete": kit.FormAction(func(ctx *kit.Ctx, in ID) (Done, error) {
		return Done{Done: "deleted"}, app.Current.Mail.DeleteKey(ctx, app.UserID(ctx), in.ID)
	}),
	"public": kit.FormAction(func(ctx *kit.Ctx, in ImportPublic) (Done, error) {
		emails, err := app.Current.Mail.AddPublic(ctx, app.UserID(ctx), in.Key, "import")
		if err != nil {
			return Done{}, kit.FieldError("key", err.Error())
		}
		return Done{Done: "public", Emails: emails}, nil
	}),
	"lookup": kit.FormAction(func(ctx *kit.Ctx, in Lookup) (Done, error) {
		emails, err := app.Current.Mail.LookupWKD(ctx, app.UserID(ctx), in.Email)
		if err != nil {
			return Done{}, kit.FieldError("email", ctx.T("e.pgpNotFound"))
		}
		return Done{Done: "public", Emails: emails}, nil
	}),
	"forget": kit.FormAction(func(ctx *kit.Ctx, in ID) (Done, error) {
		return Done{Done: "forgot"}, app.Current.Mail.DeletePublic(ctx, app.UserID(ctx), in.Email)
	}),
	"lock": kit.FormAction(func(ctx *kit.Ctx, _ struct{}) (Done, error) {
		app.LockKeys(app.UserID(ctx))
		return Done{Done: "locked"}, nil
	}),
}
