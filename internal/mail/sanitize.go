package mail

import (
	"html"
	"regexp"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
)

// Mail's HTML is written by strangers. Petrel cleans it here (no scripts,
// forms, frames or event handlers) and the browser shows it in a sandboxed
// frame with its own Content-Security-Policy, which also keeps remote
// images, and the trackers in them, from loading until the reader allows them.

var (
	policyOnce sync.Once
	policy     *bluemonday.Policy
)

func htmlPolicy() *bluemonday.Policy {
	policyOnce.Do(func() {
		p := bluemonday.UGCPolicy()
		p.AllowElements("font", "center", "span", "div", "small", "big", "u", "s", "strike", "hr")
		p.AllowAttrs("style", "class", "align", "valign", "width", "height", "bgcolor", "border",
			"cellpadding", "cellspacing", "color", "face", "size", "dir", "background").Globally()
		p.AllowUnsafe(false)
		p.AllowURLSchemes("http", "https", "mailto", "cid", "tel")
		p.AllowDataURIImages()
		p.RequireNoFollowOnLinks(true)
		p.AddTargetBlankToFullyQualifiedLinks(true)
		p.RequireNoReferrerOnLinks(true)
		policy = p
	})
	return policy
}

var reRemote = regexp.MustCompile(`(?i)(src|background)\s*=\s*["']?\s*(https?:)?//|url\(\s*['"]?\s*(https?:)?//`)

// Body is a message's content, ready to show.
type Body struct {
	HTML      string `json:"html"`
	Plain     bool   `json:"plain"`
	HasRemote bool   `json:"hasRemote"`
}

// Clean makes a message's body safe to show. Plain text becomes HTML with
// its links made clickable.
func Clean(m *Message) Body {
	if strings.TrimSpace(m.HTML) != "" {
		out := htmlPolicy().Sanitize(m.HTML)
		return Body{HTML: out, HasRemote: reRemote.MatchString(out)}
	}
	return Body{HTML: textToHTML(m.Text), Plain: true}
}

var reLink = regexp.MustCompile(`https?://[^\s<>"']+[^\s<>"'.,;:!?)\]]`)

func textToHTML(s string) string {
	var b strings.Builder
	quote := 0
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		level := 0
		for strings.HasPrefix(line, ">") {
			level++
			line = strings.TrimPrefix(strings.TrimPrefix(line, ">"), " ")
		}
		for quote < level {
			b.WriteString("<blockquote>")
			quote++
		}
		for quote > level {
			b.WriteString("</blockquote>")
			quote--
		}
		esc := html.EscapeString(line)
		esc = reLink.ReplaceAllStringFunc(esc, func(u string) string {
			return `<a href="` + u + `" target="_blank" rel="noopener noreferrer nofollow">` + u + `</a>`
		})
		b.WriteString(esc)
		b.WriteString("<br>")
	}
	for ; quote > 0; quote-- {
		b.WriteString("</blockquote>")
	}
	return `<div class="petrel-plain">` + b.String() + `</div>`
}
