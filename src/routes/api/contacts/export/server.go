// Package export downloads the address book as vCards.
package export

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

func GET(ctx *kit.Ctx) error {
	h := ctx.Writer().Header()
	h.Set("Content-Type", "text/vcard; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="contacts.vcf"`)
	h.Set("Cache-Control", "private, no-store")
	return app.Current.Mail.ExportContacts(ctx, app.UserID(ctx), ctx.Writer())
}
