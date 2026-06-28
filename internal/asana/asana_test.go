package asana

import (
	"testing"
)

func TestParseActualHours(t *testing.T) {
	fieldGID := "cf-hours"
	hours := 4.5
	zero := 0.0

	tests := []struct {
		name   string
		fields []CustomField
		gid    string
		want   float64
	}{
		{
			name:   "empty fields",
			fields: nil,
			gid:    fieldGID,
			want:   0,
		},
		{
			name: "matching field",
			fields: []CustomField{
				{GID: "cf-other", NumberValue: &hours},
				{GID: fieldGID, NumberValue: &hours},
			},
			gid:  fieldGID,
			want: 4.5,
		},
		{
			name: "no matching field",
			fields: []CustomField{
				{GID: "cf-other", NumberValue: &hours},
			},
			gid:  fieldGID,
			want: 0,
		},
		{
			name: "field exists but nil value",
			fields: []CustomField{
				{GID: fieldGID, NumberValue: nil},
			},
			gid:  fieldGID,
			want: 0,
		},
		{
			name: "zero value",
			fields: []CustomField{
				{GID: fieldGID, NumberValue: &zero},
			},
			gid:  fieldGID,
			want: 0,
		},
		{
			name: "multiple fields, match first",
			fields: []CustomField{
				{GID: fieldGID, NumberValue: &hours},
				{GID: fieldGID, NumberValue: &zero},
			},
			gid:  fieldGID,
			want: 4.5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseActualHours(tt.fields, tt.gid); got != tt.want {
				t.Errorf("ParseActualHours() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWeekToDateRange(t *testing.T) {
	tests := []struct {
		name      string
		year      int
		week      int
		wantStart string
		wantEnd   string
	}{
		{
			name:      "2026 week 1",
			year:      2026,
			week:      1,
			wantStart: "2025-12-29",
			wantEnd:   "2026-01-04",
		},
		{
			name:      "2026 week 27",
			year:      2026,
			week:      27,
			wantStart: "2026-06-29",
			wantEnd:   "2026-07-05",
		},
		{
			name:      "2025 week 1",
			year:      2025,
			week:      1,
			wantStart: "2024-12-30",
			wantEnd:   "2025-01-05",
		},
		{
			name:      "2024 week 1 (leap year)",
			year:      2024,
			week:      1,
			wantStart: "2024-01-01",
			wantEnd:   "2024-01-07",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd := WeekToDateRange(tt.year, tt.week)
			if gotStart != tt.wantStart {
				t.Errorf("WeekToDateRange() start = %q, want %q", gotStart, tt.wantStart)
			}
			if gotEnd != tt.wantEnd {
				t.Errorf("WeekToDateRange() end = %q, want %q", gotEnd, tt.wantEnd)
			}
		})
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		name string
		f    float64
		want string
	}{
		{"zero", 0, "0"},
		{"integer", 5, "5"},
		{"one decimal", 3.5, "3.5"},
		{"many decimals", 1.23456789, "1.23456789"},
		{"negative", -2.5, "-2.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatFloat(tt.f); got != tt.want {
				t.Errorf("FormatFloat(%v) = %q, want %q", tt.f, got, tt.want)
			}
		})
	}
}

func TestValidateWeek(t *testing.T) {
	tests := []struct {
		name    string
		year    int
		week    int
		wantErr bool
	}{
		{"week 0", 2026, 0, true},
		{"week 1", 2026, 1, false},
		{"week 53", 2026, 53, false},
		{"week 54", 2026, 54, true},
		{"negative week", 2026, -1, true},
		{"week 27", 2026, 27, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWeek(tt.year, tt.week)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWeek(%d, %d) error = %v, wantErr %v", tt.year, tt.week, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDate(t *testing.T) {
	tests := []struct {
		name    string
		date    string
		wantErr bool
	}{
		{"valid date", "2026-01-15", false},
		{"valid end of year", "2026-12-31", false},
		{"valid leap day", "2024-02-29", false},
		{"invalid format", "01-15-2026", true},
		{"invalid month", "2026-13-01", true},
		{"invalid day", "2026-01-32", true},
		{"empty string", "", true},
		{"garbage", "not-a-date", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDate(tt.date)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDate(%q) error = %v, wantErr %v", tt.date, err, tt.wantErr)
			}
		})
	}
}
