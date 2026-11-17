// Package recipients says which addresses Petrel has keys for, and looks
// up the rest in their domains' Web Key Directories.
package recipients

import (
	"strings"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	Emails string `query:"emails" validate:"max=5000"`
	// Lookup asks WKD for the addresses without a key.
	Lookup bool `query:"lookup"`
}

func GET(ctx *kit.Ctx, in In) (map[string]bool, error) {
	user := app.UserID(ctx)
	var list []string
	for _, e := range strings.Split(in.Emails, ",") {
		if e = strings.TrimSpace(e); e != "" && len(list) < 50 {
			list = append(list, e)
		}
	}
	has := app.Current.Mail.HasKeys(ctx, user, list)
	if in.Lookup {
		for e, ok := range has {
			if !ok {
				if _, err := app.Current.Mail.LookupWKD(ctx, user, e); err == nil {
					has[e] = true
				}
			}
		}
	}
	return has, nil
}
