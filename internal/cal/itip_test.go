package cal

import (
	"strings"
	"testing"
)

const invite = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//EN\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:abc-123@example.com\r\nDTSTAMP:20261117T100000Z\r\nDTSTART:20261120T140000Z\r\nDTEND:20261120T150000Z\r\nSEQUENCE:2\r\nSUMMARY:Design review\r\nLOCATION:Room 4\r\nORGANIZER;CN=Lena:mailto:lena@example.com\r\nATTENDEE;CN=Sam;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:sam@example.com\r\nATTENDEE;CN=Max;PARTSTAT=ACCEPTED:mailto:max@example.com\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func TestInvite(t *testing.T) {
	inv, raw, err := ParseInvite([]byte(invite))
	if err != nil {
		t.Fatal(err)
	}
	if inv.Method != "REQUEST" || inv.Event.Title != "Design review" || inv.Event.Organizer.Email != "lena@example.com" || len(inv.Event.Attendees) != 2 || inv.Event.Attendees[0].Status != "NEEDS-ACTION" {
		t.Fatalf("invite: %+v", inv)
	}
	reply, err := Reply(raw, "sam@example.com", "Sam", "ACCEPTED")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Encode(reply)
	s := string(b)
	for _, want := range []string{"METHOD:REPLY", "UID:abc-123@example.com", "SEQUENCE:2", "PARTSTAT=ACCEPTED", "mailto:sam@example.com", "ORGANIZER"} {
		if !strings.Contains(s, want) {
			t.Errorf("reply lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "max@example.com") || strings.Contains(s, "RSVP") {
		t.Errorf("reply names others:\n%s", s)
	}
	mine, _ := Encode(Accepted(raw, "sam@example.com", "Sam", "TENTATIVE"))
	if strings.Contains(string(mine), "METHOD") || !strings.Contains(string(mine), "PARTSTAT=TENTATIVE") || !strings.Contains(string(mine), "max@example.com") {
		t.Errorf("my copy:\n%s", mine)
	}
	// Invited at an address the invitation doesn't list (an alias).
	r2, _ := Reply(raw, "s.taylor@example.com", "", "DECLINED")
	b2, _ := Encode(r2)
	if !strings.Contains(string(b2), "mailto:s.taylor@example.com") || !strings.Contains(string(b2), "PARTSTAT=DECLINED") {
		t.Errorf("alias reply:\n%s", b2)
	}
}
