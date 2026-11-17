// Package print is a message laid out for paper.
package print

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type Page struct {
	Message app.MessageView `json:"message"`
	Images  bool            `json:"images"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	q := ctx.URL().Query()
	v, err := app.Current.Mail.Message(ctx, app.UserID(ctx), q.Get("id"))
	if err != nil {
		return Page{}, err
	}
	return Page{Message: *v, Images: q.Get("images") == "1"}, nil
}
