// Package unlock opens the user's OpenPGP keys for a while.
package unlock

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	Passphrase string `json:"passphrase" validate:"required,max=1000"`
}

type Out struct {
	Unlocked int `json:"unlocked"`
}

func POST(ctx *kit.Ctx, in In) (Out, error) {
	n, err := app.Current.Mail.UnlockKeys(ctx, app.UserID(ctx), in.Passphrase)
	return Out{Unlocked: n}, err
}
