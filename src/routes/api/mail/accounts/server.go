// Package mailaccounts lists the user's mail accounts with their folders.
package mailaccounts

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func GET(ctx *kit.Ctx) ([]app.AccountView, error) {
	return app.Current.Mail.Accounts(ctx, app.UserID(ctx))
}
