package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// imapProvider keeps one connection per account, used by one request at a
// time, and opens it again when the server drops it.
type imapProvider struct {
	s Settings
	c Credentials
	o Options

	mu       sync.Mutex
	cl       *imapclient.Client
	selected string
	validity map[string]uint32
}

func newIMAP(s Settings, c Credentials, o Options) *imapProvider {
	return &imapProvider{s: s, c: c, o: o, validity: map[string]uint32{}}
}

func (p *imapProvider) dialLogin(ctx context.Context, opts *imapclient.Options) (*imapclient.Client, error) {
	conn, err := dial(ctx, p.s.Incoming, p.o.AllowPrivate, p.o)
	if err != nil {
		return nil, fmt.Errorf("mail: connecting to %s: %w", p.s.Incoming.Host, err)
	}
	if opts == nil {
		opts = &imapclient.Options{}
	}
	opts.TLSConfig = tlsConfig(p.s.Incoming.Host, p.o)
	var cl *imapclient.Client
	if p.s.Incoming.Security == StartTLS {
		if cl, err = imapclient.NewStartTLS(conn, opts); err != nil {
			conn.Close()
			return nil, err
		}
	} else {
		cl = imapclient.New(conn, opts)
	}
	if err := cl.WaitGreeting(); err != nil {
		cl.Close()
		return nil, err
	}
	if p.c.OAuth != nil {
		var tok string
		if tok, err = token(ctx, &p.c, p.o); err == nil {
			err = cl.Authenticate(xoauth2{user: p.c.Username, token: tok})
		}
	} else {
		err = cl.Login(p.c.Username, p.c.Password).Wait()
	}
	if err != nil {
		cl.Close()
		var ie *imap.Error
		if errors.As(err, &ie) {
			return nil, ErrAuth
		}
		return nil, err
	}
	return cl, nil
}

// do runs fn on the account's connection, connecting first if needed and
// once more if the connection turns out to be gone.
func (p *imapProvider) do(ctx context.Context, fn func(cl *imapclient.Client) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for attempt := 0; ; attempt++ {
		if p.cl != nil {
			select {
			case <-p.cl.Closed():
				p.cl = nil
			default:
			}
		}
		if p.cl == nil {
			cl, err := p.dialLogin(ctx, nil)
			if err != nil {
				return err
			}
			p.cl, p.selected = cl, ""
		}
		err := fn(p.cl)
		if err == nil || attempt > 0 {
			return err
		}
		select {
		case <-p.cl.Closed():
			p.cl = nil // dropped: try again on a new connection
		default:
			return err
		}
	}
}

func (p *imapProvider) selectBox(cl *imapclient.Client, name string) error {
	if p.selected == name {
		return nil
	}
	data, err := cl.Select(name, nil).Wait()
	if err != nil {
		return err
	}
	p.selected = name
	p.validity[name] = data.UIDValidity
	return nil
}

func encBox(name string) string { return base64.RawURLEncoding.EncodeToString([]byte(name)) }
func decBox(id string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	return string(b), err
}

func encID(box string, validity uint32, uid imap.UID) string {
	return encBox(box) + "." + strconv.FormatUint(uint64(validity), 10) + "." + strconv.FormatUint(uint64(uid), 10)
}

func decID(id string) (box string, validity uint32, uid imap.UID, err error) {
	parts := strings.Split(id, ".")
	if len(parts) != 3 {
		return "", 0, 0, ErrNotFound
	}
	box, err = decBox(parts[0])
	v, err2 := strconv.ParseUint(parts[1], 10, 32)
	u, err3 := strconv.ParseUint(parts[2], 10, 32)
	if err != nil || err2 != nil || err3 != nil {
		return "", 0, 0, ErrNotFound
	}
	return box, uint32(v), imap.UID(u), nil
}

var roleNames = map[string][]string{
	RoleSent:    {"sent", "sent items", "sent messages", "sent mail", "gesendet", "gesendete objekte", "gesendete elemente"},
	RoleDrafts:  {"drafts", "draft", "entwürfe", "entwurf"},
	RoleTrash:   {"trash", "deleted", "deleted items", "deleted messages", "bin", "papierkorb", "gelöschte elemente", "gelöscht"},
	RoleJunk:    {"junk", "spam", "junk e-mail", "junk email", "bulk mail"},
	RoleArchive: {"archive", "archives", "archiv"},
}

