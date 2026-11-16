// Package attachment downloads an attachment, or serves an inline image.
package attachment

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

// safeInline are the types a browser may show itself; anything else is
// downloaded, so a message can't run a page on Petrel's origin.
var safeInline = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "application/pdf": true, "text/plain": true}

func GET(ctx *kit.Ctx) error {
	q := ctx.URL().Query()
	account, id, ok := app.SplitID(q.Get("id"))
	if !ok {
		return kit.NotFound()
	}
	p, _, err := app.Current.Mail.Provider(ctx, app.UserID(ctx), account)
	if err != nil {
		return app.MailError(err)
	}
	att, body, err := p.Attachment(ctx, id, q.Get("part"))
	if err != nil {
		return app.MailError(err)
	}
	defer body.Close()
	w := ctx.Writer()
	typ := strings.ToLower(att.Type)
	disposition := "attachment"
	if safeInline[typ] && q.Get("download") == "" {
		disposition = "inline"
	} else if !safeInline[typ] {
		typ = "application/octet-stream"
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": att.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, body)
	return err
}
