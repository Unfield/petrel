// Package mailapp is the mail app: a full-screen page of its own.
package mailapp

import (
	"encoding/json"
	"net/url"
	"regexp"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Prefs are the reader's appearance settings, kept in a cookie so the page
// renders in them from the first byte.
type Prefs struct {
	Mode    string   `json:"mode"`
	Preset  string   `json:"preset"`
	Accent  string   `json:"accent"`
	Density string   `json:"density"`
	Layout  string   `json:"layout"`
	Threads bool     `json:"threads"`
	RTL     bool     `json:"rtl"`
	ListW   int      `json:"listW"`
	ListH   int      `json:"listH"`
	Rail    bool     `json:"rail"`
	Images  []string `json:"images"`
	Labels  []Label  `json:"labels"`
}

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Page struct {
	User     User              `json:"user"`
	Accounts []app.MailAccount `json:"accounts"`
	Prefs    Prefs             `json:"prefs"`
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func oneOf(v string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return allowed[0]
}

// readPrefs takes the user's saved preferences, then this browser's
// cookie for the appearance (the most recent choice on this device).
func readPrefs(ctx *kit.Ctx, userID int64) Prefs {
	p := Prefs{Threads: true, ListW: 420, ListH: 44, Images: []string{},
		Labels: []Label{{"Work", "#3b82f6"}, {"Personal", "#e64980"}}}
	if saved := app.Current.Mail.Prefs(ctx, userID); saved != "" {
		_ = json.Unmarshal([]byte(saved), &p)
	}
	if c, ok := ctx.Cookie("petrel_prefs"); ok {
		if raw, err := url.QueryUnescape(c); err == nil {
			_ = json.Unmarshal([]byte(raw), &p)
		}
	}
	p.Mode = oneOf(p.Mode, "system", "light", "dark")
	p.Preset = oneOf(p.Preset, "slate", "paper", "fjord", "contrast")
	p.Density = oneOf(p.Density, "comfortable", "compact", "spacious")
	p.Layout = oneOf(p.Layout, "right", "below", "off")
	if !hexColor.MatchString(p.Accent) {
		p.Accent = "#3b6cf6"
	}
	p.ListW = min(max(p.ListW, 300), 900)
	p.ListH = min(max(p.ListH, 22), 75)
	if p.Images == nil {
		p.Images = []string{}
	}
	if p.Labels == nil {
		p.Labels = []Label{}
	}
	for i := range p.Labels {
		if !hexColor.MatchString(p.Labels[i].Color) {
			p.Labels[i].Color = "#7d8798"
		}
	}
	return p
}

func Load(ctx *kit.Ctx) (Page, error) {
	u := accounts.Current(ctx)
	list, err := app.Current.Mail.List(ctx, u.ID)
	if err != nil {
		return Page{}, err
	}
	return Page{User: User{Name: u.Name, Email: u.Email}, Accounts: list, Prefs: readPrefs(ctx, u.ID)}, nil
}
