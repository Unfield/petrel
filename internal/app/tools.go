package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/mail"
)

// The everyday tools around messages: folders, emptying the trash, the
// original message, unsubscribing and blocking.

func (m *MailAccounts) changed(userID, account int64) {
	m.forgetBoxes(account)
	_ = m.Hub.Publish("user:"+strconv.FormatInt(userID, 10), Change{AccountID: account})
}

// folders returns an account's FolderProvider and its mailboxes.
func (m *MailAccounts) folders(ctx context.Context, userID, account int64) (mail.FolderProvider, []mail.Mailbox, error) {
	p, _, err := m.Provider(ctx, userID, account)
	if err != nil {
		return nil, nil, MailError(ctx, err)
	}
	fp, ok := p.(mail.FolderProvider)
	if !ok {
		return nil, nil, kit.Error(http.StatusBadRequest, tr(ctx, "e.unsupported"))
	}
	boxes, err := m.Boxes(ctx, userID, account, true)
	if err != nil {
		return nil, nil, MailError(ctx, err)
	}
	return fp, boxes, nil
}

// ownFolder finds a folder the user made: the standard ones (Inbox,
// Trash…) can't be renamed or deleted.
func ownFolder(ctx context.Context, boxes []mail.Mailbox, id string) error {
	for _, b := range boxes {
		if b.ID == id {
			if b.Role != "" {
				return kit.Error(http.StatusBadRequest, tr(ctx, "e.standardFolder"))
			}
			return nil
		}
	}
	return kit.NotFound()
}

func folderError(ctx context.Context, err error) error {
	if errors.Is(err, mail.ErrName) {
		return kit.Error(http.StatusBadRequest, tr(ctx, "e.folderName"))
	}
	return MailError(ctx, err)
}

// CreateFolder makes a folder in an account, inside parent (a global ID)
// if given, and returns its global ID.
func (m *MailAccounts) CreateFolder(ctx context.Context, userID, account int64, name, parent string) (string, error) {
	fp, boxes, err := m.folders(ctx, userID, account)
	if err != nil {
		return "", err
	}
	pid := ""
	if parent != "" {
		a, id, ok := SplitID(parent)
		if !ok || a != account {
			return "", kit.NotFound()
		}
		pid = id
	}
	for _, b := range boxes {
		if strings.EqualFold(b.Name, strings.TrimSpace(name)) && pid == "" {
			return "", kit.Error(http.StatusBadRequest, tr(ctx, "e.folderExists"))
		}
	}
	id, err := fp.CreateMailbox(ctx, name, pid)
	if err != nil {
		return "", folderError(ctx, err)
	}
	m.changed(userID, account)
	return GlobalID(account, id), nil
}

// RenameFolder renames one of the user's own folders.
func (m *MailAccounts) RenameFolder(ctx context.Context, userID int64, folder, name string) (string, error) {
	account, id, ok := SplitID(folder)
	if !ok {
		return "", kit.NotFound()
	}
	fp, boxes, err := m.folders(ctx, userID, account)
	if err != nil {
		return "", err
	}
	if err := ownFolder(ctx, boxes, id); err != nil {
		return "", err
	}
	now, err := fp.RenameMailbox(ctx, id, name)
	if err != nil {
		return "", folderError(ctx, err)
	}
	m.changed(userID, account)
	return GlobalID(account, now), nil
}

// DeleteFolder deletes one of the user's own folders and the mail in it.
func (m *MailAccounts) DeleteFolder(ctx context.Context, userID int64, folder string) error {
	account, id, ok := SplitID(folder)
	if !ok {
		return kit.NotFound()
	}
	fp, boxes, err := m.folders(ctx, userID, account)
	if err != nil {
		return err
	}
	if err := ownFolder(ctx, boxes, id); err != nil {
		return err
	}
	if err := fp.DeleteMailbox(ctx, id); err != nil {
		return MailError(ctx, err)
	}
	m.changed(userID, account)
	return nil
}

