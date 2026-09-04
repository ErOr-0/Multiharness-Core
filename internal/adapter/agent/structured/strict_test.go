package structured_test

import (
	"strings"
	"testing"

	"multiharness-core/internal/adapter/agent/structured"
)

func TestStrictAgentResponses(t *testing.T) {
	for _, role := range []struct {
		name, valid string
		parse       func([]byte) error
	}{
		{"plan", `{"schema_version":"2","action":"implement","answer":"","summary":"Plan","steps":["Edit"],"acceptance_criteria":["Pass"]}`, func(data []byte) error { _, err := structured.ParsePlan(data); return err }},
		{"review", `{"schema_version":"1","approved":true,"summary":"Review","findings":[],"suggestions":[]}`, func(data []byte) error { _, err := structured.ParseReview(data); return err }},
		{"implementation", `{"schema_version":"1","summary":"Implemented","changed_files":[]}`, func(data []byte) error { _, err := structured.ParseImplementation(data); return err }},
	} {
		t.Run(role.name, func(t *testing.T) {
			if err := role.parse([]byte(role.valid)); err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string]string{
				"duplicate":       strings.TrimSuffix(role.valid, "}") + `,"summary":"second"}`,
				"escaped key":     strings.TrimSuffix(role.valid, "}") + `,"summar\u0079":"second"}`,
				"case collision":  strings.TrimSuffix(role.valid, "}") + `,"SUMMARY":"second"}`,
				"uppercase only":  strings.Replace(role.valid, `"summary"`, `"SUMMARY"`, 1),
				"invalid UTF-8":   strings.Replace(role.valid, `"summary":"`, "\"summary\":\"\xff", 1),
				"deep nesting":    `{"unknown":` + strings.Repeat("[", 1000) + "0" + strings.Repeat("]", 1000) + "}",
				"trailing value":  role.valid + " true",
				"unknown field":   strings.TrimSuffix(role.valid, "}") + `,"unexpected":true}`,
				"truncated JSON":  strings.TrimSuffix(role.valid, "}"),
				"top-level array": "[" + role.valid + "]",
			} {
				t.Run(name, func(t *testing.T) {
					if role.parse([]byte(data)) == nil {
						t.Fatal("accepted ambiguous or malformed agent response")
					}
				})
			}
		})
	}
}

func TestStrictNestedReviewFindings(t *testing.T) {
	valid := `{"schema_version":"1","approved":true,"summary":"Review","findings":[{"severity":"info","blocking":false,"file":"","line":0,"description":"Observation","evidence":"","required_action":""}],"suggestions":[]}`
	if _, err := structured.ParseReview([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, replacement := range []string{`"blocking":true,"blocking":false`, `"blocking":true,"BLOCKING":false`, `"blocking":true,"block\u0069ng":false`} {
		if _, err := structured.ParseReview([]byte(strings.Replace(valid, `"blocking":false`, replacement, 1))); err == nil {
			t.Fatal("ambiguous nested finding became approval")
		}
	}
}
