// Package lock closes the user's OpenPGP keys.
package lock

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func POST(ctx *kit.Ctx, _ struct{}) (struct{}, error) {
	app.LockKeys(app.UserID(ctx))
	return struct{}{}, nil
}
