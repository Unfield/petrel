package app

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"petrel/internal/mail"
)

// The browser sees one ID space across accounts: "<account>~<id>".

// GlobalID joins an account and a provider's ID.
func GlobalID(account int64, id string) string { return strconv.FormatInt(account, 10) + "~" + id }

// SplitID splits a global ID.
func SplitID(id string) (int64, string, bool) {
	a, rest, ok := strings.Cut(id, "~")
	if !ok {
		return 0, "", false
	}
	n, err := strconv.ParseInt(a, 10, 64)
	return n, rest, err == nil
}

// MailboxView is a folder, for the sidebar.
type MailboxView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Total  int    `json:"total"`
	Unread int    `json:"unread"`
}

// AccountView is an account and its folders. Error is set when its server
// couldn't be reached, so the others still show.
type AccountView struct {
	MailAccount
	Mailboxes []MailboxView `json:"mailboxes"`
	Error     string        `json:"error,omitempty"`
}

// SummaryView is a message in the list.
type SummaryView struct {
	mail.Summary
	ID          string `json:"id"`
	ThreadID    string `json:"threadId"`
	MailboxID   string `json:"mailboxId"`
	MailboxRole string `json:"mailboxRole"`
	AccountID   int64  `json:"accountId"`
}

// AttachmentView is an attachment and where to download it.
type AttachmentView struct {
	mail.Attachment
	URL string `json:"url"`
}

// MessageView is a whole message, cleaned to show.
type MessageView struct {
	SummaryView
	ReplyTo     []mail.Address   `json:"replyTo"`
	Bcc         []mail.Address   `json:"bcc"`
	Attachments []AttachmentView `json:"attachments"`
	Body        mail.Body        `json:"body"`
}

// UserID is the signed-in user's ID.
func UserID(ctx *kit.Ctx) int64 { return accounts.Current(ctx).ID }

// MailError turns mail errors into answers the user can act on.
func MailError(ctx context.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNoAccount), errors.Is(err, mail.ErrNotFound):
		return kit.NotFound()
	case errors.Is(err, mail.ErrAuth):
		return kit.Error(http.StatusBadGateway, tr(ctx, "e.auth"))
	case errors.Is(err, mail.ErrUnsupported):
		return kit.Error(http.StatusBadRequest, tr(ctx, "e.unsupported"))
	case errors.Is(err, mail.ErrBlocked):
		return kit.Error(http.StatusBadRequest, tr(ctx, "e.blocked"))
	case errors.Is(err, context.Canceled):
		return err
	}
	var ke *kit.HTTPError
	if errors.As(err, &ke) {
		return err
	}
	return kit.Error(http.StatusBadGateway, tr(ctx, "e.server", map[string]any{"error": err.Error()}))
}

// boxCache keeps each account's folders for a short while: queries need
// their roles, and asking the server every time would be slow.
var boxCache = struct {
	sync.Mutex
	m map[int64]boxEntry
}{m: map[int64]boxEntry{}}

type boxEntry struct {
	boxes []mail.Mailbox
	at    time.Time
}

// Boxes returns an account's folders, from the cache if fresh is false.
func (m *MailAccounts) Boxes(ctx context.Context, userID, id int64, fresh bool) ([]mail.Mailbox, error) {
	boxCache.Lock()
	e, ok := boxCache.m[id]
	boxCache.Unlock()
	if ok && !fresh && time.Since(e.at) < 30*time.Second {
		return e.boxes, nil
	}
	p, _, err := m.Provider(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	boxes, err := p.Mailboxes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range boxes {
		if boxes[i].Role == "" && strings.EqualFold(boxes[i].Name, SnoozedName) {
			boxes[i].Role = "snoozed"
		}
	}
	boxCache.Lock()
	boxCache.m[id] = boxEntry{boxes: boxes, at: time.Now()}
	boxCache.Unlock()
	return boxes, nil
}

// Accounts returns every account of the user with its folders, asking the
// servers at once.
func (m *MailAccounts) Accounts(ctx context.Context, userID int64) ([]AccountView, error) {
	list, err := m.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]AccountView, len(list))
	var wg sync.WaitGroup
	for i, a := range list {
		out[i] = AccountView{MailAccount: a, Mailboxes: []MailboxView{}}
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			boxes, err := m.Boxes(cctx, userID, a.ID, true)
			if err != nil {
				out[i].Error = MailError(ctx, err).Error()
				var ke *kit.HTTPError
				if errors.As(MailError(ctx, err), &ke) {
					out[i].Error = ke.Message
				}
				return
			}
			for _, b := range boxes {
				out[i].Mailboxes = append(out[i].Mailboxes, MailboxView{ID: GlobalID(a.ID, b.ID), Name: b.Name, Role: b.Role, Total: b.Total, Unread: b.Unread})
			}
		}()
	}
	wg.Wait()
	return out, nil
}

// RoleBox finds an account's folder with a role.
func (m *MailAccounts) RoleBox(ctx context.Context, userID, id int64, role string) (string, error) {
	boxes, err := m.Boxes(ctx, userID, id, false)
	if err != nil {
		return "", err
	}
	for _, b := range boxes {
		if b.Role == role {
			return b.ID, nil
		}
	}
	return "", kit.Error(http.StatusBadRequest, tr(ctx, "e.noFolder", map[string]any{"folder": tr(ctx, "m."+role)}))
}