var roleOrder = map[string]int{RoleInbox: 0, RoleDrafts: 1, RoleSent: 2, RoleArchive: 3, RoleJunk: 4, RoleTrash: 5}

func roleOf(d *imap.ListData) string {
	if strings.EqualFold(d.Mailbox, "INBOX") {
		return RoleInbox
	}
	for _, a := range d.Attrs {
		switch a {
		case imap.MailboxAttrSent:
			return RoleSent
		case imap.MailboxAttrDrafts:
			return RoleDrafts
		case imap.MailboxAttrTrash:
			return RoleTrash
		case imap.MailboxAttrJunk:
			return RoleJunk
		case imap.MailboxAttrArchive:
			return RoleArchive
		case imap.MailboxAttrAll, imap.MailboxAttrFlagged:
			return "virtual"
		}
	}
	leaf := strings.ToLower(leafName(d))
	for role, names := range roleNames {
		if slices.Contains(names, leaf) {
			return role
		}
	}
	return ""
}

func leafName(d *imap.ListData) string {
	if d.Delim != 0 {
		if i := strings.LastIndex(d.Mailbox, string(d.Delim)); i >= 0 {
			return d.Mailbox[i+1:]
		}
	}
	return d.Mailbox
}

func (p *imapProvider) listBoxes(cl *imapclient.Client) ([]Mailbox, map[string]string, error) {
	caps := cl.Caps()
	opts := &imap.ListOptions{}
	rev2 := caps.Has(imap.CapIMAP4rev2)
	status := &imap.StatusOptions{NumMessages: true, NumUnseen: true}
	if rev2 || caps.Has(imap.CapListStatus) {
		opts.ReturnStatus = status
	}
	if rev2 || caps.Has(imap.CapSpecialUse) {
		opts.ReturnSpecialUse = true
	}
	list, err := cl.List("", "*", opts).Collect()
	if err != nil {
		return nil, nil, err
	}
	var boxes []Mailbox
	byRole := map[string]string{}
	for _, d := range list {
		if slices.Contains(d.Attrs, imap.MailboxAttrNoSelect) || slices.Contains(d.Attrs, imap.MailboxAttrNonExistent) {
			continue
		}
		role := roleOf(d)
		if role == "virtual" {
			continue
		}
		if role != "" {
			if _, dup := byRole[role]; dup {
				role = ""
			} else {
				byRole[role] = d.Mailbox
			}
		}
		st := d.Status
		if st == nil {
			if st, err = cl.Status(d.Mailbox, status).Wait(); err != nil {
				st = &imap.StatusData{}
			}
		}
		b := Mailbox{ID: encBox(d.Mailbox), Name: leafName(d), Role: role, Path: d.Mailbox}
		if role == RoleInbox {
			b.Name = "Inbox"
		}
		if st.NumMessages != nil {
			b.Total = int(*st.NumMessages)
		}
		if st.NumUnseen != nil {
			b.Unread = int(*st.NumUnseen)
		}
		boxes = append(boxes, b)
	}
	sort.SliceStable(boxes, func(i, j int) bool {
		oi, iok := roleOrder[boxes[i].Role]
		oj, jok := roleOrder[boxes[j].Role]
		if iok != jok {
			return iok
		}
		if iok {
			return oi < oj
		}
		return strings.ToLower(boxes[i].Name) < strings.ToLower(boxes[j].Name)
	})
	return boxes, byRole, nil
}

func (p *imapProvider) Mailboxes(ctx context.Context) (boxes []Mailbox, err error) {
	err = p.do(ctx, func(cl *imapclient.Client) error {
		boxes, _, err = p.listBoxes(cl)
		return err
	})
	return boxes, err
}

var preSection = &imap.FetchItemBodySection{Peek: true, Partial: &imap.SectionPartial{Offset: 0, Size: 8192}}

