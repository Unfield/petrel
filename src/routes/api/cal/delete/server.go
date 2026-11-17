// Package delete deletes an event.
package delete

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	ID string `json:"id" validate:"required"`
}

func POST(ctx *kit.Ctx, in In) (struct{}, error) {
	return struct{}{}, app.Current.Mail.DeleteEvent(ctx, app.UserID(ctx), in.ID)
}
