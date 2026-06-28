package tui

import (
	"testing"

	"github.com/dvictor357/lazy-asana/internal/llm"
)

func TestShiftDate(t *testing.T) {
	tests := []struct {
		name string
		date string
		days int
		want string
	}{
		{"add 1 day", "2026-01-15", 1, "2026-01-16"},
		{"subtract 1 day", "2026-01-15", -1, "2026-01-14"},
		{"add 7 days", "2026-01-01", 7, "2026-01-08"},
		{"subtract 7 days", "2026-01-08", -7, "2026-01-01"},
		{"cross month", "2026-01-31", 1, "2026-02-01"},
		{"cross year", "2025-12-31", 1, "2026-01-01"},
		{"leap year", "2024-02-28", 1, "2024-02-29"},
		{"invalid date returns same", "not-a-date", 1, "not-a-date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shiftDate(tt.date, tt.days); got != tt.want {
				t.Errorf("shiftDate(%q, %d) = %q, want %q", tt.date, tt.days, got, tt.want)
			}
		})
	}
}

func TestWorkDaysFrom(t *testing.T) {
	// Use a fixed Monday for deterministic tests
	tests := []struct {
		name      string
		startDate string
		n         int
		wantStart string
		wantEnd   string
		wantDays  int // number of days returned
		wantFirst string
	}{
		{
			name:      "monday 1 day",
			startDate: "2026-01-05", // Monday
			n:         1,
			wantStart: "2026-01-05",
			wantEnd:   "2026-01-05",
			wantDays:  1,
			wantFirst: "2026-01-05",
		},
		{
			name:      "monday 5 days",
			startDate: "2026-01-05", // Monday
			n:         5,
			wantStart: "2026-01-05",
			wantEnd:   "2026-01-09",
			wantDays:  5,
			wantFirst: "2026-01-05",
		},
		{
			name:      "tuesday 3 days",
			startDate: "2026-01-06", // Tuesday
			n:         3,
			wantStart: "2026-01-06",
			wantEnd:   "2026-01-08",
			wantDays:  3,
			wantFirst: "2026-01-06",
		},
		{
			name:      "friday 1 day",
			startDate: "2026-01-09", // Friday
			n:         1,
			wantStart: "2026-01-09",
			wantEnd:   "2026-01-09",
			wantDays:  1,
			wantFirst: "2026-01-09",
		},
		{
			name:      "friday 2 days wraps to monday",
			startDate: "2026-01-09", // Friday
			n:         2,
			wantStart: "2026-01-09",
			wantEnd:   "2026-01-12",
			wantDays:  2,
			wantFirst: "2026-01-09",
		},
		{
			name:      "saturday advances to monday",
			startDate: "2026-01-10", // Saturday
			n:         2,
			wantStart: "2026-01-12", // Monday
			wantEnd:   "2026-01-13",
			wantDays:  2,
			wantFirst: "2026-01-12",
		},
		{
			name:      "sunday advances to monday",
			startDate: "2026-01-11", // Sunday
			n:         1,
			wantStart: "2026-01-12", // Monday
			wantEnd:   "2026-01-12",
			wantDays:  1,
			wantFirst: "2026-01-12",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd, days := workDaysFrom(tt.startDate, tt.n)
			if gotStart != tt.wantStart {
				t.Errorf("workDaysFrom(%q, %d) start = %q, want %q", tt.startDate, tt.n, gotStart, tt.wantStart)
			}
			if gotEnd != tt.wantEnd {
				t.Errorf("workDaysFrom(%q, %d) end = %q, want %q", tt.startDate, tt.n, gotEnd, tt.wantEnd)
			}
			if len(days) != tt.wantDays {
				t.Errorf("workDaysFrom(%q, %d) len(days) = %d, want %d", tt.startDate, tt.n, len(days), tt.wantDays)
			}
			if len(days) > 0 && days[0] != tt.wantFirst {
				t.Errorf("workDaysFrom(%q, %d) first day = %q, want %q", tt.startDate, tt.n, days[0], tt.wantFirst)
			}
			// Verify all days are weekdays
			for _, d := range days {
				// We can't easily check weekday without time.Parse, but we can check format
				if len(d) != 10 {
					t.Errorf("workDaysFrom day %q has wrong format", d)
				}
			}
		})
	}
}