// each runs fn on the IDs in a folder, a page at a time, newest first.
// Only matching messages count when unread is set.
func (m *MailAccounts) each(ctx context.Context, userID int64, folder string, unread bool, fn func(p mail.Provider, ids []string) error) (int64, int, error) {
	account, box, ok := SplitID(folder)
	if !ok {
		return 0, 0, kit.NotFound()
	}
	p, _, err := m.Provider(ctx, userID, account)
	if err != nil {
		return 0, 0, MailError(ctx, err)
	}
	n := 0
	for range 100 { // 50,000 messages at most
		page, err := p.Query(ctx, mail.Query{MailboxID: box, Unread: unread, Limit: 500})
		if err != nil {
			return account, n, MailError(ctx, err)
		}
		if len(page.Messages) == 0 {
			break
		}
		ids := make([]string, len(page.Messages))
		for i, s := range page.Messages {
			ids[i] = s.ID
		}
		if err := fn(p, ids); err != nil {
			return account, n, MailError(ctx, err)
		}
		n += len(ids)
	}
	return account, n, nil
}

// EmptyFolder deletes everything in a Trash or Junk folder for good.
func (m *MailAccounts) EmptyFolder(ctx context.Context, userID int64, folder string) (int, error) {
	account, box, ok := SplitID(folder)
	if !ok {
		return 0, kit.NotFound()
	}
	boxes, err := m.Boxes(ctx, userID, account, false)
	if err != nil {
		return 0, MailError(ctx, err)
	}
	allowed := false
	for _, b := range boxes {
		allowed = allowed || (b.ID == box && (b.Role == mail.RoleTrash || b.Role == mail.RoleJunk))
	}
	if !allowed {
		return 0, kit.Error(http.StatusBadRequest, tr(ctx, "e.emptyOnly"))
	}
	_, n, err := m.each(ctx, userID, folder, false, func(p mail.Provider, ids []string) error { return p.Destroy(ctx, ids) })
	m.changed(userID, account)
	return n, err
}

// MarkAllRead marks every unread message in a folder as read.
func (m *MailAccounts) MarkAllRead(ctx context.Context, userID int64, folder string) (int, error) {
	yes := true
	account, n, err := m.each(ctx, userID, folder, true, func(p mail.Provider, ids []string) error { return p.SetFlags(ctx, ids, &yes, nil) })
	if account != 0 {
		m.changed(userID, account)
	}
	return n, err
}

// Raw returns a message as it was sent.
func (m *MailAccounts) Raw(ctx context.Context, userID int64, message string) (io.ReadCloser, error) {
	account, id, ok := SplitID(message)
	if !ok {
		return nil, kit.NotFound()
	}
	p, _, err := m.Provider(ctx, userID, account)
	if err != nil {
		return nil, MailError(ctx, err)
	}
	rp, ok := p.(mail.RawProvider)
	if !ok {
		return nil, kit.Error(http.StatusBadRequest, tr(ctx, "e.unsupported"))
	}
	r, err := rp.Raw(ctx, id)
	if err != nil {
		return nil, MailError(ctx, err)
	}
	return r, nil
}

// Unsubscribed says how unsubscribing went: done, or a page to open.
type Unsubscribed struct {
	Done bool   `json:"done"`
	URL  string `json:"url,omitempty"`
}