func (p *imapProvider) Query(ctx context.Context, q Query) (Page, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	var page Page
	err := p.do(ctx, func(cl *imapclient.Client) error {
		var names []string
		if q.MailboxID == "" {
			_, byRole, err := p.listBoxes(cl)
			if err != nil {
				return err
			}
			for _, r := range []string{RoleInbox, RoleArchive, RoleSent} {
				if n, ok := byRole[r]; ok {
					names = append(names, n)
				}
			}
		} else {
			n, err := decBox(q.MailboxID)
			if err != nil {
				return ErrNotFound
			}
			names = []string{n}
		}
		var all []Summary
		for _, name := range names {
			list, total, err := p.queryBox(cl, name, q, q.Offset+q.Limit)
			if err != nil {
				return err
			}
			all = append(all, list...)
			page.Total += total
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].Date.After(all[j].Date) })
		if q.Offset < len(all) {
			all = all[q.Offset:]
		} else {
			all = nil
		}
		if len(all) > q.Limit {
			all = all[:q.Limit]
		}
		page.Messages = all
		return nil
	})
	if page.Messages == nil {
		page.Messages = []Summary{}
	}
	return page, err
}

// queryBox returns the newest n messages of a mailbox that match q.
func (p *imapProvider) queryBox(cl *imapclient.Client, name string, q Query, n int) ([]Summary, int, error) {
	if err := p.selectBox(cl, name); err != nil {
		return nil, 0, err
	}
	crit := &imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagDeleted}}
	if q.Unread {
		crit.NotFlag = append(crit.NotFlag, imap.FlagSeen)
	}
	if q.Flagged {
		crit.Flag = append(crit.Flag, imap.FlagFlagged)
	}
	if q.Keyword != "" {
		crit.Flag = append(crit.Flag, imap.Flag(q.Keyword))
	}
	if !q.Since.IsZero() {
		crit.Since = q.Since
	}
	for _, w := range strings.Fields(q.Search) {
		crit.Text = append(crit.Text, w)
	}
	data, err := cl.UIDSearch(crit, nil).Wait()
	if err != nil {
		return nil, 0, err
	}
	uids := data.AllUIDs()
	slices.SortFunc(uids, func(a, b imap.UID) int { return int(b) - int(a) })
	total := len(uids)
	if len(uids) > n {
		uids = uids[:n]
	}
	if len(uids) == 0 {
		return nil, total, nil
	}
	msgs, err := cl.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID: true, Flags: true, Envelope: true, InternalDate: true,
		BodySection: []*imap.FetchItemBodySection{preSection},
	}).Collect()
	if err != nil {
		return nil, 0, err
	}
	var out []Summary
	for _, m := range msgs {
		var raw []byte
		if len(m.BodySection) > 0 {
			raw = m.BodySection[0].Bytes
		}
		pr, _ := parse(bytes.NewReader(raw), false)
		out = append(out, p.summary(name, m, pr))
	}
	if q.HasAttachment {
		out = slices.DeleteFunc(out, func(s Summary) bool { return !s.HasAttachment })
	}
	return out, total, nil
}

func envAddrs(list []imap.Address) []Address {
	out := make([]Address, 0, len(list))
	for _, a := range list {
		if e := a.Addr(); e != "" {
			out = append(out, Address{Name: a.Name, Email: e})
		}
	}
	return out
}

func (p *imapProvider) summary(box string, m *imapclient.FetchMessageBuffer, pr *parsed) Summary {
	s := Summary{
		ID: encID(box, p.validity[box], m.UID), MailboxID: encBox(box),
		From: []Address{}, To: []Address{}, Cc: []Address{}, Date: m.InternalDate,
	}
	var refs []string
	inReplyTo := ""
	if e := m.Envelope; e != nil {
		s.Subject, s.MessageID = e.Subject, e.MessageID
		s.From, s.To, s.Cc = envAddrs(e.From), envAddrs(e.To), envAddrs(e.Cc)
		if !e.Date.IsZero() {
			s.Date = e.Date
		}
		if len(e.InReplyTo) > 0 {
			inReplyTo = e.InReplyTo[0]
		}
	}
	if pr != nil {
		refs = msgIDs(pr.header, "References")
		s.Preview = preview(pr.text, pr.html, 200)
		s.HasAttachment = hasAttachments(pr.header)
	}
	s.ThreadID = threadOf(s.MessageID, inReplyTo, refs)
	if s.ThreadID == "" {
		s.ThreadID = s.ID
	}
	s.Keywords = []string{}
	for _, f := range m.Flags {
		switch f {
		case imap.FlagSeen:
			s.Seen = true
		case imap.FlagFlagged:
			s.Flagged = true
		case imap.FlagAnswered:
			s.Answered = true
		default:
			if k := string(f); !strings.HasPrefix(k, "\\") && !strings.HasPrefix(k, "$") {
				s.Keywords = append(s.Keywords, strings.ToLower(k))
			}
		}
	}
	return s
}

