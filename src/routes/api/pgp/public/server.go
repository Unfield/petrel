// Package public downloads one of the user's public keys, to share.
package public

import (
	"strconv"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func GET(ctx *kit.Ctx) error {
	id, _ := strconv.ParseInt(ctx.URL().Query().Get("id"), 10, 64)
	own, err := app.Current.Mail.OwnKeys(ctx, app.UserID(ctx))
	if err != nil {
		return err
	}
	for _, k := range own {
		if k.ID == id {
			h := ctx.Writer().Header()
			h.Set("Content-Type", "application/pgp-keys")
			h.Set("Content-Disposition", `attachment; filename="`+k.Email+`.asc"`)
			_, err := ctx.Writer().Write([]byte(k.Public))
			return err
		}
	}
	return kit.NotFound()
}
