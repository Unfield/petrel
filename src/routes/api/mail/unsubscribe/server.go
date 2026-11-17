// Package unsubscribe leaves a message's mailing list.
package unsubscribe

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	ID string `json:"id" validate:"required"`
}

func POST(ctx *kit.Ctx, in In) (app.Unsubscribed, error) {
	u, err := app.Current.Mail.Unsubscribe(ctx, app.UserID(ctx), in.ID)
	if err != nil {
		return app.Unsubscribed{}, err
	}
	return *u, nil
}
