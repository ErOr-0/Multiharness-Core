package codex

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEmbeddedSchemasAreValidAndVersioned(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		properties []string
		version    string
	}{
		{
			name:       "plan",
			data:       planSchema,
			properties: []string{"schema_version", "action", "answer", "summary", "steps", "acceptance_criteria"},
			version:    "2",
		},
		{
			name:       "review",
			data:       reviewSchema,
			properties: []string{"schema_version", "approved", "summary", "findings", "suggestions"},
			version:    "1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var schema struct {
				AdditionalProperties bool                       `json:"additionalProperties"`
				Properties           map[string]json.RawMessage `json:"properties"`
				Required             []string                   `json:"required"`
			}
			if err := json.Unmarshal(test.data, &schema); err != nil {
				t.Fatalf("embedded schema is invalid JSON: %v", err)
			}
			if schema.AdditionalProperties {
				t.Fatal("additionalProperties = true; want false")
			}
			for _, property := range test.properties {
				if _, exists := schema.Properties[property]; !exists {
					t.Errorf("property %q is missing", property)
				}
				if !contains(schema.Required, property) {
					t.Errorf("property %q is not required", property)
				}
			}
			versionSchema := string(schema.Properties["schema_version"])
			if !strings.Contains(versionSchema, `"enum"`) ||
				!strings.Contains(versionSchema, `"`+test.version+`"`) {
				t.Fatalf("schema_version property = %s; want v%s enum", versionSchema, test.version)
			}
		})
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
