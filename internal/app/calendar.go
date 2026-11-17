package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/emersion/go-webdav/caldav"

	"petrel/internal/cal"
	"petrel/internal/mail"
)

// Calendars come from the mail account's provider over CalDAV, signed in
// the same way as its mail.

// CalendarView is one calendar, of one account.
type CalendarView struct {
	ID      string `json:"id"` // global: account~path
	Account int64  `json:"account"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Color   string `json:"color"`
}

// EventView is an event with global IDs.
type EventView struct {
	cal.Event
	ID       string `json:"id"`       // account~href
	Calendar string `json:"calendar"` // account~path
	Account  int64  `json:"account"`
	Color    string `json:"color"`
}

var calColors = []string{"#3b6cf6", "#0f9d8f", "#e8590c", "#7c5cff", "#e64980", "#2f9e44", "#d6336c", "#1c7ed6"}

type calEntry struct {
	client    *cal.Client
	calendars []cal.Calendar
	err       error
	at        time.Time
}

var calCache = struct {
	sync.Mutex
	m map[int64]calEntry
}{m: map[int64]calEntry{}}

// calCandidates are the addresses to look for an account's calendars at,
// most likely first.
func calCandidates(ctx context.Context, s mail.Settings, email string) []string {
	if s.CalDAV != "" {
		return []string{s.CalDAV}
	}
	var out []string
	add := func(u string) {
		for _, x := range out {
			if x == u {
				return
			}
		}
		out = append(out, u)
	}
	if s.JMAPURL != "" {
		if u, err := url.Parse(s.JMAPURL); err == nil {
			add(u.Scheme + "://" + u.Host + "/.well-known/caldav")
		}
	}
	domain := email[strings.LastIndexByte(email, '@')+1:]
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if u, err := caldav.DiscoverContextURL(sctx, domain); err == nil && u != "" {
		add(u)
	}
	cancel()
	add("https://" + domain + "/.well-known/caldav")
	if s.Incoming.Host != "" && !strings.EqualFold(s.Incoming.Host, domain) {
		add("https://" + s.Incoming.Host + "/.well-known/caldav")
	}
	return out
}

// calClient connects to an account's calendars, once every few minutes.
func (m *MailAccounts) calClient(ctx context.Context, userID, account int64) (*cal.Client, []cal.Calendar, error) {
	calCache.Lock()
	e, ok := calCache.m[account]
	calCache.Unlock()
	if ok && time.Since(e.at) < 10*time.Minute {
		return e.client, e.calendars, e.err
	}
	s, err := m.get(ctx, userID, account)
	if err != nil {
		return nil, nil, err
	}
	creds := s.creds
	auth := func(r *http.Request) error {
		if creds.OAuth != nil {
			tok, err := mail.AccessToken(r.Context(), &creds, m.opts)
			if err != nil {
				return err
			}
			r.Header.Set("Authorization", "Bearer "+tok)
			return nil
		}
		r.SetBasicAuth(creds.Username, creds.Password)
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	e = calEntry{at: time.Now()}
	e.client, _, e.err = cal.Connect(cctx, mail.HTTPClient(m.opts, 30*time.Second), auth, calCandidates(cctx, s.settings, s.Email))
	if e.err == nil {
		e.calendars, e.err = e.client.Calendars(cctx)
	}
	if e.err != nil {
		e.client = nil
		// Ask again sooner when it failed: the server may come back.
		e.at = time.Now().Add(-8 * time.Minute)
	}
	calCache.Lock()
	calCache.m[account] = e
	calCache.Unlock()
	return e.client, e.calendars, e.err
}

func forgetCalendars(account int64) {
	calCache.Lock()
	delete(calCache.m, account)
	calCache.Unlock()
}

// Calendars lists the calendars of all the user's accounts that have them.
func (m *MailAccounts) Calendars(ctx context.Context, userID int64) ([]CalendarView, error) {
	list, err := m.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([][]CalendarView, len(list))
	var wg sync.WaitGroup
	for i, a := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, cals, err := m.calClient(ctx, userID, a.ID)
			if err != nil {
				return
			}
			for j, c := range cals {
				out[i] = append(out[i], CalendarView{ID: GlobalID(a.ID, c.ID), Account: a.ID, Email: a.Email, Name: c.Name, Color: calColors[(i*3+j)%len(calColors)]})
			}
		}()
	}
	wg.Wait()
	all := []CalendarView{}
	for _, l := range out {
		all = append(all, l...)
	}
	return all, nil
}

// Events lists the events between from and to in the given calendars, or
// in all of them.
func (m *MailAccounts) Events(ctx context.Context, userID int64, from, to time.Time, only []string) ([]EventView, error) {
	if to.Sub(from) > 400*24*time.Hour || !to.After(from) {
		return nil, kit.Error(http.StatusBadRequest, tr(ctx, "e.calRange"))
	}
	cals, err := m.Calendars(ctx, userID)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range only {
		want[id] = true
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	out := []EventView{}
	for _, c := range cals {
		if len(want) > 0 && !want[c.ID] {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, _, err := m.calClient(ctx, userID, c.Account)
			if err != nil {
				return
			}
			_, path, _ := SplitID(c.ID)
			events, err := client.Events(ctx, path, from, to)
			if err != nil {
				return
			}
			mu.Lock()
			for _, e := range events {
				out = append(out, EventView{Event: e, ID: GlobalID(c.Account, e.Href), Calendar: c.ID, Account: c.Account, Color: c.Color})
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

// EventInput is an event to save, with global IDs.
type EventInput struct {
	ID          string    `json:"id"` // "" for a new event
	Calendar    string    `json:"calendar" validate:"required"`
	Title       string    `json:"title" validate:"max=500"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Location    string    `json:"location" validate:"max=500"`
	Description string    `json:"description" validate:"max=10000"`
	Repeat      string    `json:"repeat" validate:"omitempty,oneof=daily|weekly|monthly|yearly|custom"`
}

