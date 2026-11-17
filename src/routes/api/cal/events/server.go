// Package events lists the events in a range of time.
package events

import (
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	From time.Time `query:"from" validate:"required"`
	To   time.Time `query:"to" validate:"required"`
	// Calendars are the calendars to read, comma-separated; all if empty.
	Calendars string `query:"calendars"`
}

func GET(ctx *kit.Ctx, in In) ([]app.EventView, error) {
	var only []string
	for _, c := range strings.Split(in.Calendars, ",") {
		if c != "" {
			only = append(only, c)
		}
	}
	return app.Current.Mail.Events(ctx, app.UserID(ctx), in.From, in.To, only)
}
