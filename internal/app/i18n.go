package app

import (
	"context"

	"github.com/Unfield/Tern/kit"
)

// tr translates a message from src/locales for the request behind ctx.
// Background work has no request: it gets the key, which only reaches logs.
func tr(ctx context.Context, key string, params ...map[string]any) string {
	if c, ok := ctx.(*kit.Ctx); ok {
		return c.T(key, params...)
	}
	return key
}
