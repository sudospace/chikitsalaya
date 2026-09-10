package appointment

import (
	"fmt"
	"time"

	"chikitsalaya/internal/opd/schedule"
)

// Slot is one bookable window on a specific date.
type Slot struct {
	Start           time.Time
	End             time.Time
	DurationMinutes int
	ServiceUnitID   *int64
	ServiceUnitName string
}

// BookedRange is an existing appointment's occupied time window.
type BookedRange struct {
	Start time.Time
	End   time.Time
}

// Overlaps is half-open: a slot ending exactly when another begins is not a conflict.
func Overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

// GenerateDaySlots expands weekly schedule blocks into concrete slots for
// one date. A misconfigured block (end <= start, non-positive duration)
// silently contributes no slots rather than erroring.
func GenerateDaySlots(schedules []schedule.Schedule, date time.Time) ([]Slot, error) {
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	weekday := int(date.Weekday())

	var slots []Slot
	for _, sc := range schedules {
		if !sc.IsActive || sc.DayOfWeek != weekday {
			continue
		}
		start, err := parseTimeOfDay(date, sc.StartTime)
		if err != nil {
			return nil, fmt.Errorf("schedule %d: parse start_time: %w", sc.ID, err)
		}
		end, err := parseTimeOfDay(date, sc.EndTime)
		if err != nil {
			return nil, fmt.Errorf("schedule %d: parse end_time: %w", sc.ID, err)
		}
		step := time.Duration(sc.SlotDurationMinutes) * time.Minute
		if step <= 0 {
			continue
		}
		for t := start; !t.Add(step).After(end); t = t.Add(step) {
			slots = append(slots, Slot{
				Start:           t,
				End:             t.Add(step),
				DurationMinutes: sc.SlotDurationMinutes,
				ServiceUnitID:   sc.ServiceUnitID,
				ServiceUnitName: sc.ServiceUnitName,
			})
		}
	}
	return slots, nil
}

func parseTimeOfDay(date time.Time, hhmm string) (time.Time, error) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), 0, 0, date.Location()), nil
}

// AvailableSlots returns the slots that don't overlap any booked range.
func AvailableSlots(slots []Slot, booked []BookedRange) []Slot {
	var out []Slot
	for _, s := range slots {
		conflict := false
		for _, b := range booked {
			if Overlaps(s.Start, s.End, b.Start, b.End) {
				conflict = true
				break
			}
		}
		if !conflict {
			out = append(out, s)
		}
	}
	return out
}