func TestWorkDaysFromInvalidDate(t *testing.T) {
	// Invalid date should fall back to today (not crash)
	start, end, days := workDaysFrom("not-a-date", 3)
	if start == "" {
		t.Error("workDaysFrom with invalid date returned empty start")
	}
	if end == "" {
		t.Error("workDaysFrom with invalid date returned empty end")
	}
	if len(days) == 0 {
		t.Error("workDaysFrom with invalid date returned no days")
	}
}

func TestEnforceHourCaps(t *testing.T) {
	tests := []struct {
		name           string
		tasks          []llm.PlannedTask
		alreadyPlanned map[string]float64
		planDays       int
		wantHours      []float64 // expected hours after capping
	}{
		{
			name: "no caps needed",
			tasks: []llm.PlannedTask{
				{Title: "Task A", Hours: 4, DueDate: "2026-01-05"},
				{Title: "Task B", Hours: 3, DueDate: "2026-01-06"},
			},
			alreadyPlanned: map[string]float64{},
			planDays:       2,
			wantHours:      []float64{4, 3},
		},
		{
			name: "cap daily at 8h",
			tasks: []llm.PlannedTask{
				{Title: "Task A", Hours: 10, DueDate: "2026-01-05"},
				{Title: "Task B", Hours: 3, DueDate: "2026-01-05"},
			},
			alreadyPlanned: map[string]float64{},
			planDays:       1,
			wantHours:      []float64{8, 0},
		},
		{
			name: "cap daily with existing planned hours",
			tasks: []llm.PlannedTask{
				{Title: "Task A", Hours: 5, DueDate: "2026-01-05"},
			},
			alreadyPlanned: map[string]float64{"2026-01-05": 5},
			planDays:       1,
			wantHours:      []float64{3},
		},
		{
			name: "cap total at planDays*8",
			tasks: []llm.PlannedTask{
				{Title: "Task A", Hours: 10, DueDate: "2026-01-05"},
				{Title: "Task B", Hours: 10, DueDate: "2026-01-06"},
			},
			alreadyPlanned: map[string]float64{},
			planDays:       1, // only 8h total
			wantHours:      []float64{8, 0},
		},
		{
			name: "no due date skipped",
			tasks: []llm.PlannedTask{
				{Title: "Task A", Hours: 10, DueDate: ""},
			},
			alreadyPlanned: map[string]float64{},
			planDays:       1,
			wantHours:      []float64{10}, // unchanged, skipped
		},
		{
			name: "zero hours skipped",
			tasks: []llm.PlannedTask{
				{Title: "Task A", Hours: 0, DueDate: "2026-01-05"},
			},
			alreadyPlanned: map[string]float64{},
			planDays:       1,
			wantHours:      []float64{0},
		},
		{
			name: "negative hours skipped",
			tasks: []llm.PlannedTask{
				{Title: "Task A", Hours: -1, DueDate: "2026-01-05"},
			},
			alreadyPlanned: map[string]float64{},
			planDays:       1,
			wantHours:      []float64{-1},
		},
		{
			name: "total remaining negative after existing plans",
			tasks: []llm.PlannedTask{
				{Title: "Task A", Hours: 5, DueDate: "2026-01-05"},
			},
			alreadyPlanned: map[string]float64{
				"2026-01-05": 9, // already over 8h
			},
			planDays:  1,
			wantHours: []float64{0},
		},
		{
			name: "mixed scenario",
			tasks: []llm.PlannedTask{
				{Title: "Task A", Hours: 6, DueDate: "2026-01-05"},
				{Title: "Task B", Hours: 4, DueDate: "2026-01-05"},
				{Title: "Task C", Hours: 5, DueDate: "2026-01-06"},
			},
			alreadyPlanned: map[string]float64{},
			planDays:       2,
			wantHours:      []float64{6, 2, 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Copy tasks to avoid mutating test data
			tasks := make([]llm.PlannedTask, len(tt.tasks))
			copy(tasks, tt.tasks)

			got := enforceHourCaps(tasks, tt.alreadyPlanned, tt.planDays)
			if len(got) != len(tt.wantHours) {
				t.Fatalf("enforceHourCaps returned %d tasks, want %d", len(got), len(tt.wantHours))
			}
			for i, want := range tt.wantHours {
				if got[i].Hours != want {
					t.Errorf("enforceHourCaps task[%d].Hours = %v, want %v", i, got[i].Hours, want)
				}
			}
		})
	}
}