func (p *imapProvider) fetchRaw(cl *imapclient.Client, id string) (string, *imapclient.FetchMessageBuffer, []byte, error) {
	box, validity, uid, err := decID(id)
	if err != nil {
		return "", nil, nil, err
	}
	if err := p.selectBox(cl, box); err != nil {
		return "", nil, nil, err
	}
	if p.validity[box] != validity {
		return "", nil, nil, ErrNotFound
	}
	msgs, err := cl.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{
		UID: true, Flags: true, Envelope: true, InternalDate: true,
		BodySection: []*imap.FetchItemBodySection{{Peek: true}},
	}).Collect()
	if err != nil {
		return "", nil, nil, err
	}
	if len(msgs) == 0 || len(msgs[0].BodySection) == 0 {
		return "", nil, nil, ErrNotFound
	}
	return box, msgs[0], msgs[0].BodySection[0].Bytes, nil
}

func (p *imapProvider) Get(ctx context.Context, id string) (*Message, error) {
	var msg *Message
	err := p.do(ctx, func(cl *imapclient.Client) error {
		box, buf, raw, err := p.fetchRaw(cl, id)
		if err != nil {
			return err
		}
		pr, _ := parse(bytes.NewReader(raw), false)
		msg = &Message{Summary: p.summary(box, buf, pr)}
		msg.HTML, msg.Text, msg.Attachments = pr.html, pr.text, pr.attachments
		msg.ReplyTo, msg.Bcc = addresses(pr.header, "Reply-To"), addresses(pr.header, "Bcc")
		msg.References = msgIDs(pr.header, "References")
		if ids := msgIDs(pr.header, "In-Reply-To"); len(ids) > 0 {
			msg.InReplyTo = ids[0]
		}
		msg.Unsubscribe = unsubscribeOf(pr.header)
		if msg.Attachments == nil {
			msg.Attachments = []Attachment{}
		}
		return nil
	})
	return msg, err
}

