package mail

import (
	"context"
	"encoding/json"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Quota asks for the inbox's quota root. STORAGE is in units of 1024 bytes.
func (p *imapProvider) Quota(ctx context.Context) (*Quota, error) {
	var q *Quota
	err := p.do(ctx, func(cl *imapclient.Client) error {
		if !cl.Caps().Has(imap.CapQuota) {
			return ErrUnsupported
		}
		roots, err := cl.GetQuotaRoot("INBOX").Wait()
		if err != nil {
			return err
		}
		for _, r := range roots {
			if s, ok := r.Resources[imap.QuotaResourceStorage]; ok {
				q = &Quota{Used: s.Usage * 1024, Limit: s.Limit * 1024}
				return nil
			}
		}
		return ErrUnsupported
	})
	return q, err
}

const capQuota = "urn:ietf:params:jmap:quota"

// Quota reads the account's quotas and picks the one on its storage: the
// one for mail if the server splits them by data type.
func (p *jmapProvider) Quota(ctx context.Context) (*Quota, error) {
	s, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	if _, ok := s.Capabilities[capQuota]; !ok {
		return nil, ErrUnsupported
	}
	acc, err := p.accountID(ctx)
	if err != nil {
		return nil, err
	}
	res, err := p.call(ctx, []string{capQuota}, invocation{"Quota/get", map[string]any{"accountId": acc, "ids": nil}, "q"})
	if err != nil {
		return nil, err
	}
	var got struct {
		List []struct {
			ResourceType string   `json:"resourceType"`
			Scope        string   `json:"scope"`
			Used         int64    `json:"used"`
			HardLimit    int64    `json:"hardLimit"`
			Types        []string `json:"types"`
		} `json:"list"`
	}
	if err := json.Unmarshal(res["q"], &got); err != nil {
		return nil, err
	}
	var best *Quota
	for _, x := range got.List {
		if x.ResourceType != "octets" {
			continue
		}
		q := &Quota{Used: x.Used, Limit: x.HardLimit}
		forMail := len(x.Types) == 0
		for _, t := range x.Types {
			forMail = forMail || t == "Mail" || t == "Email"
		}
		// The account's own quota for mail beats a domain's or another type's.
		if forMail && x.Scope == "account" {
			return q, nil
		}
		if best == nil && forMail {
			best = q
		}
	}
	if best == nil {
		return nil, ErrUnsupported
	}
	return best, nil
}
