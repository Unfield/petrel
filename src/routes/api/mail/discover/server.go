// Package discover finds the servers for an email address.
package discover

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
	"petrel/internal/mail"
)

var Access = kit.SignedIn

type In struct {
	Email string `query:"email" validate:"required,email"`
}

func GET(ctx *kit.Ctx, in In) (mail.Discovered, error) {
	return mail.Discover(ctx, in.Email, app.Current.Mail.AllowPrivate()), nil
}
