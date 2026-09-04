package store_test

import (
	"encoding/json"
	"testing"

	"multiharness-core/internal/store"
)

func TestProviderFailureContract(t *testing.T) {
	for _, kind := range []store.ProviderFailureKind{store.ProviderBillingExhausted, store.ProviderRateLimited, store.ProviderOverloaded, store.ProviderAuthentication, store.ProviderAccessDenied, store.ProviderUnknown} {
		original := store.ProviderFailure{Kind: kind, Attempts: 2}
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var decoded store.ProviderFailure
		if err := json.Unmarshal(data, &decoded); err != nil || decoded != original || decoded.Validate() != nil || decoded.Error() == "" || decoded.Action() == "" {
			t.Fatalf("invalid provider contract: %#v", decoded)
		}
	}
	for _, failure := range []store.ProviderFailure{{Kind: "invented", Attempts: 1}, {Kind: store.ProviderUnknown}, {Kind: store.ProviderRateLimited, Attempts: 1, RetryAfterMillis: -1}, {Kind: store.ProviderBillingExhausted, Attempts: 1, RetryAfterMillis: 1}} {
		if failure.Validate() == nil {
			t.Fatal("accepted invalid provider failure")
		}
	}
}
