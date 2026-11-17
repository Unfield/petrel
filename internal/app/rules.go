package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/mail"
)

// Filters are a mailbox's rules and vacation reply, and what its server
// can do with them.
type Filters struct {
	Rules    []mail.Rule   `json:"rules"`
	Vacation mail.Vacation `json:"vacation"`
	// Supported: the server runs Sieve, so filters work.
	Supported bool `json:"supported"`
	// VacationSupported: the server can send vacation replies, with Sieve
	// or on its own.
	VacationSupported bool `json:"vacationSupported"`
	// OtherScript is another Sieve script the server runs now, which
	// saving replaces.
	OtherScript string `json:"otherScript,omitempty"`
}

const maxRules = 200

// Filters returns a mailbox's filters, asking its server what it supports.
func (m *MailAccounts) Filters(ctx context.Context, userID, accountID int64) (*Filters, error) {
	f := &Filters{Rules: []mail.Rule{}}
	var rules, vacation string
	err := m.db.QueryRowContext(ctx, `SELECT rules, vacation FROM mail_rules WHERE account_id = ? AND user_id = ?`, accountID, userID).Scan(&rules, &vacation)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		_ = json.Unmarshal([]byte(rules), &f.Rules)
		_ = json.Unmarshal([]byte(vacation), &f.Vacation)
	}
	p, _, err := m.Provider(ctx, userID, accountID)
	if err != nil {
		return f, nil // the page still shows what's saved
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if sp, ok := p.(mail.SieveProvider); ok {
		if st, err := sp.SieveStatus(cctx); err == nil {
			f.Supported = true
			f.VacationSupported = contains(st.Extensions, "vacation")
			if st.Active != "" && st.Active != mail.ScriptName {
				f.OtherScript = st.Active
			}
		}
	}
	if _, ok := p.(mail.VacationProvider); ok && !f.VacationSupported {
		f.VacationSupported = jmapHas(cctx, p, "urn:ietf:params:jmap:vacationresponse")
	}
	return f, nil
}

// jmapHas reports whether a JMAP provider's server has a capability.
func jmapHas(ctx context.Context, p mail.Provider, capability string) bool {
	c, ok := p.(interface {
		Capabilities(context.Context) ([]string, error)
	})
	if !ok {
		return false
	}
	list, err := c.Capabilities(ctx)
	return err == nil && contains(list, capability)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// SaveFilters checks rules and a vacation reply, has the server run them,
// and keeps them. Nothing is kept if the server refuses.
func (m *MailAccounts) SaveFilters(ctx context.Context, userID, accountID int64, rules []mail.Rule, v mail.Vacation) error {
	if len(rules) > maxRules {
		return kit.FormError(tr(ctx, "e.tooManyRules"))
	}
	for i := range rules {
		if rules[i].ID == "" {
			b := make([]byte, 6)
			_, _ = rand.Read(b)
			rules[i].ID = hex.EncodeToString(b)
		}
		if why := rules[i].Valid(); why != "" {
			return kit.FormError(tr(ctx, "e.rule", map[string]any{"name": rules[i].Name}))
		}
	}
	switch v.Valid() {
	case "date", "range":
		return kit.FieldError("until", tr(ctx, "e.vacationDates"))
	case "text":
		return kit.FieldError("text", tr(ctx, "e.vacationText"))
	}
	p, acc, err := m.Provider(ctx, userID, accountID)
	if err != nil {
		return MailError(ctx, err)
	}

	// The server's own vacation reply if it has one, else Sieve's.
	native := false
	if vp, ok := p.(mail.VacationProvider); ok {
		switch err := vp.SetVacation(ctx, v); {
		case err == nil:
			native = true
		case !errors.Is(err, mail.ErrUnsupported):
			return MailError(ctx, err)
		}
	}
	o := mail.SieveOptions{NoVacation: native, Me: []string{acc.Email}}
	if ids, err := m.Identities(ctx, userID); err == nil {
		for _, id := range ids {
			if id.AccountID == accountID && id.Email != acc.Email {
				o.Me = append(o.Me, id.Email)
			}
		}
	}
	if boxes, err := m.Boxes(ctx, userID, accountID, false); err == nil {
		for _, b := range boxes {
			if b.Role == mail.RoleTrash {
				o.Trash = b.Path
			}
		}
	}
	needsSieve := v.Enabled && !native
	for _, r := range rules {
		needsSieve = needsSieve || r.Enabled
	}
	sp, ok := p.(mail.SieveProvider)
	if !ok && needsSieve {
		return kit.FormError(tr(ctx, "e.noSieve"))
	}
	if ok {
		err := sp.PutSieve(ctx, mail.Script(rules, v, o))
		var refused *mail.ErrSieve
		switch {
		case errors.As(err, &refused):
			return kit.FormError(tr(ctx, "e.sieve", map[string]any{"reason": refused.Reason}))
		case errors.Is(err, mail.ErrUnsupported) && needsSieve:
			return kit.FormError(tr(ctx, "e.noSieve"))
		case err != nil && !errors.Is(err, mail.ErrUnsupported):
			return MailError(ctx, err)
		}
	}
	rj, _ := json.Marshal(rules)
	vj, _ := json.Marshal(v)
	_, err = m.db.ExecContext(ctx, `INSERT INTO mail_rules (account_id, user_id, rules, vacation, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (account_id) DO UPDATE SET rules = excluded.rules, vacation = excluded.vacation, updated_at = excluded.updated_at`,
		accountID, userID, string(rj), string(vj), time.Now().UnixMilli())
	return err
}
