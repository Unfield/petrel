package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Contact is someone in the address book, saved by the user or learned
// from mail.
type Contact struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Company string `json:"company"`
	Notes   string `json:"notes"`
	Saved   bool   `json:"saved"`
	Uses    int    `json:"uses"`
}

// Contacts lists the user's saved contacts, or the learned ones, matching q.
func (m *MailAccounts) Contacts(ctx context.Context, userID int64, saved bool, q string, limit int) ([]Contact, error) {
	like := "%" + strings.NewReplacer("%", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(q))) + "%"
	order := "uses DESC, last_used DESC"
	if saved {
		order = "CASE WHEN name = '' THEN email ELSE lower(name) END"
	}
	rows, err := m.db.QueryContext(ctx, `SELECT email, name, phone, company, notes, saved, uses FROM contacts
		WHERE user_id = ? AND saved = ? AND (email LIKE ? OR lower(name) LIKE ? OR lower(company) LIKE ?)
		ORDER BY `+order+` LIMIT ?`, userID, saved, like, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Contact{}
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.Email, &c.Name, &c.Phone, &c.Company, &c.Notes, &c.Saved, &c.Uses); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = s[:n]
	}
	return s
}

// SaveContact saves a contact to the address book. Was is its old address
// when the address changes.
func (m *MailAccounts) SaveContact(ctx context.Context, userID int64, c Contact, was string) error {
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	if a, err := mail.ParseAddress(c.Email); err != nil || a.Address != c.Email {
		return kit.FieldError("email", tr(ctx, "e.identityEmail"))
	}
	c.Name, c.Phone, c.Company, c.Notes = clip(c.Name, 200), clip(c.Phone, 50), clip(c.Company, 200), clip(c.Notes, 4000)
	was = strings.ToLower(strings.TrimSpace(was))
	if was != "" && was != c.Email {
		if _, err := m.db.ExecContext(ctx, `DELETE FROM contacts WHERE user_id = ? AND email = ?`, userID, was); err != nil {
			return err
		}
	}
	_, err := m.db.ExecContext(ctx, `INSERT INTO contacts (user_id, email, name, phone, company, notes, saved, uses, last_used) VALUES (?, ?, ?, ?, ?, ?, 1, 0, ?)
		ON CONFLICT (user_id, email) DO UPDATE SET name = excluded.name, phone = excluded.phone, company = excluded.company, notes = excluded.notes, saved = 1`,
		userID, c.Email, c.Name, c.Phone, c.Company, c.Notes, time.Now().UnixMilli())
	return err
}

// DeleteContact removes someone from the address book and suggestions.
func (m *MailAccounts) DeleteContact(ctx context.Context, userID int64, email string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM contacts WHERE user_id = ? AND email = ?`, userID, strings.ToLower(strings.TrimSpace(email)))
	return err
}

// vcf escapes a vCard value.
func vcf(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, ",", `\,`, ";", `\;`).Replace(s)
}

// ExportContacts writes the saved contacts as vCard 3.0.
func (m *MailAccounts) ExportContacts(ctx context.Context, userID int64, w io.Writer) error {
	list, err := m.Contacts(ctx, userID, true, "", 100000)
	if err != nil {
		return err
	}
	for _, c := range list {
		fn := c.Name
		if fn == "" {
			fn = c.Email
		}
		fmt.Fprintf(w, "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:%s\r\nN:%s;;;;\r\nEMAIL;TYPE=INTERNET:%s\r\n", vcf(fn), vcf(fn), vcf(c.Email))
		if c.Phone != "" {
			fmt.Fprintf(w, "TEL:%s\r\n", vcf(c.Phone))
		}
		if c.Company != "" {
			fmt.Fprintf(w, "ORG:%s\r\n", vcf(c.Company))
		}
		if c.Notes != "" {
			fmt.Fprintf(w, "NOTE:%s\r\n", vcf(c.Notes))
		}
		fmt.Fprint(w, "END:VCARD\r\n")
	}
	return nil
}

// ImportContacts reads vCards (2.1 to 4.0) and saves each that has an email
// address, returning how many.
func (m *MailAccounts) ImportContacts(ctx context.Context, userID int64, r io.Reader) (int, error) {
	sc := bufio.NewScanner(io.LimitReader(r, 10<<20))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	// Unfold: a line starting with a space continues the one before.
	var lines []string
	for sc.Scan() {
		l := strings.TrimRight(sc.Text(), "\r")
		if (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
			continue
		}
		lines = append(lines, l)
	}
	unescape := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	n := 0
	var c *Contact
	for _, l := range lines {
		key, value, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(strings.ToUpper(key), ";")
		if i := strings.LastIndexByte(name, '.'); i >= 0 { // item1.EMAIL
			name = name[i+1:]
		}
		value = unescape.Replace(value)
		switch name {
		case "BEGIN":
			c = &Contact{}
		case "FN":
			if c != nil {
				c.Name = value
			}
		case "EMAIL":
			if c != nil && c.Email == "" {
				c.Email = value
			}
		case "TEL":
			if c != nil && c.Phone == "" {
				c.Phone = strings.TrimPrefix(value, "tel:")
			}
		case "ORG":
			if c != nil {
				c.Company = strings.TrimRight(strings.ReplaceAll(value, ";", " "), " ")
			}
		case "NOTE":
			if c != nil {
				c.Notes = value
			}
		case "END":
			if c != nil && c.Email != "" && m.SaveContact(ctx, userID, *c, "") == nil {
				n++
			}
			c = nil
		}
	}
	return n, sc.Err()
}
