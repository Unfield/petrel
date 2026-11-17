package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	capSieve    = "urn:ietf:params:jmap:sieve"
	capVacation = "urn:ietf:params:jmap:vacationresponse"
)

// VacationProvider is a provider whose server has its own vacation reply
// (JMAP VacationResponse), which Petrel uses instead of Sieve's.
type VacationProvider interface {
	SetVacation(ctx context.Context, v Vacation) error
}

func (p *jmapProvider) has(ctx context.Context, capability string) (bool, error) {
	s, err := p.getSession(ctx)
	if err != nil {
		return false, err
	}
	_, ok := s.Capabilities[capability]
	return ok, nil
}

// methodError is a JMAP method's error response, if res is one.
func methodError(res json.RawMessage) error {
	var e struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	}
	if json.Unmarshal(res, &e) == nil && e.Type != "" {
		return fmt.Errorf("mail: %s: %s", e.Type, e.Description)
	}
	return nil
}

func (p *jmapProvider) SieveStatus(ctx context.Context) (*SieveStatus, error) {
	ok, err := p.has(ctx, capSieve)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrUnsupported
	}
	s, _ := p.getSession(ctx)
	var c struct {
		SieveExtensions []string `json:"sieveExtensions"`
	}
	_ = json.Unmarshal(s.Capabilities[capSieve], &c)
	acc, err := p.accountID(ctx)
	if err != nil {
		return nil, err
	}
	res, err := p.call(ctx, []string{capSieve}, invocation{"SieveScript/get", map[string]any{"accountId": acc, "properties": []string{"name", "isActive"}}, "g"})
	if err != nil {
		return nil, err
	}
	var got struct {
		List []struct {
			Name     string `json:"name"`
			IsActive bool   `json:"isActive"`
		} `json:"list"`
	}
	_ = json.Unmarshal(res["g"], &got)
	st := &SieveStatus{Extensions: c.SieveExtensions}
	for _, x := range got.List {
		if x.IsActive {
			st.Active = x.Name
		}
	}
	return st, nil
}

func (p *jmapProvider) PutSieve(ctx context.Context, script string) error {
	ok, err := p.has(ctx, capSieve)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUnsupported
	}
	acc, err := p.accountID(ctx)
	if err != nil {
		return err
	}
	blob, err := p.upload(ctx, File{Name: ScriptName + ".sieve", Type: "application/sieve", Data: []byte(script)})
	if err != nil {
		return err
	}
	res, err := p.call(ctx, []string{capSieve},
		invocation{"SieveScript/validate", map[string]any{"accountId": acc, "blobId": blob}, "v"},
		invocation{"SieveScript/get", map[string]any{"accountId": acc, "properties": []string{"name"}}, "g"},
	)
	if err != nil {
		return err
	}
	var v struct {
		Error *struct {
			Description string `json:"description"`
		} `json:"error"`
	}
	_ = json.Unmarshal(res["v"], &v)
	if v.Error != nil {
		return &ErrSieve{Reason: v.Error.Description}
	}
	var got struct {
		List []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"list"`
	}
	_ = json.Unmarshal(res["g"], &got)
	args := map[string]any{"accountId": acc}
	id := ""
	for _, x := range got.List {
		if x.Name == ScriptName {
			id = x.ID
		}
	}
	if id == "" {
		args["create"] = map[string]any{"s": map[string]any{"name": ScriptName, "blobId": blob}}
		args["onSuccessActivateScript"] = "#s"
	} else {
		args["update"] = map[string]any{id: map[string]any{"blobId": blob}}
		args["onSuccessActivateScript"] = id
	}
	res, err = p.call(ctx, []string{capSieve}, invocation{"SieveScript/set", args, "s"})
	if err != nil {
		return err
	}
	if err := methodError(res["s"]); err != nil {
		return err
	}
	var sr struct {
		NotCreated, NotUpdated map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		}
	}
	_ = json.Unmarshal(res["s"], &sr)
	for _, m := range []map[string]struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	}{sr.NotCreated, sr.NotUpdated} {
		for _, e := range m {
			return &ErrSieve{Reason: strings.TrimSpace(e.Type + " " + e.Description)}
		}
	}
	return nil
}

// SetVacation sets the server's own vacation reply. Dates are whole days in
// UTC: from the start of From to the end of Until.
func (p *jmapProvider) SetVacation(ctx context.Context, v Vacation) error {
	ok, err := p.has(ctx, capVacation)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUnsupported
	}
	acc, err := p.accountID(ctx)
	if err != nil {
		return err
	}
	patch := map[string]any{"isEnabled": v.Enabled, "subject": nilIfEmpty(v.Subject), "textBody": nilIfEmpty(v.Text), "htmlBody": nil, "fromDate": nil, "toDate": nil}
	if d, err := time.Parse("2006-01-02", v.From); err == nil {
		patch["fromDate"] = d.UTC().Format(time.RFC3339)
	}
	if d, err := time.Parse("2006-01-02", v.Until); err == nil {
		patch["toDate"] = d.Add(24*time.Hour - time.Second).UTC().Format(time.RFC3339)
	}
	res, err := p.call(ctx, []string{capVacation}, invocation{"VacationResponse/set", map[string]any{"accountId": acc, "update": map[string]any{"singleton": patch}}, "v"})
	if err != nil {
		return err
	}
	if err := methodError(res["v"]); err != nil {
		return err
	}
	var sr struct {
		NotUpdated map[string]struct {
			Description string `json:"description"`
		} `json:"notUpdated"`
	}
	_ = json.Unmarshal(res["v"], &sr)
	for _, e := range sr.NotUpdated {
		return fmt.Errorf("mail: the server refused the vacation reply: %s", e.Description)
	}
	return nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Capabilities lists the JMAP server's capabilities.
func (p *jmapProvider) Capabilities(ctx context.Context) ([]string, error) {
	s, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(s.Capabilities))
	for k := range s.Capabilities {
		out = append(out, k)
	}
	return out, nil
}
