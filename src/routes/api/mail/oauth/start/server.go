// Package oauthstart sends the user to Google or Microsoft to grant
// Petrel access to their mailbox.
package oauthstart

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func GET(ctx *kit.Ctx) error {
	url, state, verifier, err := app.Current.OAuthStart(ctx.URL().Query().Get("provider"))
	if err != nil {
		return kit.NotFound()
	}
	ctx.SetCookie(&http.Cookie{Name: "petrel_oauth", Value: ctx.URL().Query().Get("provider") + "." + state + "." + verifier, MaxAge: 600, HttpOnly: true})
	return kit.Redirect(http.StatusSeeOther, url)
}