func (p *imapProvider) Attachment(ctx context.Context, id, attID string) (Attachment, io.ReadCloser, error) {
	var part *partRef
	err := p.do(ctx, func(cl *imapclient.Client) error {
		_, _, raw, err := p.fetchRaw(cl, id)
		if err != nil {
			return err
		}
		pr, _ := parse(bytes.NewReader(raw), true)
		part = findPart(pr, attID)
		if part == nil {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return Attachment{}, nil, err
	}
	return part.att, io.NopCloser(bytes.NewReader(part.body)), nil
}

// findPart finds an attachment by its ID, or an inline image by "cid:…".
func findPart(pr *parsed, id string) *partRef {
	if cid, ok := strings.CutPrefix(id, "cid:"); ok {
		for _, part := range pr.parts {
			if part.att.CID == cid {
				return part
			}
		}
		return nil
	}
	return pr.parts[id]
}

// byBox groups message IDs by mailbox, skipping any whose mailbox was
// recreated since (another UIDVALIDITY).
func (p *imapProvider) byBox(ids []string) map[string][]imap.UID {
	out := map[string][]imap.UID{}
	for _, id := range ids {
		box, _, uid, err := decID(id)
		if err == nil {
			out[box] = append(out[box], uid)
		}
	}
	return out
}

func (p *imapProvider) SetFlags(ctx context.Context, ids []string, seen, flagged *bool) error {
	return p.do(ctx, func(cl *imapclient.Client) error {
		for box, uids := range p.byBox(ids) {
			if err := p.selectBox(cl, box); err != nil {
				return err
			}
			set := imap.UIDSetNum(uids...)
			for _, change := range []struct {
				on   *bool
				flag imap.Flag
			}{{seen, imap.FlagSeen}, {flagged, imap.FlagFlagged}} {
				if change.on == nil {
					continue
				}
				op := imap.StoreFlagsDel
				if *change.on {
					op = imap.StoreFlagsAdd
				}
				if err := cl.Store(set, &imap.StoreFlags{Op: op, Silent: true, Flags: []imap.Flag{change.flag}}, nil).Close(); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (p *imapProvider) expunge(cl *imapclient.Client, set imap.UIDSet) error {
	if err := cl.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close(); err != nil {
		return err
	}
	if cl.Caps().Has(imap.CapUIDPlus) || cl.Caps().Has(imap.CapIMAP4rev2) {
		return cl.UIDExpunge(set).Close()
	}
	return cl.Expunge().Close()
}

func (p *imapProvider) Move(ctx context.Context, ids []string, mailboxID string) (map[string]string, error) {
	dest, err := decBox(mailboxID)
	if err != nil {
		return nil, ErrNotFound
	}
	moved := map[string]string{}
	// record maps old UIDs to new ones, from the server's COPYUID answer.
	record := func(box string, validity uint32, src, dst imap.NumSet) {
		s, ok1 := src.(imap.UIDSet)
		d, ok2 := dst.(imap.UIDSet)
		if !ok1 || !ok2 {
			return
		}
		from, ok1 := s.Nums()
		to, ok2 := d.Nums()
		if !ok1 || !ok2 || len(from) != len(to) {
			return
		}
		for i := range from {
			moved[encID(box, p.validity[box], from[i])] = encID(dest, validity, to[i])
		}
	}
	err = p.do(ctx, func(cl *imapclient.Client) error {
		for box, uids := range p.byBox(ids) {
			if box == dest {
				continue
			}
			if err := p.selectBox(cl, box); err != nil {
				return err
			}
			set := imap.UIDSetNum(uids...)
			if cl.Caps().Has(imap.CapMove) || cl.Caps().Has(imap.CapIMAP4rev2) {
				data, err := cl.Move(set, dest).Wait()
				if err != nil {
					return err
				}
				if data != nil {
					record(box, data.UIDValidity, data.SourceUIDs, data.DestUIDs)
				}
				continue
			}
			data, err := cl.Copy(set, dest).Wait()
			if err != nil {
				return err
			}
			if data != nil {
				record(box, data.UIDValidity, data.SourceUIDs, data.DestUIDs)
			}
			if err := p.expunge(cl, set); err != nil {
				return err
			}
		}
		return nil
	})
	return moved, err
}

func (p *imapProvider) Destroy(ctx context.Context, ids []string) error {
	return p.do(ctx, func(cl *imapclient.Client) error {
		for box, uids := range p.byBox(ids) {
			if err := p.selectBox(cl, box); err != nil {
				return err
			}
			if err := p.expunge(cl, imap.UIDSetNum(uids...)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *imapProvider) Send(ctx context.Context, m Outgoing) error {
	raw, err := sendSMTP(ctx, p.s.SMTP, &p.c, p.o, m)
	if err != nil {
		return err
	}
	if savesSent(p.s.SMTP.Host) {
		return nil
	}
	// File a copy in Sent. The message is already on its way, so a failure
	// here isn't the user's error to see.
	_ = p.do(ctx, func(cl *imapclient.Client) error {
		_, byRole, err := p.listBoxes(cl)
		if err != nil {
			return err
		}
		sent, ok := byRole[RoleSent]
		if !ok {
			return nil
		}
		cmd := cl.Append(sent, int64(len(raw)), &imap.AppendOptions{Flags: []imap.Flag{imap.FlagSeen}, Time: time.Now()})
		if _, err := cmd.Write(raw); err != nil {
			return err
		}
		if err := cmd.Close(); err != nil {
			return err
		}
		_, err = cmd.Wait()
		return err
	})
	return nil
}

func (p *imapProvider) SaveDraft(ctx context.Context, m Outgoing, replace string) (string, error) {
	raw, _, err := build(m, domainOf(m.From.Email))
	if err != nil {
		return "", err
	}
	var id string
	err = p.do(ctx, func(cl *imapclient.Client) error {
		_, byRole, err := p.listBoxes(cl)
		if err != nil {
			return err
		}
		drafts, ok := byRole[RoleDrafts]
		if !ok {
			drafts = "Drafts"
			if err := cl.Create(drafts, nil).Wait(); err != nil {
				return err
			}
		}
		cmd := cl.Append(drafts, int64(len(raw)), &imap.AppendOptions{Flags: []imap.Flag{imap.FlagDraft, imap.FlagSeen}, Time: time.Now()})
		if _, err := cmd.Write(raw); err != nil {
			return err
		}
		if err := cmd.Close(); err != nil {
			return err
		}
		data, err := cmd.Wait()
		if err != nil {
			return err
		}
		if data != nil && data.UID != 0 {
			id = encID(drafts, data.UIDValidity, data.UID)
		}
		if replace != "" {
			if box, _, uid, err := decID(replace); err == nil {
				if err := p.selectBox(cl, box); err == nil {
					_ = p.expunge(cl, imap.UIDSetNum(uid))
				}
			}
		}
		return nil
	})
	return id, err
}

func (p *imapProvider) SetKeywords(ctx context.Context, ids []string, add, remove []string) error {
	return p.do(ctx, func(cl *imapclient.Client) error {
		for box, uids := range p.byBox(ids) {
			if err := p.selectBox(cl, box); err != nil {
				return err
			}
			for _, change := range []struct {
				op    imap.StoreFlagsOp
				words []string
			}{{imap.StoreFlagsAdd, add}, {imap.StoreFlagsDel, remove}} {
				if len(change.words) == 0 {
					continue
				}
				flags := make([]imap.Flag, 0, len(change.words))
				for _, w := range change.words {
					if w == "$answered" { // JMAP's name for a system flag
						flags = append(flags, imap.FlagAnswered)
						continue
					}
					flags = append(flags, imap.Flag(w))
				}
				if err := cl.Store(imap.UIDSetNum(uids...), &imap.StoreFlags{Op: change.op, Silent: true, Flags: flags}, nil).Close(); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (p *imapProvider) EnsureMailbox(ctx context.Context, name string) (string, error) {
	var id string
	err := p.do(ctx, func(cl *imapclient.Client) error {
		list, err := cl.List("", "*", nil).Collect()
		if err != nil {
			return err
		}
		for _, d := range list {
			if strings.EqualFold(d.Mailbox, name) || strings.EqualFold(leafName(d), name) {
				id = encBox(d.Mailbox)
				return nil
			}
		}
		if err := cl.Create(name, nil).Wait(); err != nil {
			return err
		}
		id = encBox(name)
		return nil
	})
	return id, err
}

func (p *imapProvider) State(ctx context.Context) (state string, err error) {
	err = p.do(ctx, func(cl *imapclient.Client) error {
		st, err := cl.Status("INBOX", &imap.StatusOptions{NumMessages: true, NumUnseen: true, UIDNext: true}).Wait()
		if err != nil {
			return err
		}
		state = fmt.Sprint(st.UIDNext, deref(st.NumMessages), deref(st.NumUnseen))
		return nil
	})
	return state, err
}

func deref(n *uint32) uint32 {
	if n == nil {
		return 0
	}
	return *n
}

// Watch holds a second connection in IDLE on the inbox, so new mail shows
// up within moments, and reconnects with a growing pause when it drops.
func (p *imapProvider) Watch(ctx context.Context, changed func()) error {
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		notify := make(chan struct{}, 1)
		poke := func() {
			select {
			case notify <- struct{}{}:
			default:
			}
		}
		cl, err := p.dialLogin(ctx, &imapclient.Options{UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(d *imapclient.UnilateralDataMailbox) { poke() },
			Expunge: func(uint32) { poke() },
		}})
		if err == nil && !cl.Caps().Has(imap.CapIdle) && !cl.Caps().Has(imap.CapIMAP4rev2) {
			cl.Close()
			return ErrUnsupported
		}
		if err == nil {
			_, err = cl.Select("INBOX", nil).Wait()
		}
		if err != nil {
			if errors.Is(err, ErrAuth) {
				return err
			}
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 5*time.Minute)
			continue
		}
		backoff = 5 * time.Second
		p.idle(ctx, cl, notify, changed)
		cl.Close()
	}
	return nil
}

func (p *imapProvider) idle(ctx context.Context, cl *imapclient.Client, notify chan struct{}, changed func()) {
	for {
		idle, err := cl.Idle()
		if err != nil {
			return
		}
		select {
		case <-ctx.Done():
			_ = idle.Close()
			_ = cl.Logout().Wait()
			return
		case <-cl.Closed():
			return
		case <-time.After(25 * time.Minute): // servers end IDLE after 30
			if idle.Close() != nil || idle.Wait() != nil {
				return
			}
		case <-notify:
			if idle.Close() != nil || idle.Wait() != nil {
				return
			}
			changed()
		}
	}
}

func (p *imapProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cl != nil {
		_ = p.cl.Logout().Wait()
		p.cl.Close()
		p.cl = nil
	}
	return nil
}
