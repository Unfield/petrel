// Package status says whether the user has OpenPGP keys, and if they're open.
package status

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type Out struct {
	Keys     []string `json:"keys"` // the addresses with keys
	Unlocked bool     `json:"unlocked"`
}

func GET(ctx *kit.Ctx) (Out, error) {
	user := app.UserID(ctx)
	own, err := app.Current.Mail.OwnKeys(ctx, user)
	if err != nil {
		return Out{}, err
	}
	out := Out{Keys: []string{}, Unlocked: len(app.Unlocked(user)) > 0}
	for _, k := range own {
		out.Keys = append(out.Keys, k.Email)
	}
	return out, nil
}
