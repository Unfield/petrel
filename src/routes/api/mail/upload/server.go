// Package upload keeps a file to attach to a message being written.
package upload

import (
	"io"
	"net/http"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func POST(ctx *kit.Ctx) error {
	r := ctx.Request
	r.Body = http.MaxBytesReader(ctx.Writer(), r.Body, app.MaxUpload+1<<20)
	f, h, err := r.FormFile("file")
	if err != nil {
		return kit.Error(http.StatusBadRequest, "Attachments can be 25 MB at most.")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, app.MaxUpload+1))
	if err != nil || len(data) > app.MaxUpload {
		return kit.Error(http.StatusRequestEntityTooLarge, "Attachments can be 25 MB at most.")
	}
	typ := h.Header.Get("Content-Type")
	if typ == "" {
		typ = http.DetectContentType(data)
	}
	u, err := app.Current.Mail.SaveUpload(ctx, app.UserID(ctx), h.Filename, typ, data)
	if err != nil {
		return err
	}
	return ctx.JSON(http.StatusOK, u)
}
