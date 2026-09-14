package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/store"
)

func TestUnsupportedParameterDiagnosticRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := &store.TaskFailure{Stage: store.WorkflowStageImplementation, Code: store.FailureCodeAgent, Message: "private raw message", Provider: &store.ProviderFailure{Kind: store.ProviderInvalidRequest, Reason: "unsupported_parameter", Parameter: "prompt_cache_key", HTTPStatus: 400, Attempts: 1}}
	if err := saveProviderDiagnostic(dir, "run_fixture123", f); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, diagnosticFilename))
	if err != nil {
		t.Fatal(err)
	}
	var record providerDiagnostic
	if json.Unmarshal(data, &record) != nil || record.validate() != nil || record.Provider.Parameter != "prompt_cache_key" || strings.Contains(string(data), "private") {
		t.Fatalf("invalid saved record: %s", data)
	}
	for _, bad := range []store.ProviderFailure{
		{Kind: store.ProviderInvalidRequest, Reason: "unsupported_parameter", Parameter: "private-secret", Attempts: 1},
		{Kind: store.ProviderInvalidRequest, Reason: "invalid_request", Parameter: "prompt_cache_key", Attempts: 1},
		{Kind: store.ProviderRateLimited, Reason: "unsupported_parameter", Parameter: "prompt_cache_key", Attempts: 1},
	} {
		record.Provider = bad
		if record.validate() == nil {
			t.Fatal("accepted unsafe diagnostic")
		}
	}
}
