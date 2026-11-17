// Package event creates or changes an event.
package event

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type Out struct {
	ID string `json:"id"`
}

func POST(ctx *kit.Ctx, in app.EventInput) (Out, error) {
	id, err := app.Current.Mail.SaveEvent(ctx, app.UserID(ctx), in)
	return Out{ID: id}, err
}
