package structured

import (
	"errors"
	"strings"
	"testing"

	"multiharness-core/internal/store"
)

func TestParsePlanRejectsMalformedOrInvalidOutput(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "malformed", data: `{"schema_version":`},
		{
			name: "unknown field",
			data: `{"schema_version":"2","action":"implement","answer":"","summary":"Plan","steps":["Step"],"acceptance_criteria":["Done"],"extra":true}`,
		},
		{
			name: "multiple documents",
			data: `{"schema_version":"2","action":"implement","answer":"","summary":"Plan","steps":["Step"],"acceptance_criteria":["Done"]} {}`,
		},
		{
			name: "unsupported schema",
			data: `{"schema_version":"1","action":"implement","answer":"","summary":"Plan","steps":["Step"],"acceptance_criteria":["Done"]}`,
		},
		{
			name: "blank summary",
			data: `{"schema_version":"2","action":"implement","answer":"","summary":" ","steps":["Step"],"acceptance_criteria":["Done"]}`,
		},
		{
			name: "missing steps",
			data: `{"schema_version":"2","action":"implement","answer":"","summary":"Plan","steps":[],"acceptance_criteria":["Done"]}`,
		},
		{
			name: "missing acceptance criteria",
			data: `{"schema_version":"2","action":"implement","answer":"","summary":"Plan","steps":["Step"],"acceptance_criteria":[]}`,
		},
		{
			name: "null required field",
			data: `{"schema_version":"2","action":"implement","answer":"","summary":"Plan","steps":null,"acceptance_criteria":["Done"]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParsePlan([]byte(test.data))
			var outputErr *OutputError
			if !errors.As(err, &outputErr) || outputErr.Role != rolePlanning {
				t.Fatalf("ParsePlan() error = %v; want planning OutputError", err)
			}
		})
	}
}

func TestParsePlanRequiresAnExplicitAnswerOrImplementationDecision(t *testing.T) {
	data := `{"schema_version":"2","action":"answer","answer":"The explanation.","summary":"Explained","steps":[],"acceptance_criteria":[]}`
	plan, err := ParsePlan([]byte(data))
	if err != nil || plan.Action != store.PlanActionAnswer || plan.Answer != "The explanation." {
		t.Fatalf("plan=%#v error=%v", plan, err)
	}
	for _, invalid := range []string{
		strings.Replace(data, `"action":"answer",`, "", 1),
		strings.Replace(data, `"action":"answer"`, `"action":null`, 1),
		strings.Replace(data, `"action":"answer"`, `"action":"unknown"`, 1),
		strings.Replace(data, `"answer":"The explanation.",`, "", 1),
		strings.Replace(data, `"answer":"The explanation."`, `"answer":null`, 1),
		strings.Replace(data, `"answer":"The explanation."`, `"answer":""`, 1),
		strings.Replace(data, `"steps":[]`, `"steps":["edit files"]`, 1),
	} {
		if _, err := ParsePlan([]byte(invalid)); err == nil {
			t.Fatalf("accepted ambiguous plan: %s", invalid)
		}
	}
}

func TestParseReviewRejectsMalformedOrInconsistentOutput(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "malformed", data: `not-json`},
		{
			name: "unknown field",
			data: `{"schema_version":"1","approved":true,"summary":"Approved","findings":[],"suggestions":[],"extra":true}`,
		},
		{
			name: "unsupported schema",
			data: `{"schema_version":"0","approved":true,"summary":"Approved","findings":[],"suggestions":[]}`,
		},
		{
			name: "denied without blocking finding",
			data: `{"schema_version":"1","approved":false,"summary":"Denied","findings":[],"suggestions":[]}`,
		},
		{
			name: "approved with blocking finding",
			data: `{"schema_version":"1","approved":true,"summary":"Wrong","findings":[{"severity":"error","blocking":true,"file":"x.go","line":1,"description":"Broken","evidence":"test fails","required_action":"Fix it"}],"suggestions":[]}`,
		},
		{
			name: "blocking without action",
			data: `{"schema_version":"1","approved":false,"summary":"Wrong","findings":[{"severity":"error","blocking":true,"file":"x.go","line":1,"description":"Broken","evidence":"test fails","required_action":""}],"suggestions":[]}`,
		},
		{
			name: "unsupported severity",
			data: `{"schema_version":"1","approved":false,"summary":"Wrong","findings":[{"severity":"major","blocking":true,"file":"x.go","line":1,"description":"Broken","evidence":"test fails","required_action":"Fix it"}],"suggestions":[]}`,
		},
		{
			name: "missing approved",
			data: `{"schema_version":"1","summary":"Denied","findings":[{"severity":"error","blocking":true,"file":"x.go","line":1,"description":"Broken","evidence":"test fails","required_action":"Fix it"}],"suggestions":[]}`,
		},
		{name: "missing findings", data: `{"schema_version":"1","approved":true,"summary":"Approved","suggestions":[]}`},
		{
			name: "missing nested required field",
			data: `{"schema_version":"1","approved":false,"summary":"Wrong","findings":[{"severity":"error","blocking":true,"file":"x.go","line":1,"description":"Broken","evidence":"test fails"}],"suggestions":[]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseReview([]byte(test.data))
			var outputErr *OutputError
			if !errors.As(err, &outputErr) || outputErr.Role != roleReview {
				t.Fatalf("ParseReview() error = %v; want review OutputError", err)
			}
		})
	}
}
