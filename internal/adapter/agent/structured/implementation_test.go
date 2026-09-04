package structured_test

import (
	"encoding/json"
	"testing"

	"multiharness-core/internal/adapter/agent/structured"
)

func TestSharedImplementationWireContract(t *testing.T) {
	if !json.Valid(structured.ImplementationSchema()) {
		t.Fatal("invalid schema")
	}
	for _, data := range []string{`{}`, `null`, `{"schema_version":"2","summary":"ok","changed_files":[]}`, `{"schema_version":"1","summary":"ok","changed_files":null}`, `{"schema_version":"1","summary":"","changed_files":[]}`, `{"schema_version":"1","summary":"ok","changed_files":[],"extra":true}`, `{"schema_version":"1","summary":"ok","changed_files":[]} {}`} {
		if _, err := structured.ParseImplementation([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	if _, err := structured.ParseImplementation([]byte(`{"schema_version":"1","summary":"ok","changed_files":[]}`)); err != nil {
		t.Fatal(err)
	}
}
