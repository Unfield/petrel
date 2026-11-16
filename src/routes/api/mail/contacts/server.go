// Package contacts suggests addresses while writing.
package contacts

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
	"petrel/internal/mail"
)

var Access = kit.SignedIn

type In struct {
	Q string `query:"q"`
}

func GET(ctx *kit.Ctx, in In) ([]mail.Address, error) {
	return app.Current.Mail.Suggest(ctx, app.UserID(ctx), in.Q, 8)
}
