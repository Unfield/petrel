// Package cal is Petrel's calendar: CalDAV (RFC 4791) on the mail
// account's server, and iTIP invitations (RFC 5546) that arrive by mail.
package cal

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/teambition/rrule-go"
)

// ErrNoCalendar: the server has no CalDAV calendars Petrel could find.
var ErrNoCalendar = errors.New("cal: no calendars found")

// Calendar is one of an account's calendars.
type Calendar struct {
	ID   string `json:"id"` // its path on the server
	Name string `json:"name"`
}

// Person is an organizer or attendee.
type Person struct {
	Email  string `json:"email"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"` // ACCEPTED, DECLINED, TENTATIVE, NEEDS-ACTION
}

// Event is an event, or one occurrence of a repeating one.
type Event struct {
	// Href is the event's resource on the server; with Start it names an
	// occurrence.
	Href        string    `json:"href"`
	Calendar    string    `json:"calendar"`
	UID         string    `json:"uid"`
	Title       string    `json:"title"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Location    string    `json:"location,omitempty"`
	Description string    `json:"description,omitempty"`
	// Repeat is daily, weekly, monthly or yearly for a simple repeating
	// event, "custom" for any other rule.
	Repeat    string   `json:"repeat,omitempty"`
	Organizer *Person  `json:"organizer,omitempty"`
	Attendees []Person `json:"attendees,omitempty"`
}

// Auth adds credentials to requests.
type Auth func(*http.Request) error

type authed struct {
	c    *http.Client
	auth Auth
}

func (a authed) Do(req *http.Request) (*http.Response, error) {
	if err := a.auth(req); err != nil {
		return nil, err
	}
	return a.c.Do(req)
}

// Client is a connection to an account's calendars.
type Client struct {
	c    *caldav.Client
	home string
}

// Connect finds the calendars at one of the candidate addresses: the
// first that has a calendar home answers.
func Connect(ctx context.Context, hc *http.Client, auth Auth, candidates []string) (*Client, string, error) {
	h := authed{hc, auth}
	var last error = ErrNoCalendar
	for _, base := range candidates {
		c, err := caldav.NewClient(h, base)
		if err != nil {
			last = err
			continue
		}
		wc, _ := webdav.NewClient(h, base)
		principal, err := wc.FindCurrentUserPrincipal(ctx)
		if err != nil {
			last = err
			continue
		}
		home, err := c.FindCalendarHomeSet(ctx, principal)
		if err != nil {
			last = err
			continue
		}
		return &Client{c: c, home: home}, base, nil
	}
	return nil, "", last
}

// Calendars lists the calendars that hold events.
func (c *Client) Calendars(ctx context.Context) ([]Calendar, error) {
	list, err := c.c.FindCalendars(ctx, c.home)
	if err != nil {
		return nil, err
	}
	out := []Calendar{}
	for _, cal := range list {
		events := len(cal.SupportedComponentSet) == 0
		for _, comp := range cal.SupportedComponentSet {
			events = events || comp == ical.CompEvent
		}
		if !events {
			continue
		}
		name := cal.Name
		if name == "" {
			name = strings.Trim(cal.Path[strings.LastIndex(strings.TrimSuffix(cal.Path, "/"), "/")+1:], "/")
		}
		out = append(out, Calendar{ID: cal.Path, Name: name})
	}
	if len(out) == 0 {
		return nil, ErrNoCalendar
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Events lists the events in a calendar between from and to, repeating
// events as their occurrences.
func (c *Client) Events(ctx context.Context, calendar string, from, to time.Time) ([]Event, error) {
	objs, err := c.c.QueryCalendar(ctx, calendar, &caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{Name: ical.CompCalendar, AllProps: true, AllComps: true},
		CompFilter: caldav.CompFilter{Name: ical.CompCalendar, Comps: []caldav.CompFilter{
			{Name: ical.CompEvent, Start: from, End: to},
		}},
	})
	if err != nil {
		return nil, err
	}
	out := []Event{}
	for _, o := range objs {
		out = append(out, expand(o.Data, o.Path, calendar, from, to)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

// read reads one VEVENT.
func read(comp *ical.Component) Event {
	e := Event{}
	e.UID, _ = comp.Props.Text(ical.PropUID)
	e.Title, _ = comp.Props.Text(ical.PropSummary)
	e.Location, _ = comp.Props.Text(ical.PropLocation)
	e.Description, _ = comp.Props.Text(ical.PropDescription)
	ev := &ical.Event{Component: comp}
	e.Start, _ = ev.DateTimeStart(time.Local)
	e.End, _ = ev.DateTimeEnd(time.Local)
	if p := comp.Props.Get(ical.PropDateTimeStart); p != nil && p.ValueType() == ical.ValueDate {
		e.AllDay = true
	}
	if e.End.IsZero() || !e.End.After(e.Start) {
		if e.AllDay {
			e.End = e.Start.AddDate(0, 0, 1)
		} else {
			e.End = e.Start.Add(time.Hour)
		}
	}
	if p := comp.Props.Get(ical.PropRecurrenceRule); p != nil {
		e.Repeat = "custom"
		switch r := strings.ToUpper(p.Value); {
		case r == "FREQ=DAILY":
			e.Repeat = "daily"
		case r == "FREQ=WEEKLY":
			e.Repeat = "weekly"
		case r == "FREQ=MONTHLY":
			e.Repeat = "monthly"
		case r == "FREQ=YEARLY":
			e.Repeat = "yearly"
		}
	}
	if p := comp.Props.Get(ical.PropOrganizer); p != nil {
		e.Organizer = person(p)
	}
	for _, p := range comp.Props[ical.PropAttendee] {
		e.Attendees = append(e.Attendees, *person(&p))
	}
	return e
}

func person(p *ical.Prop) *Person {
	email := p.Value
	if i := strings.Index(strings.ToLower(email), "mailto:"); i >= 0 {
		email = email[i+len("mailto:"):]
	}
	return &Person{Email: strings.ToLower(email), Name: p.Params.Get(ical.ParamCommonName), Status: strings.ToUpper(p.Params.Get(ical.ParamParticipationStatus))}
}

// expand turns a calendar object's events into occurrences in [from, to):
// the series, less the dates its exceptions move, plus the exceptions.
func expand(data *ical.Calendar, href, calendar string, from, to time.Time) []Event {
	var out []Event
	moved := map[int64]bool{}
	var master *ical.Component
	for _, comp := range data.Children {
		if comp.Name != ical.CompEvent {
			continue
		}
		if p := comp.Props.Get(ical.PropRecurrenceID); p != nil {
			if t, err := p.DateTime(time.Local); err == nil {
				moved[t.Unix()] = true
			}
			e := read(comp)
			e.Href, e.Calendar = href, calendar
			if e.End.After(from) && e.Start.Before(to) {
				out = append(out, e)
			}
			continue
		}
		master = comp
	}
	if master == nil {
		return out
	}
	e := read(master)
	e.Href, e.Calendar = href, calendar
	if master.Props.Get(ical.PropRecurrenceRule) == nil {
		if e.End.After(from) && e.Start.Before(to) {
			out = append(out, e)
		}
		return out
	}
	set, err := master.RecurrenceSet(time.Local)
	if err != nil || set == nil {
		return append(out, e)
	}
	length := e.End.Sub(e.Start)
	for i, start := range set.Between(from.Add(-length), to, true) {
		if moved[start.Unix()] || i > 2000 {
			continue
		}
		o := e
		o.Start, o.End = start, start.Add(length)
		out = append(out, o)
	}
	return out
}

func newUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + "@petrel"
}

// Input is an event to save.
type Input struct {
	Href        string    // "" for a new event
	Calendar    string
	Title       string
	Start, End  time.Time
	AllDay      bool
	Location    string
	Description string
	Repeat      string // "", daily, weekly, monthly, yearly; "custom" keeps the rule
}

// Save creates or changes an event. Changing a repeating event changes
// the series, keeping its exceptions and anything Petrel doesn't edit.
func (c *Client) Save(ctx context.Context, in Input) (string, error) {
	var calData *ical.Calendar
	var comp *ical.Component
	href := in.Href
	if href != "" {
		obj, err := c.c.GetCalendarObject(ctx, href)
		if err != nil {
			return "", err
		}
		calData = obj.Data
		for _, ch := range calData.Children {
			if ch.Name == ical.CompEvent && ch.Props.Get(ical.PropRecurrenceID) == nil {
				comp = ch
			}
		}
		if comp == nil {
			return "", errors.New("cal: the event is empty")
		}
	} else {
		calData = ical.NewCalendar()
		calData.Props.SetText(ical.PropVersion, "2.0")
		calData.Props.SetText(ical.PropProductID, "-//Petrel//Petrel//EN")
		comp = ical.NewComponent(ical.CompEvent)
		uid := newUID()
		comp.Props.SetText(ical.PropUID, uid)
		calData.Children = append(calData.Children, comp)
		href = strings.TrimSuffix(in.Calendar, "/") + "/" + url.PathEscape(uid) + ".ics"
	}
	comp.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	comp.Props.SetText(ical.PropSummary, in.Title)
	setOptional(comp, ical.PropLocation, in.Location)
	setOptional(comp, ical.PropDescription, in.Description)
	comp.Props.Del(ical.PropDuration)
	if in.AllDay {
		comp.Props.SetDate(ical.PropDateTimeStart, in.Start)
		end := in.End
		if !end.After(in.Start) {
			end = in.Start.AddDate(0, 0, 1)
		}
		comp.Props.SetDate(ical.PropDateTimeEnd, end)
	} else {
		comp.Props.SetDateTime(ical.PropDateTimeStart, in.Start.UTC())
		comp.Props.SetDateTime(ical.PropDateTimeEnd, in.End.UTC())
	}
	if in.Repeat != "custom" {
		comp.Props.Del(ical.PropRecurrenceRule)
		freq := map[string]rrule.Frequency{"daily": rrule.DAILY, "weekly": rrule.WEEKLY, "monthly": rrule.MONTHLY, "yearly": rrule.YEARLY}
		if f, ok := freq[in.Repeat]; ok {
			comp.Props.SetRecurrenceRule(&rrule.ROption{Freq: f})
		}
	}
	// Each change counts up, so invitees' calendars take the newest.
	seq := 0
	if p := comp.Props.Get(ical.PropSequence); p != nil && in.Href != "" {
		if n, err := p.Int(); err == nil {
			seq = n + 1
		}
	}
	sp := ical.NewProp(ical.PropSequence)
	sp.Value = fmt.Sprint(seq)
	comp.Props.Set(sp)
	if _, err := c.c.PutCalendarObject(ctx, href, calData); err != nil {
		return "", err
	}
	return href, nil
}

func setOptional(comp *ical.Component, name, value string) {
	if value == "" {
		comp.Props.Del(name)
	} else {
		comp.Props.SetText(name, value)
	}
}

// Delete removes an event, all of its occurrences.
func (c *Client) Delete(ctx context.Context, href string) error {
	return c.c.RemoveAll(ctx, href)
}

// PutRaw stores a calendar object as it is, such as an accepted
// invitation.
func (c *Client) PutRaw(ctx context.Context, calendar string, data *ical.Calendar) (string, error) {
	uid := ""
	for _, ch := range data.Children {
		if ch.Name == ical.CompEvent {
			uid, _ = ch.Props.Text(ical.PropUID)
		}
	}
	if uid == "" {
		uid = newUID()
	}
	href := strings.TrimSuffix(calendar, "/") + "/" + url.PathEscape(strings.ReplaceAll(uid, "/", "_")) + ".ics"
	_, err := c.c.PutCalendarObject(ctx, href, data)
	return href, err
}

// Decode reads iCalendar data.
func Decode(data []byte) (*ical.Calendar, error) {
	return ical.NewDecoder(bytes.NewReader(data)).Decode()
}

// Encode writes iCalendar data.
func Encode(c *ical.Calendar) ([]byte, error) {
	var b bytes.Buffer
	err := ical.NewEncoder(&b).Encode(c)
	return b.Bytes(), err
}