// Unsubscribe leaves the mailing list a message came from: with one click
// if the list allows it, else by email, else by opening its page.
func (m *MailAccounts) Unsubscribe(ctx context.Context, userID int64, message string) (*Unsubscribed, error) {
	account, id, ok := SplitID(message)
	if !ok {
		return nil, kit.NotFound()
	}
	p, acc, err := m.Provider(ctx, userID, account)
	if err != nil {
		return nil, MailError(ctx, err)
	}
	msg, err := p.Get(ctx, id)
	if err != nil {
		return nil, MailError(ctx, err)
	}
	u := msg.Unsubscribe
	if u == nil {
		return nil, kit.Error(http.StatusBadRequest, tr(ctx, "e.noList"))
	}
	if u.OneClick {
		if err := mail.OneClickUnsubscribe(ctx, u.URL, m.opts); err == nil {
			return &Unsubscribed{Done: true}, nil
		}
		// The list's server failed: fall back to email or its page.
	}
	if u.Mailto != "" {
		mu, err := url.Parse(u.Mailto)
		if err == nil && mu.Opaque != "" {
			q := mu.Query()
			subject := q.Get("subject")
			if subject == "" {
				subject = "unsubscribe"
			}
			out := mail.Outgoing{From: mail.Address{Name: acc.DisplayName, Email: acc.Email}, To: []mail.Address{{Email: mu.Opaque}}, Subject: subject, Text: q.Get("body")}
			if out.Text == "" {
				out.Text = "unsubscribe"
			}
			if err := p.Send(ctx, out); err == nil {
				return &Unsubscribed{Done: true}, nil
			}
		}
	}
	if u.URL != "" {
		return &Unsubscribed{URL: u.URL}, nil
	}
	return nil, kit.Error(http.StatusBadGateway, tr(ctx, "e.unsubscribe"))
}

// Block makes a filter that sends a message's sender to the trash from now
// on, and returns the address.
func (m *MailAccounts) Block(ctx context.Context, userID int64, message string) (string, error) {
	account, id, ok := SplitID(message)
	if !ok {
		return "", kit.NotFound()
	}
	p, _, err := m.Provider(ctx, userID, account)
	if err != nil {
		return "", MailError(ctx, err)
	}
	msg, err := p.Get(ctx, id)
	if err != nil {
		return "", MailError(ctx, err)
	}
	if len(msg.From) == 0 || msg.From[0].Email == "" {
		return "", kit.Error(http.StatusBadRequest, tr(ctx, "e.noSender"))
	}
	from := strings.ToLower(msg.From[0].Email)
	f, err := m.Filters(ctx, userID, account)
	if err != nil {
		return "", err
	}
	if !f.Supported {
		return "", kit.Error(http.StatusBadRequest, tr(ctx, "e.noSieve"))
	}
	for _, r := range f.Rules {
		if len(r.If) == 1 && r.If[0].Field == "from" && r.If[0].Value == from && r.Enabled {
			return from, nil // blocked already
		}
	}
	rule := mail.Rule{Name: tr(ctx, "acc.f.blocked", map[string]any{"email": from}), Enabled: true,
		If: []mail.Condition{{Field: "from", Op: "contains", Value: from}}, Then: []mail.Action{{Type: "delete"}, {Type: "stop"}}}
	if err := m.SaveFilters(ctx, userID, account, append([]mail.Rule{rule}, f.Rules...), f.Vacation); err != nil {
		return "", kit.Error(http.StatusBadGateway, err.Error())
	}
	return from, nil
}

// Message reads one message, cleaned to show, and learns its sender as a
// contact.
func (m *MailAccounts) Message(ctx context.Context, userID int64, message string) (*MessageView, error) {
	account, id, ok := SplitID(message)
	if !ok {
		return nil, kit.NotFound()
	}
	p, _, err := m.Provider(ctx, userID, account)
	if err != nil {
		return nil, MailError(ctx, err)
	}
	msg, err := p.Get(ctx, id)
	if err != nil {
		return nil, MailError(ctx, err)
	}
	boxes, _ := m.Boxes(ctx, userID, account, false)
	m.Remember(ctx, userID, msg.From, 1)
	v := &MessageView{
		SummaryView: View(account, msg.Summary, boxes),
		ReplyTo:     msg.ReplyTo, Bcc: msg.Bcc, Body: mail.Clean(msg), Attachments: []AttachmentView{},
		Unsubscribe: msg.Unsubscribe,
	}
	for _, a := range msg.Attachments {
		v.Attachments = append(v.Attachments, AttachmentView{
			Attachment: a,
			URL:        "/api/mail/attachment?id=" + url.QueryEscape(message) + "&part=" + url.QueryEscape(a.ID),
		})
	}
	m.openPGP(ctx, userID, p, id, msg, v, message)
	return v, nil
}
