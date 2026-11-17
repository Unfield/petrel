// Package block filters a sender's mail into the trash from now on.
package block

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	ID string `json:"id" validate:"required"`
}

type Out struct {
	Email string `json:"email"`
}

func POST(ctx *kit.Ctx, in In) (Out, error) {
	email, err := app.Current.Mail.Block(ctx, app.UserID(ctx), in.ID)
	return Out{Email: email}, err
}
