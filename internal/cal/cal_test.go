package cal_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"petrel/internal/cal"
	"petrel/internal/mail"
)

// TestCalDAV runs against a real server (scripts/stalwart-test.sh): set
// PETREL_CALDAV_URL, such as http://127.0.0.1:8080/.well-known/caldav, and
// PETREL_JMAP_USER1 and PETREL_JMAP_PASSWORD.
func TestCalDAV(t *testing.T) {
	base := os.Getenv("PETREL_CALDAV_URL")
	if base == "" {
		t.Skip("PETREL_CALDAV_URL isn't set")
	}
	ctx := context.Background()
	user, pass := os.Getenv("PETREL_JMAP_USER1"), os.Getenv("PETREL_JMAP_PASSWORD")
	auth := func(r *http.Request) error { r.SetBasicAuth(user, pass); return nil }
	hc := mail.HTTPClient(mail.Options{AllowPrivate: true}, 20*time.Second)
	if _, _, err := cal.Connect(ctx, hc, func(r *http.Request) error { r.SetBasicAuth(user, "wrong"); return nil }, []string{base}); err == nil {
		t.Error("connected with a wrong password")
	}
	c, found, err := cal.Connect(ctx, hc, auth, []string{"http://127.0.0.1:1/nothing", base})
	if err != nil || found != base {
		t.Fatalf("connect: %q %v", found, err)
	}
	cals, err := c.Calendars(ctx)
	if err != nil {
		t.Fatalf("calendars: %v", err)
	}
	home := cals[0].ID
	day := time.Now().Truncate(24 * time.Hour).Add(48 * time.Hour)
	stamp := time.Now().Format("150405")

	// A meeting, then a change to it.
	href, err := c.Save(ctx, cal.Input{Calendar: home, Title: "Planning " + stamp, Start: day.Add(10 * time.Hour), End: day.Add(11 * time.Hour), Location: "Room 2"})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := c.Save(ctx, cal.Input{Href: href, Calendar: home, Title: "Planning " + stamp + " (moved)", Start: day.Add(14 * time.Hour), End: day.Add(15 * time.Hour), Description: "Bring the plan"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	// Every week, for the four weeks after.
	weekly, err := c.Save(ctx, cal.Input{Calendar: home, Title: "Standup " + stamp, Start: day.Add(9 * time.Hour), End: day.Add(9*time.Hour + 15*time.Minute), Repeat: "weekly"})
	if err != nil {
		t.Fatalf("save weekly: %v", err)
	}
	// All day.
	allDay, err := c.Save(ctx, cal.Input{Calendar: home, Title: "Holiday " + stamp, Start: day, End: day.AddDate(0, 0, 1), AllDay: true})
	if err != nil {
		t.Fatalf("save all day: %v", err)
	}

	events, err := c.Events(ctx, home, day.Add(-time.Hour), day.AddDate(0, 0, 28))
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var planning, standups, holidays int
	for _, e := range events {
		switch {
		case e.Title == "Planning "+stamp+" (moved)":
			planning++
			if !e.Start.Equal(day.Add(14*time.Hour)) || e.Description != "Bring the plan" || e.Location != "" {
				t.Errorf("moved meeting: %+v", e)
			}
		case e.Title == "Standup "+stamp:
			standups++
			if e.Repeat != "weekly" || e.End.Sub(e.Start) != 15*time.Minute {
				t.Errorf("standup: %+v", e)
			}
		case e.Title == "Holiday "+stamp:
			holidays++
			if !e.AllDay {
				t.Errorf("holiday: %+v", e)
			}
		}
	}
	if planning != 1 || standups != 4 || holidays != 1 {
		t.Errorf("planning %d, standups %d, holidays %d in %d events", planning, standups, holidays, len(events))
	}
	for _, h := range []string{href, weekly, allDay} {
		if err := c.Delete(ctx, h); err != nil {
			t.Errorf("delete: %v", err)
		}
	}
	events, _ = c.Events(ctx, home, day.Add(-time.Hour), day.AddDate(0, 0, 28))
	for _, e := range events {
		if strings.HasSuffix(e.Title, stamp) || strings.Contains(e.Title, stamp+" ") {
			t.Errorf("left after deleting: %+v", e)
		}
	}
}
