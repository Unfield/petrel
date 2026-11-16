// Package messages is the message list: one folder, or a role across accounts.
package messages

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func GET(ctx *kit.Ctx, in app.ListQuery) (app.ListResult, error) {
	return app.Current.Mail.Query(ctx, app.UserID(ctx), in)
}
