package mail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// jmapProvider speaks JMAP (RFC 8620, 8621): JSON over HTTPS, many method
// calls in one request.
type jmapProvider struct {
	s    Settings
	c    Credentials
	o    Options
	http *http.Client

	mu      sync.Mutex
	session *jmapSession
}

type jmapSession struct {
	APIURL          string                     `json:"apiUrl"`
	DownloadURL     string                     `json:"downloadUrl"`
	UploadURL       string                     `json:"uploadUrl"`
	EventSourceURL  string                     `json:"eventSourceUrl"`
	PrimaryAccounts map[string]string          `json:"primaryAccounts"`
	Capabilities    map[string]json.RawMessage `json:"capabilities"`
	Username        string                     `json:"username"`
	account         string
}

const (
	capCore       = "urn:ietf:params:jmap:core"
	capMail       = "urn:ietf:params:jmap:mail"
	capSubmission = "urn:ietf:params:jmap:submission"
)

func newJMAP(s Settings, c Credentials, o Options) *jmapProvider {
	h := httpClient(o.AllowPrivate, 60*time.Second)
	if o.TLS != nil {
		h.Transport.(*http.Transport).TLSClientConfig = o.TLS.Clone()
	}
	if o.Transport != nil {
		h.Transport = o.Transport
	}
	return &jmapProvider{s: s, c: c, o: o, http: h}
}

func (p *jmapProvider) auth(req *http.Request) error {
	req.Header.Set("User-Agent", "Petrel")
	if p.c.OAuth != nil {
		tok, err := token(req.Context(), &p.c, p.o)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		return nil
	}
	req.SetBasicAuth(p.c.Username, p.c.Password)
	return nil
}

func (p *jmapProvider) getSession(ctx context.Context) (*jmapSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session != nil {
		return p.session, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.s.JMAPURL, nil)
	if err != nil {
		return nil, err
	}
	if err := p.auth(req); err != nil {
		return nil, err
	}
	res, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, ErrAuth
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mail: JMAP session: %s", res.Status)
	}
	var s jmapSession
	if err := json.NewDecoder(limited(res.Body, 1<<20)).Decode(&s); err != nil {
		return nil, fmt.Errorf("mail: JMAP session: %w", err)
	}
	s.account = s.PrimaryAccounts[capMail]
	if s.account == "" || s.APIURL == "" {
		return nil, errors.New("mail: this JMAP server offers no mail account")
	}
	// Relative URLs are resolved against the session's address. They're
	// templates ({accountId}, {blobId}…), so they're joined as text: url.URL
	// would percent-encode the braces and the placeholders would stay.
	base, _ := url.Parse(p.s.JMAPURL)
	for _, u := range []*string{&s.APIURL, &s.DownloadURL, &s.UploadURL, &s.EventSourceURL} {
		if strings.HasPrefix(*u, "/") {
			*u = base.Scheme + "://" + base.Host + *u
		}
	}
	p.session = &s
	return p.session, nil
}

type invocation [3]any

// call sends method calls and returns each response's arguments by call ID.
func (p *jmapProvider) call(ctx context.Context, using []string, calls ...invocation) (map[string]json.RawMessage, error) {
	s, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{"using": append([]string{capCore, capMail}, using...), "methodCalls": calls})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.APIURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err := p.auth(req); err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return nil, ErrAuth
	}
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(limited(res.Body, 4096))
		return nil, fmt.Errorf("mail: JMAP: %s: %s", res.Status, b)
	}
	var out struct {
		MethodResponses []struct {
			Name string
			Args json.RawMessage
			ID   string
		} `json:"-"`
		Raw [][3]json.RawMessage `json:"methodResponses"`
	}
	if err := json.NewDecoder(limited(res.Body, 64<<20)).Decode(&out); err != nil {
		return nil, err
	}
	results := map[string]json.RawMessage{}
	for _, r := range out.Raw {
		var name, id string
		_ = json.Unmarshal(r[0], &name)
		_ = json.Unmarshal(r[2], &id)
		if name == "error" {
			var e struct {
				Type        string `json:"type"`
				Description string `json:"description"`
			}
			_ = json.Unmarshal(r[1], &e)
			return nil, fmt.Errorf("mail: JMAP %s: %s %s", id, e.Type, e.Description)
		}
		results[id] = r[1]
	}
	return results, nil
}

