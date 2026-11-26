package activity

import (
	"time"

	"github.com/elad/rolebook-backend/internal/model"
)

const defaultSessionDuration = 4 * time.Hour

// dayPartHours is each day-part's local window. An end past 24 is the next day.
var dayPartHours = map[string][2]int{
	"morning": {7, 14},
	"noon":    {11, 19},
	"evening": {16, 27},
}

// StampSession finds the campaign session whose confirmed slot contains now and
// returns its id, name and date. When windows overlap, the latest start wins.
func StampSession(c *model.Campaign, now time.Time) (id, name, date string) {
	if c == nil {
		return "", "", ""
	}
	var bestStart time.Time
	for i := range c.Sessions {
		s := &c.Sessions[i]
		if s.Schedule == nil || s.Schedule.ConfirmedSlot == nil {
			continue
		}
		start, end, ok := slotWindow(s.Schedule)
		if !ok || now.Before(start) || now.After(end) {
			continue
		}
		if id == "" || start.After(bestStart) {
			bestStart = start
			id, name, date = s.ID, s.Name, s.Schedule.ConfirmedSlot.Date
		}
	}
	return id, name, date
}

func slotWindow(schedule *model.SessionSchedule) (start, end time.Time, ok bool) {
	slot := schedule.ConfirmedSlot
	if slot.StartAt != nil {
		duration := defaultSessionDuration
		if slot.DurationMinutes != nil {
			duration = time.Duration(*slot.DurationMinutes) * time.Minute
		}
		return slot.StartAt.Add(-time.Hour), slot.StartAt.Add(duration + 2*time.Hour), true
	}
	hours, known := dayPartHours[slot.DayPart]
	if !known {
		return time.Time{}, time.Time{}, false
	}
	loc, err := time.LoadLocation(schedule.DMTimezone)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	day, err := time.ParseInLocation("2006-01-02", slot.Date, loc)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	y, m, d := day.Date()
	return time.Date(y, m, d, hours[0], 0, 0, 0, loc), time.Date(y, m, d, hours[1], 0, 0, 0, loc), true
}
