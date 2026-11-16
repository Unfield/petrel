// Package oauthcallback finishes connecting a mailbox with OAuth.
package oauthcallback

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func GET(ctx *kit.Ctx) error {
	back := func(param, value string) error {
		return kit.Redirect(http.StatusSeeOther, "/settings/mailboxes?"+param+"="+url.QueryEscape(value))
	}
	c, ok := ctx.Cookie("petrel_oauth")
	ctx.DeleteCookie("petrel_oauth")
	parts := strings.SplitN(c, ".", 3)
	q := ctx.URL().Query()
	if !ok || len(parts) != 3 || q.Get("state") != parts[1] {
		return back("error", "The sign-in took too long or came from elsewhere. Try again.")
	}
	if e := q.Get("error"); e != "" {
		return back("error", "Access wasn’t granted ("+e+").")
	}
	email, err := app.Current.OAuthFinish(ctx, app.UserID(ctx), parts[0], q.Get("code"), parts[2])
	if err != nil {
		return back("error", "Couldn’t connect: "+err.Error())
	}
	return back("connected", email)
}
