// Package calendar opens the calendar, which lives in the mail app.
package calendar

import (
	"net/http"

	"github.com/Unfield/Tern/kit"
)

var Access = kit.SignedIn

func Load(ctx *kit.Ctx) (struct{}, error) {
	return struct{}{}, kit.Redirect(http.StatusSeeOther, "/mail#calendar")
}
