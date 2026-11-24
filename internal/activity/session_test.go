package activity

import (
	"testing"
	"time"

	"github.com/elad/rolebook-backend/internal/model"
)

func intPtr(v int) *int { return &v }

func campaignWithSlot(id string, tz string, slot *model.SessionConfirmedSlot) *model.Campaign {
	return &model.Campaign{Sessions: []model.Session{{
		ID: id, Name: "Session " + id,
		Schedule: &model.SessionSchedule{DMTimezone: tz, ConfirmedSlot: slot},
	}}}
}

func TestStampSession_StartAtWindow(t *testing.T) {
	start := time.Date(2026, 11, 8, 20, 0, 0, 0, time.UTC)
	c := campaignWithSlot("12", "UTC", &model.SessionConfirmedSlot{Date: "2026-11-08", DayPart: "evening", StartAt: &start, DurationMinutes: intPtr(180)})
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{"one hour before start", start.Add(-time.Hour), "12"},
		{"just before window", start.Add(-time.Hour - time.Second), ""},
		{"window end", start.Add(5 * time.Hour), "12"},
		{"just after window", start.Add(5*time.Hour + time.Second), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if id, _, _ := StampSession(c, tc.now); id != tc.want {
				t.Fatalf("got %q want %q", id, tc.want)
			}
		})
	}
}

func TestStampSession_NilDurationDefaults4h(t *testing.T) {
	start := time.Date(2026, 11, 8, 20, 0, 0, 0, time.UTC)
	c := campaignWithSlot("12", "UTC", &model.SessionConfirmedSlot{StartAt: &start, Date: "2026-11-08"})
	if id, _, _ := StampSession(c, start.Add(5*time.Hour+59*time.Minute)); id != "12" {
		t.Fatalf("inside: %q", id)
	}
	if id, _, _ := StampSession(c, start.Add(6*time.Hour+time.Minute)); id != "" {
		t.Fatalf("outside: %q", id)
	}
}

func TestStampSession_DateOnly(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Jerusalem")
	if err != nil {
		t.Fatal(err)
	}
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, loc) }
	cases := []struct {
		name, part string
		now        time.Time
		want       string
	}{
		{"morning start", "morning", at(8, 7, 0), "12"},
		{"morning end", "morning", at(8, 14, 0), "12"},
		{"after morning", "morning", at(8, 14, 1), ""},
		{"noon start", "noon", at(8, 11, 0), "12"},
		{"noon end", "noon", at(8, 19, 0), "12"},
		{"after noon", "noon", at(8, 19, 1), ""},
		{"evening crosses midnight", "evening", at(9, 2, 59), "12"},
		{"after evening", "evening", at(9, 3, 1), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := campaignWithSlot("12", "Asia/Jerusalem", &model.SessionConfirmedSlot{Date: "2026-11-08", DayPart: tc.part})
			if id, _, _ := StampSession(c, tc.now); id != tc.want {
				t.Fatalf("got %q want %q", id, tc.want)
			}
		})
	}
}

func TestStampSession_ReturnsNameAndDate(t *testing.T) {
	c := campaignWithSlot("12", "UTC", &model.SessionConfirmedSlot{Date: "2026-11-08", DayPart: "noon"})
	id, name, date := StampSession(c, time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC))
	if id != "12" || name != "Session 12" || date != "2026-11-08" {
		t.Fatalf("got %q %q %q", id, name, date)
	}
}

func TestStampSession_InvalidTimezone(t *testing.T) {
	c := campaignWithSlot("12", "Mars/Olympus", &model.SessionConfirmedSlot{Date: "2026-11-08", DayPart: "noon"})
	if id, name, date := StampSession(c, time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)); id+name+date != "" {
		t.Fatalf("got %q %q %q", id, name, date)
	}
}

func TestStampSession_NoConfirmedSlot(t *testing.T) {
	c := campaignWithSlot("12", "UTC", nil)
	if id, _, _ := StampSession(c, time.Now()); id != "" {
		t.Fatalf("got %q", id)
	}
	if id, _, _ := StampSession(nil, time.Now()); id != "" {
		t.Fatalf("nil campaign: %q", id)
	}
}

func TestStampSession_OverlapLatestStartWins(t *testing.T) {
	a := time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC)
	b := time.Date(2026, 11, 8, 20, 0, 0, 0, time.UTC)
	c := &model.Campaign{Sessions: []model.Session{
		{ID: "late", Schedule: &model.SessionSchedule{DMTimezone: "UTC", ConfirmedSlot: &model.SessionConfirmedSlot{Date: "2026-11-08", StartAt: &b}}},
		{ID: "early", Schedule: &model.SessionSchedule{DMTimezone: "UTC", ConfirmedSlot: &model.SessionConfirmedSlot{Date: "2026-11-08", StartAt: &a}}},
	}}
	if id, _, _ := StampSession(c, b.Add(time.Hour)); id != "late" {
		t.Fatalf("got %q", id)
	}
}
