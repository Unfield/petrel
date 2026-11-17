// Package respond answers an invitation that came by mail.
package respond

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	ID     string `json:"id" validate:"required"`
	Part   string `json:"part" validate:"required"`
	Status string `json:"status" validate:"required,oneof=accepted|tentative|declined"`
}

func POST(ctx *kit.Ctx, in In) (struct{}, error) {
	return struct{}{}, app.Current.Mail.RespondInvite(ctx, app.UserID(ctx), in.ID, in.Part, in.Status)
}