func calError(ctx context.Context, err error) error {
	var ke *kit.HTTPError
	if errors.As(err, &ke) {
		return err
	}
	var ne net.Error
	if errors.As(err, &ne) || errors.Is(err, mail.ErrBlocked) {
		return kit.Error(http.StatusBadGateway, tr(ctx, "e.calUnreachable"))
	}
	return kit.Error(http.StatusBadGateway, tr(ctx, "e.calServer", map[string]any{"error": err.Error()}))
}

// SaveEvent creates or changes an event.
func (m *MailAccounts) SaveEvent(ctx context.Context, userID int64, in EventInput) (string, error) {
	account, path, ok := SplitID(in.Calendar)
	if !ok {
		return "", kit.NotFound()
	}
	href := ""
	if in.ID != "" {
		a, h, ok := SplitID(in.ID)
		if !ok || a != account {
			return "", kit.Error(http.StatusBadRequest, tr(ctx, "e.crossAccount"))
		}
		href = h
	}
	if in.Start.IsZero() || in.End.Before(in.Start) {
		return "", kit.Error(http.StatusBadRequest, tr(ctx, "e.calTimes"))
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = tr(ctx, "cal.untitled")
	}
	client, _, err := m.calClient(ctx, userID, account)
	if err != nil {
		return "", calError(ctx, err)
	}
	h, err := client.Save(ctx, cal.Input{Href: href, Calendar: path, Title: in.Title, Start: in.Start, End: in.End, AllDay: in.AllDay,
		Location: in.Location, Description: in.Description, Repeat: in.Repeat})
	if err != nil {
		return "", calError(ctx, err)
	}
	return GlobalID(account, h), nil
}

// DeleteEvent deletes an event, every occurrence of it.
func (m *MailAccounts) DeleteEvent(ctx context.Context, userID int64, id string) error {
	account, href, ok := SplitID(id)
	if !ok {
		return kit.NotFound()
	}
	client, _, err := m.calClient(ctx, userID, account)
	if err != nil {
		return calError(ctx, err)
	}
	if err := client.Delete(ctx, href); err != nil {
		return calError(ctx, err)
	}
	return nil
}

