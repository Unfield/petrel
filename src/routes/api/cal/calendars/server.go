// Package calendars lists the calendars of all the user's mailboxes.
package calendars

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func GET(ctx *kit.Ctx) ([]app.CalendarView, error) {
	return app.Current.Mail.Calendars(ctx, app.UserID(ctx))
}
