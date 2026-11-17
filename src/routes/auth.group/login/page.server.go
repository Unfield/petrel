package login

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

// Access lets anyone in: this is how people sign in.
var Access = kit.Public

type Credentials struct {
	Email    string `form:"email" validate:"required,email"`
	Password string `form:"password" validate:"required"`
	Remember bool   `form:"remember"`
	Next     string `form:"next"`
}

type Page struct {
	Direct    bool           `json:"direct"`
	Next      string         `json:"next"`
	Error     string         `json:"error"`
	Providers []app.Provider `json:"providers"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	q := ctx.URL().Query()
	next := app.SafeNext(q.Get("next"))
	if accounts.Current(ctx) != nil {
		return Page{}, kit.Redirect(http.StatusSeeOther, next)
	}
	// error is what sign-in with GitHub or Google sends back when it fails.
	return Page{Direct: app.Current.Direct(), Next: next, Error: q.Get("error"), Providers: app.Current.Providers}, nil
}

type Mailbox struct {
	Email    string `form:"email" validate:"required,email,max=254" label:"Email address"`
	Password string `form:"password" validate:"required,max=500"`
	Next     string `form:"next"`
}

var Actions = kit.Actions{
	// mailbox signs in with a mailbox's own address and password (direct mode).
	"mailbox": kit.FormAction(func(ctx *kit.Ctx, in Mailbox) (struct{}, error) {
		if !app.Current.Direct() {
			return struct{}{}, kit.NotFound()
		}
		if err := app.Current.DirectSignIn(ctx, in.Email, in.Password); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, app.SafeNext(in.Next))
	}),
	"default": kit.FormAction(func(ctx *kit.Ctx, in Credentials) (struct{}, error) {
		if app.Current.Direct() {
			return struct{}{}, kit.FormError(ctx.T("e.useMailbox"))
		}
		next := app.SafeNext(in.Next)
		_, err := app.Current.Accounts.SignIn(ctx, in.Email, in.Password, in.Remember)
		if errors.Is(err, accounts.ErrTwoStepRequired) {
			return struct{}{}, kit.Redirect(http.StatusSeeOther, "/login/code?next="+url.QueryEscape(next))
		}
		if err != nil {
			return struct{}{}, accounts.FormError(ctx, err)
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, next)
	}),
}
