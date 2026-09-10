package appointment

import (
	"testing"
	"time"

	"chikitsalaya/internal/opd/schedule"
)

func TestOverlaps(t *testing.T) {
	base := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	at := func(mins int) time.Time { return base.Add(time.Duration(mins) * time.Minute) }

	tests := []struct {
		name         string
		aStart, aEnd time.Time
		bStart, bEnd time.Time
		want         bool
	}{
		{"identical ranges", at(0), at(30), at(0), at(30), true},
		{"fully contained", at(0), at(60), at(15), at(30), true},
		{"partial overlap", at(0), at(30), at(15), at(45), true},
		{"touching boundary, a ends when b starts", at(0), at(30), at(30), at(60), false},
		{"touching boundary, b ends when a starts", at(30), at(60), at(0), at(30), false},
		{"disjoint, gap between", at(0), at(15), at(30), at(45), false},
		{"disjoint, reversed order", at(30), at(45), at(0), at(15), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Overlaps(tt.aStart, tt.aEnd, tt.bStart, tt.bEnd)
			if got != tt.want {
				t.Errorf("Overlaps(%v,%v,%v,%v) = %v, want %v", tt.aStart, tt.aEnd, tt.bStart, tt.bEnd, got, tt.want)
			}
		})
	}
}

// monday returns a fixed Monday, and the other weekdays are derived from it
// so the tests don't depend on a hardcoded weekday number matching the date.
func monday() time.Time {
	d := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	if d.Weekday() != time.Monday {
		panic("test fixture date is not a Monday")
	}
	return d
}

func TestGenerateDaySlots(t *testing.T) {
	mon := monday()
	tue := mon.AddDate(0, 0, 1)

	dow := func(d time.Time) int { return int(d.Weekday()) }

	tests := []struct {
		name      string
		schedules []schedule.Schedule
		date      time.Time
		wantTimes []string // "HH:MM" of each slot start, in order
	}{
		{
			name: "exact fit, no remainder",
			schedules: []schedule.Schedule{
				{ID: 1, DayOfWeek: dow(mon), StartTime: "09:00", EndTime: "10:00", SlotDurationMinutes: 15, IsActive: true},
			},
			date:      mon,
			wantTimes: []string{"09:00", "09:15", "09:30", "09:45"},
		},
		{
			name: "remainder left unscheduled",
			schedules: []schedule.Schedule{
				{ID: 1, DayOfWeek: dow(mon), StartTime: "09:00", EndTime: "09:50", SlotDurationMinutes: 15, IsActive: true},
			},
			date:      mon,
			wantTimes: []string{"09:00", "09:15", "09:30"}, // 09:45 would end at 10:00, past 09:50
		},
		{
			name: "wrong weekday contributes nothing",
			schedules: []schedule.Schedule{
				{ID: 1, DayOfWeek: dow(mon), StartTime: "09:00", EndTime: "10:00", SlotDurationMinutes: 15, IsActive: true},
			},
			date:      tue,
			wantTimes: nil,
		},
		{
			name: "inactive block contributes nothing",
			schedules: []schedule.Schedule{
				{ID: 1, DayOfWeek: dow(mon), StartTime: "09:00", EndTime: "10:00", SlotDurationMinutes: 15, IsActive: false},
			},
			date:      mon,
			wantTimes: nil,
		},
		{
			name: "misconfigured block (end before start) contributes nothing, no error",
			schedules: []schedule.Schedule{
				{ID: 1, DayOfWeek: dow(mon), StartTime: "10:00", EndTime: "09:00", SlotDurationMinutes: 15, IsActive: true},
			},
			date:      mon,
			wantTimes: nil,
		},
		{
			name: "zero slot duration contributes nothing, no error",
			schedules: []schedule.Schedule{
				{ID: 1, DayOfWeek: dow(mon), StartTime: "09:00", EndTime: "10:00", SlotDurationMinutes: 0, IsActive: true},
			},
			date:      mon,
			wantTimes: nil,
		},
		{
			name: "two blocks same day (morning + evening) combine in order",
			schedules: []schedule.Schedule{
				{ID: 1, DayOfWeek: dow(mon), StartTime: "09:00", EndTime: "09:30", SlotDurationMinutes: 15, IsActive: true},
				{ID: 2, DayOfWeek: dow(mon), StartTime: "17:00", EndTime: "17:30", SlotDurationMinutes: 15, IsActive: true},
			},
			date:      mon,
			wantTimes: []string{"09:00", "09:15", "17:00", "17:15"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slots, err := GenerateDaySlots(tt.schedules, tt.date)
			if err != nil {
				t.Fatalf("GenerateDaySlots returned error: %v", err)
			}
			if len(slots) != len(tt.wantTimes) {
				t.Fatalf("got %d slots, want %d (%v)", len(slots), len(tt.wantTimes), slots)
			}
			for i, s := range slots {
				got := s.Start.Format("15:04")
				if got != tt.wantTimes[i] {
					t.Errorf("slot %d start = %s, want %s", i, got, tt.wantTimes[i])
				}
			}
		})
	}
}

func TestAvailableSlots(t *testing.T) {
	mon := monday()
	dow := int(mon.Weekday())

	schedules := []schedule.Schedule{
		{ID: 1, DayOfWeek: dow, StartTime: "09:00", EndTime: "10:00", SlotDurationMinutes: 30, IsActive: true},
	}
	slots, err := GenerateDaySlots(schedules, mon)
	if err != nil {
		t.Fatalf("GenerateDaySlots error: %v", err)
	}
	if len(slots) != 2 {
		t.Fatalf("expected 2 slots (09:00, 09:30), got %d", len(slots))
	}

	at := func(hh, mm int) time.Time {
		return time.Date(mon.Year(), mon.Month(), mon.Day(), hh, mm, 0, 0, mon.Location())
	}

	t.Run("booking exactly the first slot removes only that slot", func(t *testing.T) {
		booked := []BookedRange{{Start: at(9, 0), End: at(9, 30)}}
		available := AvailableSlots(slots, booked)
		if len(available) != 1 || available[0].Start.Format("15:04") != "09:30" {
			t.Fatalf("expected only 09:30 to remain available, got %v", available)
		}
	})

	t.Run("booking that only touches a boundary blocks nothing", func(t *testing.T) {
		// A booking from 08:30-09:00 ends exactly when the first slot starts.
		booked := []BookedRange{{Start: at(8, 30), End: at(9, 0)}}
		available := AvailableSlots(slots, booked)
		if len(available) != 2 {
			t.Fatalf("expected both slots to remain available, got %v", available)
		}
	})

	t.Run("no bookings leaves everything available", func(t *testing.T) {
		available := AvailableSlots(slots, nil)
		if len(available) != 2 {
			t.Fatalf("expected 2 available slots, got %d", len(available))
		}
	})
}
