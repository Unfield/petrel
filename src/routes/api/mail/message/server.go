// Package message reads one message, cleaned to show.
package message

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	ID string `query:"id" validate:"required"`
}

func GET(ctx *kit.Ctx, in In) (app.MessageView, error) {
	v, err := app.Current.Mail.Message(ctx, app.UserID(ctx), in.ID)
	if err != nil {
		return app.MessageView{}, err
	}
	return *v, nil
}
