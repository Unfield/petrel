package routes

import (
	"net/http"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

type Home struct {
	User *accounts.User `json:"user"`
}

// Load shows the product page, except on a direct-sign-in installation:
// that's someone's own mail server, so / opens the inbox or the sign-in.
func Load(ctx *kit.Ctx) (Home, error) {
	u := accounts.Current(ctx)
	if app.Current.Direct() {
		if u != nil {
			return Home{}, kit.Redirect(http.StatusSeeOther, "/mail")
		}
		return Home{}, kit.Redirect(http.StatusSeeOther, "/login")
	}
	return Home{User: u}, nil
}
