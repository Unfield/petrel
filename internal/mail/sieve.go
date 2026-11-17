package mail

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Filters and vacation replies run on the mail server, as a Sieve script
// (RFC 5228) that Petrel writes from rules it keeps, so they work while
// Petrel is closed. Petrel never parses Sieve: it only writes it.

// Rule is a filter: when its conditions match a new message, its actions run.
type Rule struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Enabled bool        `json:"enabled"`
	Any     bool        `json:"any"` // any condition matches, else all must
	If      []Condition `json:"if"`
	Then    []Action    `json:"then"`
}

// Condition tests one part of a message.
type Condition struct {
	// Field: from, to (To or Cc), subject, body, list (List-Id), size, or
	// header:Name for any header.
	Field string `json:"field"`
	// Op: contains, notContains, is, matches (* and ?), over, under (size
	// in KB).
	Op    string `json:"op"`
	Value string `json:"value"`
}

// Action is what a rule does.
type Action struct {
	// Type: move (Value is a mailbox path), copy, label (a keyword),
	// read, star, forward (Value is an address), delete (to Trash), stop.
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Vacation is an automatic reply.
type Vacation struct {
	Enabled bool   `json:"enabled"`
	From    string `json:"from"` // a date, 2006-01-02, or ""
	Until   string `json:"until"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	// Days between replies to the same sender.
	Days int `json:"days"`
}

// Valid reports what's wrong with a vacation reply, or "".
func (v Vacation) Valid() string {
	for _, d := range []string{v.From, v.Until} {
		if d != "" {
			if _, err := time.Parse("2006-01-02", d); err != nil {
				return "date"
			}
		}
	}
	if v.From != "" && v.Until != "" && v.Until < v.From {
		return "range"
	}
	if v.Enabled && strings.TrimSpace(v.Text) == "" {
		return "text"
	}
	return ""
}

// Valid reports what's wrong with a rule, or "".
func (r Rule) Valid() string {
	if len(r.If) == 0 || len(r.Then) == 0 {
		return "empty"
	}
	for _, c := range r.If {
		switch c.Op {
		case "contains", "notContains", "is", "matches":
			if c.Field == "size" || strings.TrimSpace(c.Value) == "" {
				return "condition"
			}
		case "over", "under":
			if n, err := strconv.Atoi(c.Value); c.Field != "size" || err != nil || n <= 0 {
				return "size"
			}
		default:
			return "condition"
		}
		switch {
		case c.Field == "from", c.Field == "to", c.Field == "subject", c.Field == "body", c.Field == "list", c.Field == "size":
		case strings.HasPrefix(c.Field, "header:") && validHeader(strings.TrimPrefix(c.Field, "header:")):
		default:
			return "field"
		}
	}
	for _, a := range r.Then {
		switch a.Type {
		case "move", "copy", "label", "forward":
			if strings.TrimSpace(a.Value) == "" {
				return "action"
			}
		case "read", "star", "delete", "stop":
		default:
			return "action"
		}
	}
	return ""
}

func validHeader(h string) bool {
	if h == "" || len(h) > 76 {
		return false
	}
	for _, c := range h {
		if c <= 32 || c >= 127 || c == ':' {
			return false
		}
	}
	return true
}

// quote is a Sieve string.
func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// SieveOptions say how the script is written for one server.
type SieveOptions struct {
	// Trash is the path of the Trash mailbox, for delete.
	Trash string
	// Me are the account's addresses, which vacation replies answer for.
	Me []string
	// NoVacation leaves the vacation reply out: the server has its own
	// (JMAP VacationResponse).
	NoVacation bool
}

// Script writes rules and a vacation reply as Sieve.
func Script(rules []Rule, v Vacation, o SieveOptions) string {
	need := map[string]bool{}
	var body strings.Builder
	body.WriteString("# Written by Petrel from its filters and vacation reply.\n# Changes made here are replaced when they're saved in Petrel.\n\n")

	if v.Enabled && !o.NoVacation && strings.TrimSpace(v.Text) != "" {
		need["vacation"] = true
		var tests []string
		if v.From != "" {
			need["date"], need["relational"] = true, true
			tests = append(tests, `currentdate :value "ge" "date" `+quote(v.From))
		}
		if v.Until != "" {
			need["date"], need["relational"] = true, true
			tests = append(tests, `currentdate :value "le" "date" `+quote(v.Until))
		}
		days := v.Days
		if days < 1 {
			days = 1
		}
		cmd := fmt.Sprintf("vacation :days %d", days)
		if v.Subject != "" {
			cmd += " :subject " + quote(v.Subject)
		}
		if len(o.Me) > 0 {
			q := make([]string, len(o.Me))
			for i, a := range o.Me {
				q[i] = quote(a)
			}
			cmd += " :addresses [" + strings.Join(q, ", ") + "]"
		}
		cmd += " " + quote(v.Text) + ";\n"
		body.WriteString("# Vacation reply\n")
		if len(tests) > 0 {
			body.WriteString("if allof (" + strings.Join(tests, ", ") + ") {\n\t" + cmd + "}\n\n")
		} else {
			body.WriteString(cmd + "\n")
		}
	}

	for _, r := range rules {
		if !r.Enabled || r.Valid() != "" {
			continue
		}
		var tests []string
		for _, c := range r.If {
			tests = append(tests, test(c, need))
		}
		join := "allof"
		if r.Any {
			join = "anyof"
		}
		var acts []string
		moved := false
		var flags []string
		for _, a := range r.Then {
			switch a.Type {
			case "read":
				flags = append(flags, `"\\Seen"`)
			case "star":
				flags = append(flags, `"\\Flagged"`)
			case "label":
				flags = append(flags, quote(a.Value))
			}
		}
		if len(flags) > 0 {
			need["imap4flags"] = true
			acts = append(acts, "addflag ["+strings.Join(flags, ", ")+"];")
		}
		for _, a := range r.Then {
			switch a.Type {
			case "move":
				need["fileinto"] = true
				acts = append(acts, "fileinto "+quote(a.Value)+";")
				moved = true
			case "copy":
				need["fileinto"], need["copy"] = true, true
				acts = append(acts, "fileinto :copy "+quote(a.Value)+";")
			case "forward":
				need["copy"] = true
				acts = append(acts, "redirect :copy "+quote(a.Value)+";")
			case "delete":
				if o.Trash != "" {
					need["fileinto"] = true
					acts = append(acts, "fileinto "+quote(o.Trash)+";")
				} else {
					acts = append(acts, "discard;")
				}
				moved = true
			}
		}
		// Flags set with addflag apply to wherever the message is filed,
		// and to the inbox copy when it isn't moved.
		if len(flags) > 0 && !moved {
			acts = append(acts, "keep;")
		}
		for _, a := range r.Then {
			if a.Type == "stop" {
				acts = append(acts, "stop;")
			}
		}
		name := strings.ReplaceAll(strings.ReplaceAll(r.Name, "\n", " "), "\r", " ")
		fmt.Fprintf(&body, "# %s\nif %s (%s) {\n", name, join, strings.Join(tests, ", "))
		for _, a := range acts {
			body.WriteString("\t" + a + "\n")
		}
		// Each rule's flags are its own.
		if len(flags) > 0 {
			body.WriteString("\tremoveflag [" + strings.Join(flags, ", ") + "];\n")
		}
		body.WriteString("}\n\n")
	}

	var req []string
	for k := range need {
		req = append(req, quote(k))
	}
	sort.Strings(req)
	if len(req) == 0 {
		return body.String()
	}
	return "require [" + strings.Join(req, ", ") + "];\n\n" + body.String()
}

func test(c Condition, need map[string]bool) string {
	if c.Field == "size" {
		return "size :" + c.Op + " " + c.Value + "K"
	}
	match := ":contains"
	switch c.Op {
	case "is":
		match = ":is"
	case "matches":
		match = ":matches"
	}
	var t string
	switch {
	case c.Field == "from":
		t = "header " + match + ` ["from", "sender"] ` + quote(c.Value)
	case c.Field == "to":
		t = "header " + match + ` ["to", "cc"] ` + quote(c.Value)
	case c.Field == "subject":
		t = "header " + match + ` "subject" ` + quote(c.Value)
	case c.Field == "list":
		t = "header " + match + ` "list-id" ` + quote(c.Value)
	case c.Field == "body":
		need["body"] = true
		t = "body :text " + match + " " + quote(c.Value)
	default:
		t = "header " + match + " " + quote(strings.ToLower(strings.TrimPrefix(c.Field, "header:"))) + " " + quote(c.Value)
	}
	if c.Op == "notContains" {
		return "not " + t
	}
	return t
}
