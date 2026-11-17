// Package raw serves a message as it was sent: to read its headers, or to
// download as an .eml file.
package raw

import (
	"io"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func GET(ctx *kit.Ctx) error {
	q := ctx.URL().Query()
	r, err := app.Current.Mail.Raw(ctx, app.UserID(ctx), q.Get("id"))
	if err != nil {
		return err
	}
	defer r.Close()
	w := ctx.Writer()
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, no-store")
	if q.Get("download") != "" {
		h.Set("Content-Type", "message/rfc822")
		h.Set("Content-Disposition", `attachment; filename="message.eml"`)
	} else {
		// Shown as text, never as HTML.
		h.Set("Content-Type", "text/plain; charset=utf-8")
	}
	_, err = io.Copy(w, io.LimitReader(r, 50<<20))
	return err
}
