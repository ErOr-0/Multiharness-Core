package provider_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/store"
)

func TestClassifyProviderErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		kind       store.ProviderFailureKind
	}{
		{"billing 429", `{"statusCode":429,"code":"insufficient_quota"}`, store.ProviderBillingExhausted},
		{"credits", `{"error":{"code":"credit_balance_exhausted"}}`, store.ProviderBillingExhausted},
		{"organization spend", `{"code":"organization_spend_limit_exceeded"}`, store.ProviderBillingExhausted},
		{"project spend", `{"code":"project_spend_limit_exceeded"}`, store.ProviderBillingExhausted},
		{"organization usage", `{"code":"organization_usage_limit_exceeded"}`, store.ProviderBillingExhausted},
		{"quota wins over rate", `{"statusCode":429,"code":"rate_limit_exceeded","message":"quota exhausted"}`, store.ProviderBillingExhausted},
		{"OpenCode payment", `{"name":"APIError","data":{"statusCode":402,"message":"redacted"}}`, store.ProviderBillingExhausted},
		{"nested body", `{"data":{"statusCode":429,"responseBody":"{\"error\":{\"code\":\"insufficient_quota\"}}"}}`, store.ProviderBillingExhausted},
		{"real rate", `{"status_code":429,"code":"rate_limit_exceeded"}`, store.ProviderRateLimited},
		{"slow down", `{"code":"slow_down"}`, store.ProviderRateLimited},
		{"ambiguous 429", `{"statusCode":429}`, store.ProviderUnknown},
		{"overload", `{"code":"server_is_overloaded"}`, store.ProviderOverloaded},
		{"service unavailable", `{"statusCode":503}`, store.ProviderOverloaded},
		{"authentication", `{"statusCode":401,"message":"secret-key"}`, store.ProviderAuthentication},
		{"model access", `{"code":"model_not_found"}`, store.ProviderAccessDenied},
		{"forbidden", `{"statusCode":403}`, store.ProviderAccessDenied},
		{"unknown", `{"code":"new_unknown_code","message":"secret-key"}`, store.ProviderUnknown},
		{"malformed status", `{"statusCode":500.5}`, store.ProviderUnknown},
		{"null", `null`, store.ProviderUnknown},
		{"request text ignored", `{"request":{"message":"quota exhausted"}}`, store.ProviderUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := provider.Classify([]byte(tc.body), time.Now())
			if f == nil || f.Kind != tc.kind || f.Validate() != nil {
				t.Fatalf("failure=%#v", f)
			}
			encoded, _ := json.Marshal(f)
			if strings.Contains(f.Error()+string(encoded), "secret-key") {
				t.Fatal("raw provider data leaked")
			}
		})
	}
}

func TestConflictingOrMalformedRetryAfterNeverShortensWait(t *testing.T) {
	for _, tc := range []struct {
		body   string
		millis int64
	}{
		{`{"code":"rate_limit_exceeded","retry_after":60,"headers":{"Retry-After":"1","retry-after":"2"}}`, 60000},
		{`{"code":"rate_limit_exceeded","headers":{"Retry-After":"60","retry-after":"1"}}`, 60000},
		{`{"code":"rate_limit_exceeded","retry_after":""}`, math.MaxInt64},
		{`{"code":"rate_limit_exceeded","retry_after":null}`, math.MaxInt64},
		{`{"code":"rate_limit_exceeded","headers":{"retry-after":[]}}`, math.MaxInt64},
		{`{"code":"rate_limit_exceeded","retry_after":"invalid","data":{"retry_after":1}}`, math.MaxInt64},
	} {
		failure := provider.Classify([]byte(tc.body), time.Unix(0, 0))
		if failure.RetryAfterMillis != tc.millis {
			t.Fatalf("retry minimum=%d want=%d", failure.RetryAfterMillis, tc.millis)
		}
	}
}

func TestRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 9, 4, 1, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		header string
		millis int64
	}{{"2", 2000}, {"0.0011", 2}, {"Fri, 04 Sep 2026 01:00:37 GMT", 37000}, {"Fri, 04 Sep 2026 00:00:00 GMT", 0}, {"-1", math.MaxInt64}, {"NaN", math.MaxInt64}, {"Inf", math.MaxInt64}, {"9999999999999999999999", math.MaxInt64}, {"invalid", math.MaxInt64}} {
		data, _ := json.Marshal(map[string]any{"code": "rate_limit_exceeded", "responseHeaders": map[string]string{"Retry-After": tc.header}})
		f := provider.Classify(data, now)
		if f.RetryAfterMillis != tc.millis {
			t.Errorf("header=%q got=%d want=%d", tc.header, f.RetryAfterMillis, tc.millis)
		}
	}
	f := provider.Classify([]byte(`{"code":"insufficient_quota","retry_after":3}`), now)
	if f.Transient() || f.RetryAfterMillis != 0 {
		t.Fatal("billing requested retry")
	}
}

func TestConservativeTextFallback(t *testing.T) {
	for _, s := range []string{"quota exhausted", "You've hit your usage limit", "insufficient credits", "Error: exceeded your current quota"} {
		if f := provider.Text(s); f == nil || f.Kind != store.ProviderBillingExhausted {
			t.Fatalf("unclassified %q", s)
		}
	}
	for _, s := range []string{"429", "quota", "billing", "test output", "unexpected generic failure"} {
		if provider.Text(s) != nil {
			t.Fatalf("guessed classification for %q", s)
		}
	}
}

func FuzzClassifyNeverLeaksRawErrors(f *testing.F) {
	f.Add(`{"error":{"code":"insufficient_quota"}}`)
	f.Add(`{"data":{"statusCode":503}}`)
	f.Add("not json")
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 1<<20 {
			t.Skip()
		}
		failure := provider.Classify([]byte(body), time.Unix(0, 0))
		if failure != nil && failure.Validate() != nil {
			t.Fatal("classifier returned invalid contract")
		}
		// Vary only the raw diagnostic; classification may change, but arbitrary
		// diagnostic bytes must not be copied into the public error/JSON contract.
		const secret = "provider-canary-not-for-public-output"
		payload, _ := json.Marshal(map[string]string{"message": body + secret})
		safe := provider.Classify(payload, time.Unix(0, 0))
		encoded, _ := json.Marshal(safe)
		if safe == nil || strings.Contains(safe.Error()+string(encoded), secret) {
			t.Fatal("raw diagnostic leaked")
		}
	})
}
