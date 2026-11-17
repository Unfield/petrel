package cal

import (
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// Invite is an iTIP message from mail (RFC 5546): an invitation (REQUEST),
// a cancellation (CANCEL), or someone's answer (REPLY).
type Invite struct {
	Method string `json:"method"`
	Event  Event  `json:"event"`
}

// ParseInvite reads a text/calendar part.
func ParseInvite(data []byte) (*Invite, *ical.Calendar, error) {
	c, err := Decode(data)
	if err != nil {
		return nil, nil, err
	}
	inv := &Invite{}
	inv.Method, _ = c.Props.Text(ical.PropMethod)
	inv.Method = strings.ToUpper(inv.Method)
	for _, ch := range c.Children {
		if ch.Name == ical.CompEvent && ch.Props.Get(ical.PropRecurrenceID) == nil {
			inv.Event = read(ch)
			return inv, c, nil
		}
	}
	for _, ch := range c.Children {
		if ch.Name == ical.CompEvent {
			inv.Event = read(ch)
			return inv, c, nil
		}
	}
	return nil, nil, ErrNoCalendar
}

// Answers are the participation statuses a reply can give.
var Answers = map[string]bool{"ACCEPTED": true, "TENTATIVE": true, "DECLINED": true}

// setMe sets the attendee me's answer in an event, adding them if the
// invitation didn't name them (it went to an alias, or a list).
func setMe(ev *ical.Component, me, name, status string) {
	for i, p := range ev.Props[ical.PropAttendee] {
		if strings.EqualFold(person(&p).Email, me) {
			ev.Props[ical.PropAttendee][i].Params.Set(ical.ParamParticipationStatus, status)
			ev.Props[ical.PropAttendee][i].Params.Del("RSVP")
			return
		}
	}
	p := ical.NewProp(ical.PropAttendee)
	p.Value = "mailto:" + me
	p.Params.Set(ical.ParamParticipationStatus, status)
	if name != "" {
		p.Params.Set(ical.ParamCommonName, name)
	}
	ev.Props.Add(p)
}

// Reply is the answer to an invitation, from me, for the organizer.
func Reply(inv *ical.Calendar, me, name, status string) (*ical.Calendar, error) {
	out := ical.NewCalendar()
	out.Props.SetText(ical.PropVersion, "2.0")
	out.Props.SetText(ical.PropProductID, "-//Petrel//Petrel//EN")
	out.Props.SetText(ical.PropMethod, "REPLY")
	for _, ch := range inv.Children {
		if ch.Name != ical.CompEvent {
			continue
		}
		ev := ical.NewComponent(ical.CompEvent)
		for _, name := range []string{ical.PropUID, ical.PropSequence, ical.PropDateTimeStart, ical.PropDateTimeEnd, ical.PropSummary, ical.PropOrganizer, ical.PropRecurrenceID} {
			if p := ch.Props.Get(name); p != nil {
				ev.Props.Set(p)
			}
		}
		ev.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
		setMe(ev, me, name, status)
		// Only me: a reply names its sender alone.
		mine := ev.Props[ical.PropAttendee][:0]
		for _, p := range ev.Props[ical.PropAttendee] {
			if strings.EqualFold(person(&p).Email, me) {
				mine = append(mine, p)
			}
		}
		ev.Props[ical.PropAttendee] = mine
		out.Children = append(out.Children, ev)
	}
	if len(out.Children) == 0 {
		return nil, ErrNoCalendar
	}
	return out, nil
}

// Accepted is an invitation as it goes in my calendar: without the iTIP
// method, with my answer.
func Accepted(inv *ical.Calendar, me, name, status string) *ical.Calendar {
	out := ical.NewCalendar()
	for k, v := range inv.Props {
		if k != ical.PropMethod {
			out.Props[k] = v
		}
	}
	for _, ch := range inv.Children {
		if ch.Name == ical.CompEvent {
			setMe(ch, me, name, status)
		}
		out.Children = append(out.Children, ch)
	}
	return out
}