func roleOfBox(boxes []mail.Mailbox, id string) string {
	for _, b := range boxes {
		if b.ID == id {
			return b.Role
		}
	}
	return ""
}

// View turns a provider's summary into the browser's.
func View(account int64, s mail.Summary, boxes []mail.Mailbox) SummaryView {
	return SummaryView{
		Summary: s, ID: GlobalID(account, s.ID), ThreadID: GlobalID(account, s.ThreadID),
		MailboxID: GlobalID(account, s.MailboxID), MailboxRole: roleOfBox(boxes, s.MailboxID), AccountID: account,
	}
}

// ListQuery is the message list's query.
type ListQuery struct {
	Account     string `query:"account"`
	Mailbox     string `query:"mailbox"`
	Search      string `query:"q"`
	Unread      bool   `query:"unread"`
	Flagged     bool   `query:"flagged"`
	Attachments bool   `query:"attachments"`
	Week        bool   `query:"week"`
	Label       string `query:"label"`
	Offset      int    `query:"offset"`
	Limit       int    `query:"limit"`
}

// ListResult is a page of the list.
type ListResult struct {
	Messages []SummaryView `json:"messages"`
	Total    int           `json:"total"`
	Errors   []string      `json:"errors"`
}

// Query runs the list's query over one account or all of them, merging
// their newest messages. Mailbox is a folder's global ID, or a role
// ("inbox", "sent", …) or "starred" to span accounts.
func (m *MailAccounts) Query(ctx context.Context, userID int64, in ListQuery) (ListResult, error) {
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 50
	}
	q := mail.Query{Search: in.Search, Unread: in.Unread, Flagged: in.Flagged, HasAttachment: in.Attachments, Keyword: in.Label, Limit: in.Offset + in.Limit}
	if in.Week {
		q.Since = time.Now().AddDate(0, 0, -7)
	}
	type target struct {
		account int64
		box     string
	}
	var targets []target
	if acc, box, ok := SplitID(in.Mailbox); ok {
		targets = append(targets, target{acc, box})
	} else {
		list, err := m.List(ctx, userID)
		if err != nil {
			return ListResult{}, err
		}
		for _, a := range list {
			if in.Account != "" && in.Account != "all" && in.Account != strconv.FormatInt(a.ID, 10) {
				continue
			}
			targets = append(targets, target{a.ID, ""})
		}
	}
	res := ListResult{Messages: []SummaryView{}, Errors: []string{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fail := func(err error) {
				mu.Lock()
				defer mu.Unlock()
				var ke *kit.HTTPError
				if errors.As(MailError(ctx, err), &ke) {
					res.Errors = append(res.Errors, ke.Message)
				}
			}
			p, acc, err := m.Provider(ctx, userID, t.account)
			if err != nil {
				fail(err)
				return
			}
			boxes, err := m.Boxes(ctx, userID, t.account, false)
			if err != nil {
				fail(err)
				return
			}
			tq := q
			switch {
			case t.box != "":
				tq.MailboxID = t.box
			case in.Mailbox == "starred":
				tq.Flagged = true
			case strings.HasPrefix(in.Mailbox, "label:"):
				tq.Keyword = strings.TrimPrefix(in.Mailbox, "label:")
				// Labels span the inbox and the archive, like starred.
				ids := []string{}
				for _, b := range boxes {
					if b.Role == mail.RoleInbox || b.Role == mail.RoleArchive || b.Role == mail.RoleSent {
						ids = append(ids, b.ID)
					}
				}
				var all []mail.Summary
				total := 0
				for _, id := range ids {
					lq := tq
					lq.MailboxID = id
					page, err := p.Query(ctx, lq)
					if err != nil {
						fail(err)
						return
					}
					all = append(all, page.Messages...)
					total += page.Total
				}
				mu.Lock()
				defer mu.Unlock()
				res.Total += total
				for _, s := range all {
					res.Messages = append(res.Messages, View(acc.ID, s, boxes))
				}
				return
			default:
				role := in.Mailbox
				if role == "" {
					role = mail.RoleInbox
				}
				id := ""
				for _, b := range boxes {
					if b.Role == role {
						id = b.ID
					}
				}
				if id == "" {
					return // this account has no such folder
				}
				tq.MailboxID = id
			}
			page, err := p.Query(ctx, tq)
			if err != nil {
				fail(err)
				return
			}
			// Senders become address suggestions.
			var from []mail.Address
			for _, s := range page.Messages {
				from = append(from, s.From...)
			}
			go m.Remember(context.WithoutCancel(ctx), userID, from, 0)
			mu.Lock()
			defer mu.Unlock()
			res.Total += page.Total
			for _, s := range page.Messages {
				res.Messages = append(res.Messages, View(acc.ID, s, boxes))
			}
		}()
	}
	wg.Wait()
	sort.SliceStable(res.Messages, func(i, j int) bool { return res.Messages[i].Date.After(res.Messages[j].Date) })
	if in.Offset < len(res.Messages) {
		res.Messages = res.Messages[in.Offset:]
	} else {
		res.Messages = []SummaryView{}
	}
	if len(res.Messages) > in.Limit {
		res.Messages = res.Messages[:in.Limit]
	}
	return res, nil
}
