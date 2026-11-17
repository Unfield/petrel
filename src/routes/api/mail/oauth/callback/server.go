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
		return back("error", ctx.T("e.oauthState"))
	}
	if e := q.Get("error"); e != "" {
		return back("error", ctx.T("e.oauthDenied", map[string]any{"error": e}))
	}
	email, err := app.Current.OAuthFinish(ctx, app.UserID(ctx), parts[0], q.Get("code"), parts[2])
	if err != nil {
		return back("error", ctx.T("e.connect", map[string]any{"error": err.Error()}))
	}
	return back("connected", email)
}
