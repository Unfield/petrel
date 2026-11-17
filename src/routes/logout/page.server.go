package logout

import (
	"net/http"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

// Access lets anyone see it.
var Access = kit.Public

var Actions = kit.Actions{
	"default": func(ctx *kit.Ctx) (any, error) {
		if u := accounts.Current(ctx); u != nil {
			app.Current.DirectSignOut(ctx, u.ID) // direct mode forgets the mailbox password
			app.LockKeys(u.ID)                   // and OpenPGP keys close
		}
		if err := app.Current.Accounts.SignOut(ctx); err != nil {
			return nil, err
		}
		// next, from a form that signs out to come back, such as an
		// invitation for another address.
		next := "/"
		if form, err := ctx.Form(); err == nil && form.Get("next") != "" {
			next = app.SafeNext(form.Get("next"))
		}
		return nil, kit.Redirect(http.StatusSeeOther, next)
	},
}
