// Package events streams changes to the user's mail to their open tabs.
package events

import (
	"context"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func GET(ctx *kit.Ctx) error {
	user := app.UserID(ctx)
	return ctx.EventStream(func(c context.Context, s *kit.EventStream) error {
		changes, err := app.Current.Mail.Subscribe(c, user)
		if err != nil {
			return err
		}
		if err := s.Send("ready", map[string]bool{"ok": true}); err != nil {
			return err
		}
		for {
			select {
			case <-c.Done():
				return nil
			case ch, ok := <-changes:
				if !ok {
					return nil
				}
				if err := s.Send("change", ch); err != nil {
					return err
				}
			}
		}
	})
}
