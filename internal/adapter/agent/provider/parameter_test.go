package provider_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/store"
)

func TestUnsupportedRequestParameterRetainsOnlyKnownName(t *testing.T) {
	for _, tc := range []struct{ message, parameter string }{
		{"Error from provider (Console): Upstream request failed: [invalid_request_error] Unrecognized request argument supplied: prompt_cache_key", "prompt_cache_key"},
		{"Unsupported parameter: 'temperature' is not supported; private-secret", "temperature"},
		{"Unknown parameter: private_secret", ""},
	} {
		data, err := json.Marshal(map[string]any{"name": "APIError", "data": map[string]any{"message": tc.message, "statusCode": 400, "responseHeaders": map[string]string{"Authorization": "private-secret"}, "request": map[string]string{"prompt": "private-secret"}}})
		if err != nil {
			t.Fatal(err)
		}
		f := provider.Classify(data, time.Now())
		if f.Kind != store.ProviderInvalidRequest || f.Reason != "unsupported_parameter" || f.Parameter != tc.parameter || f.Transient() || f.Validate() != nil {
			t.Fatalf("wrong diagnostic: %#v", f)
		}
		encoded, _ := json.Marshal(f)
		if strings.Contains(string(encoded)+f.Error(), "private") || strings.Contains(f.Error(), "Upstream") {
			t.Fatal("raw diagnostic exposed")
		}
	}
	// Higher-priority account failures must not inherit parameter metadata.
	f := provider.Classify([]byte(`{"statusCode":401,"message":"Unsupported parameter: temperature"}`), time.Now())
	if f.Kind != store.ProviderAuthentication || f.Parameter != "" || f.Reason != "" {
		t.Fatalf("wrong precedence: %#v", f)
	}
}