func (p *jmapProvider) accountID(ctx context.Context) (string, error) {
	s, err := p.getSession(ctx)
	if err != nil {
		return "", err
	}
	return s.account, nil
}

type jmapMailbox struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	ParentID     string `json:"parentId"`
	TotalEmails  int    `json:"totalEmails"`
	UnreadEmails int    `json:"unreadEmails"`
	SortOrder    int    `json:"sortOrder"`
}

func (p *jmapProvider) mailboxes(ctx context.Context) ([]jmapMailbox, error) {
	acc, err := p.accountID(ctx)
	if err != nil {
		return nil, err
	}
	res, err := p.call(ctx, nil, invocation{"Mailbox/get", map[string]any{"accountId": acc}, "m"})
	if err != nil {
		return nil, err
	}
	var got struct {
		List []jmapMailbox `json:"list"`
	}
	if err := json.Unmarshal(res["m"], &got); err != nil {
		return nil, err
	}
	return got.List, nil
}

func (p *jmapProvider) Mailboxes(ctx context.Context) ([]Mailbox, error) {
	list, err := p.mailboxes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Mailbox, 0, len(list))
	byID := map[string]jmapMailbox{}
	for _, m := range list {
		byID[m.ID] = m
	}
	path := func(m jmapMailbox) string {
		p := m.Name
		for i := 0; m.ParentID != "" && i < 20; i++ {
			m = byID[m.ParentID]
			p = m.Name + "/" + p
		}
		return p
	}
	for _, m := range list {
		role := m.Role
		if _, known := roleOrder[role]; !known {
			role = ""
		}
		name := m.Name
		if role == RoleInbox {
			name = "Inbox"
		}
		out = append(out, Mailbox{ID: m.ID, Name: name, Role: role, Total: m.TotalEmails, Unread: m.UnreadEmails, Path: path(m)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		oi, iok := roleOrder[out[i].Role]
		oj, jok := roleOrder[out[j].Role]
		if iok != jok {
			return iok
		}
		if iok {
			return oi < oj
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

type jmapAddr struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type jmapEmail struct {
	ID            string               `json:"id"`
	ThreadID      string               `json:"threadId"`
	MailboxIDs    map[string]bool      `json:"mailboxIds"`
	Keywords      map[string]bool      `json:"keywords"`
	From          []jmapAddr           `json:"from"`
	To            []jmapAddr           `json:"to"`
	Cc            []jmapAddr           `json:"cc"`
	Bcc           []jmapAddr           `json:"bcc"`
	ReplyTo       []jmapAddr           `json:"replyTo"`
	Subject       string               `json:"subject"`
	Preview       string               `json:"preview"`
	ReceivedAt    time.Time            `json:"receivedAt"`
	SentAt        *time.Time           `json:"sentAt"`
	HasAttachment bool                 `json:"hasAttachment"`
	MessageID     []string             `json:"messageId"`
	InReplyTo     []string             `json:"inReplyTo"`
	References    []string             `json:"references"`
	TextBody      []jmapPart           `json:"textBody"`
	HTMLBody      []jmapPart           `json:"htmlBody"`
	Attachments   []jmapPart           `json:"attachments"`
	BodyValues    map[string]bodyValue `json:"bodyValues"`
	BlobID        string               `json:"blobId"`
	Unsubscribe   []string             `json:"header:List-Unsubscribe:asURLs"`
	UnsubPost     *string              `json:"header:List-Unsubscribe-Post:asText"`
}

type jmapPart struct {
	PartID      string `json:"partId"`
	BlobID      string `json:"blobId"`
	Size        int64  `json:"size"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	CID         string `json:"cid"`
	Disposition string `json:"disposition"`
}

type bodyValue struct {
	Value string `json:"value"`
}

var summaryProps = []string{"id", "threadId", "mailboxIds", "keywords", "from", "to", "cc", "subject", "preview", "receivedAt", "sentAt", "hasAttachment", "messageId"}

func addrs(list []jmapAddr) []Address {
	out := make([]Address, 0, len(list))
	for _, a := range list {
		out = append(out, Address{Name: a.Name, Email: a.Email})
	}
	return out
}

func (e *jmapEmail) summary() Summary {
	s := Summary{
		ID: e.ID, ThreadID: e.ThreadID, Subject: e.Subject, Preview: e.Preview, Date: e.ReceivedAt,
		From: addrs(e.From), To: addrs(e.To), Cc: addrs(e.Cc), HasAttachment: e.HasAttachment,
		Seen: e.Keywords["$seen"], Flagged: e.Keywords["$flagged"], Answered: e.Keywords["$answered"],
	}
	if e.SentAt != nil && !e.SentAt.IsZero() {
		s.Date = *e.SentAt
	}
	for id := range e.MailboxIDs {
		s.MailboxID = id
		break
	}
	s.Keywords = []string{}
	for k, on := range e.Keywords {
		if on && !strings.HasPrefix(k, "$") {
			s.Keywords = append(s.Keywords, strings.ToLower(k))
		}
	}
	if len(e.MessageID) > 0 {
		s.MessageID = e.MessageID[0]
	}
	return s
}

func (p *jmapProvider) Query(ctx context.Context, q Query) (Page, error) {
	acc, err := p.accountID(ctx)
	if err != nil {
		return Page{}, err
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	var conds []any
	if q.MailboxID != "" {
		conds = append(conds, map[string]any{"inMailbox": q.MailboxID})
	} else {
		boxes, err := p.mailboxes(ctx)
		if err != nil {
			return Page{}, err
		}
		var skip []string
		for _, b := range boxes {
			if b.Role == RoleTrash || b.Role == RoleJunk {
				skip = append(skip, b.ID)
			}
		}
		if len(skip) > 0 {
			conds = append(conds, map[string]any{"inMailboxOtherThan": skip})
		}
	}
	if s := strings.TrimSpace(q.Search); s != "" {
		conds = append(conds, map[string]any{"text": s})
	}
	if q.Unread {
		conds = append(conds, map[string]any{"notKeyword": "$seen"})
	}
	if q.Flagged {
		conds = append(conds, map[string]any{"hasKeyword": "$flagged"})
	}
	if q.HasAttachment {
		conds = append(conds, map[string]any{"hasAttachment": true})
	}
	if q.Keyword != "" {
		conds = append(conds, map[string]any{"hasKeyword": q.Keyword})
	}
	if !q.Since.IsZero() {
		conds = append(conds, map[string]any{"after": q.Since.UTC().Format(time.RFC3339)})
	}
	args := map[string]any{
		"accountId": acc, "sort": []any{map[string]any{"property": "receivedAt", "isAscending": false}},
		"position": q.Offset, "limit": q.Limit, "calculateTotal": true,
	}
	if len(conds) == 1 {
		args["filter"] = conds[0]
	} else if len(conds) > 1 {
		args["filter"] = map[string]any{"operator": "AND", "conditions": conds}
	}
	res, err := p.call(ctx, nil,
		invocation{"Email/query", args, "q"},
		invocation{"Email/get", map[string]any{
			"accountId": acc, "properties": summaryProps,
			"#ids": map[string]any{"resultOf": "q", "name": "Email/query", "path": "/ids"},
		}, "g"},
	)
	if err != nil {
		return Page{}, err
	}
	var qr struct {
		IDs   []string `json:"ids"`
		Total int      `json:"total"`
	}
	var gr struct {
		List []jmapEmail `json:"list"`
	}
	if err := json.Unmarshal(res["q"], &qr); err != nil {
		return Page{}, err
	}
	if err := json.Unmarshal(res["g"], &gr); err != nil {
		return Page{}, err
	}
	byID := map[string]*jmapEmail{}
	for i := range gr.List {
		byID[gr.List[i].ID] = &gr.List[i]
	}
	page := Page{Total: qr.Total, Messages: []Summary{}}
	for _, id := range qr.IDs {
		if e, ok := byID[id]; ok {
			s := e.summary()
			if q.MailboxID != "" {
				s.MailboxID = q.MailboxID
			}
			page.Messages = append(page.Messages, s)
		}
	}
	return page, nil
}

func (p *jmapProvider) getEmail(ctx context.Context, id string) (*jmapEmail, error) {
	acc, err := p.accountID(ctx)
	if err != nil {
		return nil, err
	}
	props := append(append([]string{}, summaryProps...), "bcc", "replyTo", "inReplyTo", "references", "textBody", "htmlBody", "attachments", "bodyValues", "blobId",
		"header:List-Unsubscribe:asURLs", "header:List-Unsubscribe-Post:asText")
	res, err := p.call(ctx, nil, invocation{"Email/get", map[string]any{
		"accountId": acc, "ids": []string{id}, "properties": props,
		"fetchTextBodyValues": true, "fetchHTMLBodyValues": true, "maxBodyValueBytes": 4 << 20,
	}, "g"})
	if err != nil {
		return nil, err
	}
	var gr struct {
		List []jmapEmail `json:"list"`
	}
	if err := json.Unmarshal(res["g"], &gr); err != nil {
		return nil, err
	}
	if len(gr.List) == 0 {
		return nil, ErrNotFound
	}
	return &gr.List[0], nil
}

func (p *jmapProvider) Get(ctx context.Context, id string) (*Message, error) {
	e, err := p.getEmail(ctx, id)
	if err != nil {
		return nil, err
	}
	m := &Message{Summary: e.summary(), ReplyTo: addrs(e.ReplyTo), Bcc: addrs(e.Bcc), References: e.References, Attachments: []Attachment{}}
	post := ""
	if e.UnsubPost != nil {
		post = *e.UnsubPost
	}
	m.Unsubscribe = unsubscribe(e.Unsubscribe, post)
	if len(e.InReplyTo) > 0 {
		m.InReplyTo = e.InReplyTo[0]
	}
	var html, text strings.Builder
	hasHTML := false
	for _, part := range e.HTMLBody {
		if part.Type == "text/html" {
			hasHTML = true
		}
		html.WriteString(e.BodyValues[part.PartID].Value)
	}
	for _, part := range e.TextBody {
		text.WriteString(e.BodyValues[part.PartID].Value)
	}
	if hasHTML {
		m.HTML = html.String()
	}
	m.Text = text.String()
	for _, a := range e.Attachments {
		name := a.Name
		if name == "" {
			name = "attachment"
		}
		m.Attachments = append(m.Attachments, Attachment{
			ID: a.BlobID, Name: name, Type: a.Type, Size: a.Size, CID: strings.Trim(a.CID, "<>"),
			Inline: a.Disposition == "inline" || (a.CID != "" && a.Disposition != "attachment"),
		})
	}
	return m, nil
}

func (p *jmapProvider) Attachment(ctx context.Context, id, attID string) (Attachment, io.ReadCloser, error) {
	m, err := p.Get(ctx, id)
	if err != nil {
		return Attachment{}, nil, err
	}
	var att *Attachment
	for i, a := range m.Attachments {
		if a.ID == attID || ("cid:"+a.CID == attID && a.CID != "") {
			att = &m.Attachments[i]
		}
	}
	if att == nil {
		return Attachment{}, nil, ErrNotFound
	}
	body, err := p.download(ctx, att.ID, att.Type, att.Name)
	if err != nil {
		return Attachment{}, nil, err
	}
	return *att, body, nil
}

// download reads a blob.
func (p *jmapProvider) download(ctx context.Context, blob, typ, name string) (io.ReadCloser, error) {
	s, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	u := strings.NewReplacer("{accountId}", url.PathEscape(s.account), "{blobId}", url.PathEscape(blob),
		"{type}", url.QueryEscape(typ), "{name}", url.PathEscape(name)).Replace(s.DownloadURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if err := p.auth(req); err != nil {
		return nil, err
	}
	res, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("mail: downloading: %s", res.Status)
	}
	return res.Body, nil
}

func (p *jmapProvider) set(ctx context.Context, args map[string]any) error {
	acc, err := p.accountID(ctx)
	if err != nil {
		return err
	}
	args["accountId"] = acc
	res, err := p.call(ctx, nil, invocation{"Email/set", args, "s"})
	if err != nil {
		return err
	}
	var sr struct {
		NotUpdated   map[string]json.RawMessage `json:"notUpdated"`
		NotDestroyed map[string]json.RawMessage `json:"notDestroyed"`
	}
	_ = json.Unmarshal(res["s"], &sr)
	for _, m := range []map[string]json.RawMessage{sr.NotUpdated, sr.NotDestroyed} {
		for id, e := range m {
			return fmt.Errorf("mail: JMAP couldn't change %s: %s", id, e)
		}
	}
	return nil
}

func (p *jmapProvider) SetFlags(ctx context.Context, ids []string, seen, flagged *bool) error {
	update := map[string]any{}
	for _, id := range ids {
		patch := map[string]any{}
		if seen != nil {
			patch["keywords/$seen"] = boolOrNil(*seen)
		}
		if flagged != nil {
			patch["keywords/$flagged"] = boolOrNil(*flagged)
		}
		update[id] = patch
	}
	return p.set(ctx, map[string]any{"update": update})
}

func boolOrNil(b bool) any {
	if b {
		return true
	}
	return nil
}

func (p *jmapProvider) Move(ctx context.Context, ids []string, mailboxID string) (map[string]string, error) {
	update := map[string]any{}
	for _, id := range ids {
		update[id] = map[string]any{"mailboxIds": map[string]bool{mailboxID: true}}
	}
	return nil, p.set(ctx, map[string]any{"update": update}) // JMAP IDs don't change
}

func (p *jmapProvider) Destroy(ctx context.Context, ids []string) error {
	return p.set(ctx, map[string]any{"destroy": ids})
}

// roleBoxes finds the account's Drafts and Sent mailboxes.
func (p *jmapProvider) roleBoxes(ctx context.Context) (drafts, sent string, err error) {
	boxes, err := p.mailboxes(ctx)
	if err != nil {
		return "", "", err
	}
	for _, b := range boxes {
		switch b.Role {
		case RoleDrafts:
			drafts = b.ID
		case RoleSent:
			sent = b.ID
		}
	}
	if drafts == "" {
		return "", "", errors.New("mail: this account has no Drafts mailbox")
	}
	return drafts, sent, nil
}

// upload sends a file to the server's blob store and returns its ID.
func (p *jmapProvider) upload(ctx context.Context, f File) (string, error) {
	s, err := p.getSession(ctx)
	if err != nil {
		return "", err
	}
	u := strings.ReplaceAll(s.UploadURL, "{accountId}", url.PathEscape(s.account))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(f.Data))
	if err != nil {
		return "", err
	}
	if err := p.auth(req); err != nil {
		return "", err
	}
	typ := f.Type
	if typ == "" {
		typ = "application/octet-stream"
	}
	req.Header.Set("Content-Type", typ)
	res, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return "", fmt.Errorf("mail: uploading %s: %s", f.Name, res.Status)
	}
	var out struct {
		BlobID string `json:"blobId"`
	}
	if err := json.NewDecoder(limited(res.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	return out.BlobID, nil
}

// email turns an outgoing message into a JMAP Email to create in drafts.
func (p *jmapProvider) email(ctx context.Context, m Outgoing, drafts string) (map[string]any, error) {
	jm := func(list []Address) []jmapAddr {
		out := []jmapAddr{}
		for _, a := range list {
			out = append(out, jmapAddr{Name: a.Name, Email: a.Email})
		}
		return out
	}
	text := m.Text
	if text == "" {
		text = preview("", m.HTML, 1<<20)
	}
	email := map[string]any{
		"mailboxIds": map[string]bool{drafts: true}, "keywords": map[string]bool{"$draft": true, "$seen": true},
		"from": jm([]Address{m.From}), "to": jm(m.To), "cc": jm(m.Cc), "bcc": jm(m.Bcc), "subject": m.Subject,
		"bodyValues": map[string]any{"t": map[string]string{"value": text}},
		"textBody":   []any{map[string]string{"partId": "t", "type": "text/plain"}},
	}
	if m.HTML != "" {
		email["bodyValues"].(map[string]any)["h"] = map[string]string{"value": m.HTML}
		email["htmlBody"] = []any{map[string]string{"partId": "h", "type": "text/html"}}
	}
	if m.InReplyTo != "" {
		email["inReplyTo"] = []string{strings.Trim(m.InReplyTo, "<>")}
	}
	if len(m.ReplyTo) > 0 {
		email["replyTo"] = jm(m.ReplyTo)
	}
	if len(m.References) > 0 {
		email["references"] = m.References
	}
	var atts []any
	for _, f := range m.Attachments {
		blob, err := p.upload(ctx, f)
		if err != nil {
			return nil, err
		}
		atts = append(atts, map[string]any{"blobId": blob, "type": f.Type, "name": f.Name, "disposition": "attachment"})
	}
	if len(atts) > 0 {
		email["attachments"] = atts
	}
	return email, nil
}

func (p *jmapProvider) Send(ctx context.Context, m Outgoing) error {
	acc, err := p.accountID(ctx)
	if err != nil {
		return err
	}
	drafts, sent, err := p.roleBoxes(ctx)
	if err != nil {
		return err
	}
	res, err := p.call(ctx, []string{capSubmission}, invocation{"Identity/get", map[string]any{"accountId": acc}, "i"})
	if err != nil {
		return err
	}
	var ir struct {
		List []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"list"`
	}
	_ = json.Unmarshal(res["i"], &ir)
	identity := ""
	for _, i := range ir.List {
		if strings.EqualFold(i.Email, m.From.Email) || identity == "" {
			identity = i.ID
		}
	}
	if identity == "" {
		return errors.New("mail: this account has no identity to send as")
	}
	email, err := p.email(ctx, m, drafts)
	if err != nil {
		return err
	}
	onSuccess := map[string]any{"keywords/$draft": nil, "mailboxIds/" + drafts: nil}
	if sent != "" {
		onSuccess["mailboxIds/"+sent] = true
	}
	res, err = p.call(ctx, []string{capSubmission},
		invocation{"Email/set", map[string]any{"accountId": acc, "create": map[string]any{"draft": email}}, "e"},
		invocation{"EmailSubmission/set", map[string]any{
			"accountId":            acc,
			"create":               map[string]any{"sub": map[string]any{"identityId": identity, "emailId": "#draft"}},
			"onSuccessUpdateEmail": map[string]any{"#sub": onSuccess},
		}, "s"},
	)
	if err != nil {
		return err
	}
	for _, id := range []string{"e", "s"} {
		var sr struct {
			NotCreated map[string]json.RawMessage `json:"notCreated"`
		}
		_ = json.Unmarshal(res[id], &sr)
		for _, e := range sr.NotCreated {
			return fmt.Errorf("mail: JMAP couldn't send: %s", e)
		}
	}
	return nil
}

func (p *jmapProvider) SaveDraft(ctx context.Context, m Outgoing, replace string) (string, error) {
	acc, err := p.accountID(ctx)
	if err != nil {
		return "", err
	}
	drafts, _, err := p.roleBoxes(ctx)
	if err != nil {
		return "", err
	}
	email, err := p.email(ctx, m, drafts)
	if err != nil {
		return "", err
	}
	args := map[string]any{"accountId": acc, "create": map[string]any{"d": email}}
	if replace != "" {
		args["destroy"] = []string{replace}
	}
	res, err := p.call(ctx, nil, invocation{"Email/set", args, "s"})
	if err != nil {
		return "", err
	}
	var sr struct {
		Created    map[string]struct{ ID string } `json:"created"`
		NotCreated map[string]json.RawMessage     `json:"notCreated"`
	}
	_ = json.Unmarshal(res["s"], &sr)
	for _, e := range sr.NotCreated {
		return "", fmt.Errorf("mail: JMAP couldn't save the draft: %s", e)
	}
	return sr.Created["d"].ID, nil
}

func (p *jmapProvider) SetKeywords(ctx context.Context, ids []string, add, remove []string) error {
	update := map[string]any{}
	for _, id := range ids {
		patch := map[string]any{}
		for _, k := range add {
			patch["keywords/"+k] = true
		}
		for _, k := range remove {
			patch["keywords/"+k] = nil
		}
		update[id] = patch
	}
	return p.set(ctx, map[string]any{"update": update})
}

func (p *jmapProvider) EnsureMailbox(ctx context.Context, name string) (string, error) {
	boxes, err := p.mailboxes(ctx)
	if err != nil {
		return "", err
	}
	for _, b := range boxes {
		if strings.EqualFold(b.Name, name) {
			return b.ID, nil
		}
	}
	acc, err := p.accountID(ctx)
	if err != nil {
		return "", err
	}
	res, err := p.call(ctx, nil, invocation{"Mailbox/set", map[string]any{"accountId": acc, "create": map[string]any{"m": map[string]any{"name": name}}}, "s"})
	if err != nil {
		return "", err
	}
	var sr struct {
		Created map[string]struct{ ID string } `json:"created"`
	}
	_ = json.Unmarshal(res["s"], &sr)
	if id := sr.Created["m"].ID; id != "" {
		return id, nil
	}
	return "", fmt.Errorf("mail: JMAP couldn't create the %s mailbox", name)
}

// Watch reads the server's event stream (RFC 8620 §7.3): it sends a
// "state" event whenever anything in the account changes.
func (p *jmapProvider) Watch(ctx context.Context, changed func()) error {
	s, err := p.getSession(ctx)
	if err != nil {
		return err
	}
	if s.EventSourceURL == "" {
		return ErrUnsupported
	}
	u := strings.NewReplacer("{types}", "Email,Mailbox", "{closeafter}", "no", "{ping}", "60").Replace(s.EventSourceURL)
	stream := &http.Client{Transport: p.http.Transport} // no overall timeout: the stream stays open
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		if err := p.auth(req); err != nil {
			return err
		}
		req.Header.Set("Accept", "text/event-stream")
		res, err := stream.Do(req)
		if err == nil && res.StatusCode == http.StatusOK {
			backoff = 5 * time.Second
			sc := bufio.NewScanner(res.Body)
			for sc.Scan() {
				if strings.HasPrefix(sc.Text(), "event: state") || strings.HasPrefix(sc.Text(), "event:state") {
					changed()
				}
			}
			res.Body.Close()
		} else if res != nil {
			res.Body.Close()
			if res.StatusCode == http.StatusUnauthorized {
				return ErrAuth
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Minute)
	}
	return nil
}

func (p *jmapProvider) State(ctx context.Context) (string, error) {
	acc, err := p.accountID(ctx)
	if err != nil {
		return "", err
	}
	res, err := p.call(ctx, nil, invocation{"Email/get", map[string]any{"accountId": acc, "ids": []string{}}, "g"})
	if err != nil {
		return "", err
	}
	var gr struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(res["g"], &gr)
	return gr.State, nil
}

func (p *jmapProvider) Close() error {
	p.http.CloseIdleConnections()
	return nil
}
