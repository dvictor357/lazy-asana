package llm

import (
	"testing"
)

func TestParsePlanValid(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantLen   int
		wantName  string
		wantHours float64
	}{
		{
			name: "single task",
			response: `[
				{
					"title": "Write tests",
					"description": "Write unit tests for all packages",
					"hours": 3.5,
					"due_date": "2026-01-05",
					"subtasks": []
				}
			]`,
			wantLen:   1,
			wantName:  "Write tests",
			wantHours: 3.5,
		},
		{
			name: "multiple tasks",
			response: `[
				{"title": "Task A", "description": "Desc A", "hours": 2, "due_date": "2026-01-05", "subtasks": []},
				{"title": "Task B", "description": "Desc B", "hours": 4, "due_date": "2026-01-06", "subtasks": []}
			]`,
			wantLen: 2,
		},
		{
			name: "with subtasks",
			response: `[
				{
					"title": "Parent",
					"description": "Parent task",
					"hours": 8,
					"due_date": "2026-01-07",
					"subtasks": [
						{"title": "Sub A", "description": "Sub desc A", "hours": 3},
						{"title": "Sub B", "description": "Sub desc B", "hours": 5}
					]
				}
			]`,
			wantLen:   1,
			wantName:  "Parent",
			wantHours: 8,
		},
		{
			name:     "empty array",
			response: `[]`,
			wantLen:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := ParsePlan(tt.response)
			if err != nil {
				t.Fatalf("ParsePlan() error = %v", err)
			}
			if len(tasks) != tt.wantLen {
				t.Errorf("ParsePlan() len = %d, want %d", len(tasks), tt.wantLen)
			}
			if tt.wantName != "" && len(tasks) > 0 {
				if tasks[0].Title != tt.wantName {
					t.Errorf("ParsePlan() first task title = %q, want %q", tasks[0].Title, tt.wantName)
				}
				if tasks[0].Hours != tt.wantHours {
					t.Errorf("ParsePlan() first task hours = %v, want %v", tasks[0].Hours, tt.wantHours)
				}
			}
			// Check subtasks
			if tt.name == "with subtasks" && len(tasks) > 0 {
				if len(tasks[0].Subtasks) != 2 {
					t.Errorf("ParsePlan() subtasks len = %d, want 2", len(tasks[0].Subtasks))
				}
			}
		})
	}
}

func TestParsePlanInvalid(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{"invalid JSON", `not json at all`},
		{"malformed object", `{"title": "bad"}`},
		{"array with bad element", `[{"title": 123}]`}, // title should be string
		{"empty string", ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePlan(tt.response)
			if err == nil {
				t.Error("ParsePlan() expected error, got nil")
			}
		})
	}
}

func TestParsePlanEmptyResponse(t *testing.T) {
	tasks, err := ParsePlan("")
	if err == nil {
		t.Error("ParsePlan(\"\") expected error, got nil")
	}
	if tasks != nil {
		t.Error("ParsePlan(\"\") expected nil tasks")
	}
}
