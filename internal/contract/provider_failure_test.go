package contract_test

import (
	"encoding/json"
	"testing"

	"multiharness-core/internal/contract"
)

func TestProviderFailureContract(t *testing.T) {
	for _, kind := range []contract.ProviderFailureKind{
		contract.ProviderBillingExhausted,
		contract.ProviderRateLimited,
		contract.ProviderOverloaded,
		contract.ProviderAuthentication,
		contract.ProviderAccessDenied,
		contract.ProviderUnknown,
		contract.ProviderConnection, contract.ProviderContextLimit, contract.ProviderInvalidRequest,
	} {
		original := contract.ProviderFailure{Kind: kind, Attempts: 2}
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var decoded contract.ProviderFailure
		if err := json.Unmarshal(data, &decoded); err != nil || decoded != original || decoded.Validate() != nil || decoded.Error() == "" || decoded.Action() == "" {
			t.Fatalf("invalid provider contract: %#v", decoded)
		}
	}
	for _, failure := range []contract.ProviderFailure{
		{Kind: "invented", Attempts: 1},
		{Kind: contract.ProviderUnknown, Attempts: 1, Source: "secret"},
		{Kind: contract.ProviderUnknown, Attempts: 1, Reason: "secret"},
		{Kind: contract.ProviderUnknown, Attempts: 1, HTTPStatus: 999},
		{Kind: contract.ProviderUnknown},
		{Kind: contract.ProviderRateLimited, Attempts: 1, RetryAfterMillis: -1},
		{Kind: contract.ProviderBillingExhausted, Attempts: 1, RetryAfterMillis: 1},
	} {
		if failure.Validate() == nil {
			t.Fatal("accepted invalid provider failure")
		}
	}
}
