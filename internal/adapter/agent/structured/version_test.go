package structured_test

import (
	"strings"
	"testing"

	"multiharness-core/internal/adapter/agent/structured"
)

func TestVersionCompatibilityDoesNotCoerceDecisions(t *testing.T) {
	for _, role := range []struct {
		name, version, valid string
		parse                func([]byte) error
	}{
		{"plan", "3", `{"schema_version":"3","action":"implement","answer":"","summary":"Plan","handoff_context":["Observed api.go"],"steps":["Edit"],"acceptance_criteria":["Pass"]}`, func(b []byte) error { _, err := structured.ParsePlan(b); return err }},
		{"implementation", "1", `{"schema_version":"1","summary":"Edited","changed_files":["calc.py"]}`, func(b []byte) error { _, err := structured.ParseImplementation(b); return err }},
		{"review", "1", `{"schema_version":"1","approved":true,"summary":"Verified","findings":[],"suggestions":[]}`, func(b []byte) error { _, err := structured.ParseReview(b); return err }},
	} {
		t.Run(role.name, func(t *testing.T) {
			field := `"schema_version":"` + role.version + `"`
			for _, spelling := range []string{`"` + role.version + `"`, role.version} {
				if err := role.parse([]byte(strings.Replace(role.valid, field, `"schema_version":`+spelling, 1))); err != nil {
					t.Fatal(err)
				}
			}
			for _, invalid := range []string{"0", "4", "1.0", "1e0", "true", "null", "[]", `{}`, `"99"`, `" 1 "`} {
				if err := role.parse([]byte(strings.Replace(role.valid, field, `"schema_version":`+invalid, 1))); err == nil {
					t.Fatalf("accepted invalid version %s", invalid)
				}
			}
			for _, suffix := range []string{`,"schema_version":` + role.version, `,"SCHEMA_VERSION":` + role.version, `,"unexpected":true`} {
				if role.parse([]byte(strings.TrimSuffix(role.valid, "}")+suffix+"}")) == nil {
					t.Fatal("accepted ambiguous response")
				}
			}
		})
	}
	for _, bad := range []string{
		`{"schema_version":1,"approved":"true","summary":"Review","findings":[],"suggestions":[]}`,
		`{"schema_version":1,"approved":1,"summary":"Review","findings":[],"suggestions":[]}`,
		`{"schema_version":1,"approved":true,"summary":"Review","findings":[{"severity":"error","blocking":true,"file":"calc.py","line":2,"description":"Wrong result","evidence":"test failed","required_action":"Fix addition"}],"suggestions":[]}`,
	} {
		if _, err := structured.ParseReview([]byte(bad)); err == nil {
			t.Fatal("version compatibility bypassed review validation")
		}
	}
}
