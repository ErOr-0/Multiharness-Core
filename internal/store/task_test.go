package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskInputRepairAvailable(t *testing.T) {
	tests := []struct {
		name              string
		maximum           int
		completed         int
		expectedAvailable bool
	}{
		{name: "zero permits no repair", maximum: 0, completed: 0, expectedAvailable: false},
		{name: "first repair is available", maximum: 2, completed: 0, expectedAvailable: true},
		{name: "second repair is available", maximum: 2, completed: 1, expectedAvailable: true},
		{name: "limit blocks another repair", maximum: 2, completed: 2, expectedAvailable: false},
		{name: "negative completed count is invalid", maximum: 2, completed: -1, expectedAvailable: false},
		{name: "negative maximum is invalid", maximum: -1, completed: 0, expectedAvailable: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := TaskInput{MaxRepairAttempts: test.maximum}
			if actual := input.RepairAvailable(test.completed); actual != test.expectedAvailable {
				t.Fatalf("RepairAvailable(%d) = %t; want %t", test.completed, actual, test.expectedAvailable)
			}
		})
	}
}

func TestTaskInputValidate(t *testing.T) {
	tests := []struct {
		name      string
		input     TaskInput
		wantField string
	}{
		{name: "valid", input: validTaskInput()},
		{
			name: "nonexistent path remains structurally valid",
			input: TaskInput{
				Task:       "add tests",
				WorkingDir: "/path/that/does/not/need/to/exist",
			},
		},
		{name: "blank task", input: TaskInput{Task: " ", WorkingDir: "/workspace"}, wantField: "task"},
		{name: "blank working directory", input: TaskInput{Task: "add tests", WorkingDir: " "}, wantField: "working_dir"},
		{
			name: "negative repair limit",
			input: TaskInput{
				Task:              "add tests",
				WorkingDir:        "/workspace",
				MaxRepairAttempts: -1,
			},
			wantField: "max_repair_attempts",
		},
		{
			name: "valid session id",
			input: TaskInput{
				Task:              "add tests",
				WorkingDir:        "/workspace",
				MaxRepairAttempts: 1,
				SessionID:         "ses_123456",
			},
		},
		{
			name: "invalid leading dash session id",
			input: TaskInput{
				Task:              "add tests",
				WorkingDir:        "/workspace",
				MaxRepairAttempts: 1,
				SessionID:         "-dash",
			},
			wantField: "session_id",
		},
		{
			name: "invalid whitespace session id",
			input: TaskInput{
				Task:              "add tests",
				WorkingDir:        "/workspace",
				MaxRepairAttempts: 1,
				SessionID:         "ses with spaces",
			},
			wantField: "session_id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertValidationResult(t, test.input.Validate(), test.wantField)
		})
	}
}

func TestTaskInputJSON(t *testing.T) {
	input := validTaskInput()
	assertJSONRoundTrip(t, input)

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal() returned an error: %v", err)
	}
	encoded := string(data)
	if !strings.Contains(encoded, `"max_repair_attempts":2`) {
		t.Fatalf("JSON = %s; expected max_repair_attempts", encoded)
	}
	if strings.Contains(encoded, "max_review_rounds") {
		t.Fatalf("JSON = %s; contains obsolete max_review_rounds", encoded)
	}
}
