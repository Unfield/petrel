// Package mailbox is one mailbox's settings: the addresses it sends as and
// their signatures.
package mailbox

import (
	"encoding/json"
	"strconv"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
	"petrel/internal/mail"
)

type Page struct {
	Account    app.MailAccount `json:"account"`
	Identities []app.Identity  `json:"identities"`
	Filters    app.Filters     `json:"filters"`
	// Folders are the mailboxes filters can file into.
	Folders []Folder `json:"folders"`
}

type Folder struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Role string `json:"role"`
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
	p := Page{Account: *a, Identities: []app.Identity{}, Folders: []Folder{}}
	f, err := app.Current.Mail.Filters(ctx, app.UserID(ctx), a.ID)
	if err != nil {
		return Page{}, err
	}
	p.Filters = *f
	if boxes, err := app.Current.Mail.Boxes(ctx, app.UserID(ctx), a.ID, false); err == nil {
		for _, b := range boxes {
			if b.Path != "" && b.Role != mail.RoleDrafts && b.Role != mail.RoleSent {
				p.Folders = append(p.Folders, Folder{Path: b.Path, Name: b.Name, Role: b.Role})
			}
		}
	}
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

// RulesForm is the whole list of filters, as JSON from the page.
type RulesForm struct {
	Rules string `form:"rules" validate:"max=200000"`
}

type VacationForm struct {
	Enabled bool   `form:"enabled"`
	From    string `form:"from" validate:"max=10"`
	Until   string `form:"until" validate:"max=10"`
	Subject string `form:"subject" validate:"max=200"`
	Text    string `form:"text" validate:"max=10000"`
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
	"filters": kit.FormAction(func(ctx *kit.Ctx, in RulesForm) (Done, error) {
		a, err := account(ctx)
		if err != nil {
			return Done{}, err
		}
		var rules []mail.Rule
		if err := json.Unmarshal([]byte(in.Rules), &rules); err != nil {
			return Done{}, kit.FormError(ctx.T("e.rulesJSON"))
		}
		f, err := app.Current.Mail.Filters(ctx, app.UserID(ctx), a.ID)
		if err != nil {
			return Done{}, err
		}
		if err := app.Current.Mail.SaveFilters(ctx, app.UserID(ctx), a.ID, rules, f.Vacation); err != nil {
			return Done{}, err
		}
		return Done{Done: "filters"}, nil
	}),
	"vacation": kit.FormAction(func(ctx *kit.Ctx, in VacationForm) (Done, error) {
		a, err := account(ctx)
		if err != nil {
			return Done{}, err
		}
		f, err := app.Current.Mail.Filters(ctx, app.UserID(ctx), a.ID)
		if err != nil {
			return Done{}, err
		}
		v := mail.Vacation{Enabled: in.Enabled, From: in.From, Until: in.Until, Subject: in.Subject, Text: in.Text, Days: 1}
		if err := app.Current.Mail.SaveFilters(ctx, app.UserID(ctx), a.ID, f.Rules, v); err != nil {
			return Done{}, err
		}
		return Done{Done: "vacation"}, nil
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
