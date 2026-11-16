// Package prefs saves the user's appearance settings and labels.
package prefs

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func POST(ctx *kit.Ctx) error {
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, 64<<10+1))
	if err != nil || len(body) > 64<<10 || !json.Valid(body) {
		return kit.Error(http.StatusBadRequest, "Settings are too large or not JSON.")
	}
	if err := app.Current.Mail.SavePrefs(ctx, app.UserID(ctx), string(body)); err != nil {
		return err
	}
	return ctx.JSON(http.StatusOK, map[string]bool{"saved": true})
}