// InviteView is an invitation in a message.
type InviteView struct {
	cal.Invite
	Part string `json:"part"`
	// Status is my answer so far, from the invitation.
	Status string `json:"status,omitempty"`
}

// invite finds an invitation among a message's attachments.
func (m *MailAccounts) invite(ctx context.Context, p mail.Provider, id string, atts []mail.Attachment, me []string) *InviteView {
	for _, a := range atts {
		t := strings.ToLower(a.Type)
		if t != "text/calendar" && t != "application/ics" && !strings.HasSuffix(strings.ToLower(a.Name), ".ics") {
			continue
		}
		_, body, err := p.Attachment(ctx, id, a.ID)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(body, 1<<20))
		body.Close()
		inv, _, err := cal.ParseInvite(data)
		if err != nil {
			continue
		}
		v := &InviteView{Invite: *inv, Part: a.ID}
		for _, at := range inv.Event.Attendees {
			for _, e := range me {
				if strings.EqualFold(at.Email, e) {
					v.Status = at.Status
				}
			}
		}
		return v
	}
	return nil
}

// RespondInvite answers an invitation: by mail to the organizer, and in
// the first calendar of the account it came to, unless declined.
func (m *MailAccounts) RespondInvite(ctx context.Context, userID int64, message, part, status string) error {
	status = strings.ToUpper(status)
	if !cal.Answers[status] {
		return kit.Error(http.StatusBadRequest, tr(ctx, "e.calAnswer"))
	}
	account, id, ok := SplitID(message)
	if !ok {
		return kit.NotFound()
	}
	p, acc, err := m.Provider(ctx, userID, account)
	if err != nil {
		return MailError(ctx, err)
	}
	_, body, err := p.Attachment(ctx, id, part)
	if err != nil {
		return MailError(ctx, err)
	}
	data, _ := io.ReadAll(io.LimitReader(body, 1<<20))
	body.Close()
	inv, raw, err := cal.ParseInvite(data)
	if err != nil || inv.Method != "REQUEST" || inv.Event.Organizer == nil {
		return kit.Error(http.StatusBadRequest, tr(ctx, "e.calNotInvite"))
	}
	// Answer as whichever of my addresses was invited.
	me, name := acc.Email, acc.DisplayName
	if ids, err := m.Identities(ctx, userID); err == nil {
		for _, at := range inv.Event.Attendees {
			for _, i := range ids {
				if i.AccountID == account && strings.EqualFold(i.Email, at.Email) {
					me, name = i.Email, i.Name
				}
			}
		}
	}
	reply, err := cal.Reply(raw, me, name, status)
	if err != nil {
		return kit.Error(http.StatusBadRequest, tr(ctx, "e.calNotInvite"))
	}
	ics, err := cal.Encode(reply)
	if err != nil {
		return err
	}
	word := map[string]string{"ACCEPTED": "cal.accepted", "TENTATIVE": "cal.tentative", "DECLINED": "cal.declined"}[status]
	subject := tr(ctx, word) + ": " + inv.Event.Title
	text := tr(ctx, word+"Text", map[string]any{"name": firstNonEmpty(name, me), "title": inv.Event.Title})
	err = p.Send(ctx, mail.Outgoing{From: mail.Address{Name: name, Email: me}, To: []mail.Address{{Name: inv.Event.Organizer.Name, Email: inv.Event.Organizer.Email}},
		Subject: subject, Text: text, Calendar: ics, CalendarMethod: "REPLY"})
	if err != nil {
		return MailError(ctx, err)
	}
	if status == "DECLINED" {
		return nil
	}
	client, cals, err := m.calClient(ctx, userID, account)
	if err != nil || len(cals) == 0 {
		return nil // answered; there's no calendar to put it in
	}
	if _, err := client.PutRaw(ctx, cals[0].ID, cal.Accepted(raw, me, name, status)); err != nil {
		return calError(ctx, err)
	}
	return nil
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
