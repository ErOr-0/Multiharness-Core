// Package provider translates CLI error envelopes into safe workflow contracts.
// It never parses task/tool text as a provider response, performs model calls,
// or exposes raw provider data in a public error.
package provider

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
)

type errorDetails struct {
	codes, messages []string
	status          int
	retries         []string
	ambiguous       bool
}

// Classify accepts only an actual provider error payload, not arbitrary output.
// Unknown HTTP 429 is deliberately not retryable: it may represent billing.
func Classify(data []byte, now time.Time) *store.ProviderFailure {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return Text(string(data))
	}
	if structured.ValidateJSON(data) != nil {
		return &store.ProviderFailure{Kind: store.ProviderUnknown, Attempts: 1}
	}
	d := errorDetails{}
	d.read(value, 0)
	if d.ambiguous {
		return &store.ProviderFailure{Kind: store.ProviderUnknown, Attempts: 1}
	}
	code := strings.ToLower(strings.Join(d.codes, " "))
	message := strings.ToLower(strings.Join(d.messages, " "))
	kind := store.ProviderUnknown
	switch {
	case contains(
		code,
		"insufficient_quota",
		"credit_balance_exhausted",
		"billing_hard_limit_reached",
		"billing_limit_exceeded",
		"organization_spend_limit_exceeded",
		"project_spend_limit_exceeded",
		"organization_usage_limit_exceeded",
		"quota_exceeded",
		"credits_exhausted",
	), billing(message), d.status == 402:
		kind = store.ProviderBillingExhausted
	case contains(code, "invalid_api_key", "authentication_error", "authentication_failed", "providerautherror"), d.status == 401:
		kind = store.ProviderAuthentication
	case contains(code, "permission_denied", "permission_error", "model_not_found", "access_denied"), d.status == 403:
		kind = store.ProviderAccessDenied
	case contains(code, "rate_limit_exceeded", "rate_limit_error", "slow_down", "too_many_requests"), contains(message, "rate limit reached", "rate limit exceeded", "too many requests", "requests per minute", "tokens per minute"):
		kind = store.ProviderRateLimited
	case contains(code, "server_is_overloaded", "overloaded_error", "service_unavailable_error", "server_error"), d.status == 500 || d.status == 502 || d.status == 503 || d.status == 504 || d.status == 529:
		kind = store.ProviderOverloaded
	default:
		if f := Text(message); f != nil {
			kind = f.Kind
		}
	}
	f := &store.ProviderFailure{Kind: kind, Attempts: 1}
	if f.Transient() {
		for _, retry := range d.retries {
			// Multiple envelopes/headers can describe one failure. Never shorten
			// a supplied minimum because a later field or map iteration wins.
			f.RetryAfterMillis = max(f.RetryAfterMillis, retryMillis(retry, now))
		}
	}
	return f
}

// Text is a conservative fallback for a failed process or an explicitly marked
// error line. A bare number, token, or the word "quota" is insufficient evidence.
func Text(text string) *store.ProviderFailure {
	v := strings.ToLower(text)
	kind := store.ProviderUnknown
	switch {
	case billing(v), contains(
		v,
		"insufficient_quota",
		"credit_balance_exhausted",
		"billing_hard_limit_reached",
		"organization_spend_limit_exceeded",
		"project_spend_limit_exceeded",
		"organization_usage_limit_exceeded",
	):
		kind = store.ProviderBillingExhausted
	case contains(v, "invalid_api_key", "invalid api key", "authentication failed", "not authenticated", "not logged in"):
		kind = store.ProviderAuthentication
	case contains(v, "model_not_found", "access denied", "permission denied for model"):
		kind = store.ProviderAccessDenied
	case contains(v, "rate_limit_exceeded", "rate_limit_error", "rate limit exceeded", "rate limit reached", "too many requests"):
		kind = store.ProviderRateLimited
	case contains(v, "server_is_overloaded", "overloaded_error", "server is overloaded", "model is overloaded", "service unavailable"):
		kind = store.ProviderOverloaded
	default:
		return nil
	}
	return &store.ProviderFailure{Kind: kind, Attempts: 1}
}

func billing(s string) bool {
	return contains(
		s,
		"quota exhausted",
		"quota exceeded",
		"exceeded your current quota",
		"billing quota",
		"billing limit",
		"spend limit",
		"spending limit",
		"usage limit reached",
		"hit your usage limit",
		"you've hit your usage limit",
		"insufficient credits",
		"insufficient balance",
		"credit balance exhausted",
		"credits exhausted",
		"out of credits",
		"no balance left",
		"payment required",
	)
}
func contains(s string, candidates ...string) bool {
	for _, c := range candidates {
		if strings.Contains(s, c) {
			return true
		}
	}
	return false
}

func (d *errorDetails) read(value any, depth int) {
	if depth > 8 {
		return
	}
	if s, ok := value.(string); ok {
		var nested any
		if json.Unmarshal([]byte(s), &nested) == nil {
			if _, object := nested.(map[string]any); object {
				if structured.ValidateJSON([]byte(s)) != nil {
					d.ambiguous = true
					return
				}
				d.read(nested, depth+1)
				return
			}
		}
		d.messages = append(d.messages, s)
		return
	}
	m, ok := value.(map[string]any)
	if !ok {
		return
	}
	// Ordered, explicit fields only. Do not traverse request bodies, prompts or
	// arbitrary response metadata that may contain secrets or task instructions.
	for _, k := range []string{"code", "type", "name"} {
		if s, ok := m[k].(string); ok {
			d.codes = append(d.codes, s)
		}
	}
	if s, ok := m["message"].(string); ok {
		d.messages = append(d.messages, s)
	}
	for _, k := range []string{"statusCode", "status_code", "status"} {
		if n, ok := m[k].(float64); ok && n >= 100 && n <= 599 && n == math.Trunc(n) {
			d.status = int(n)
		}
	}
	for _, k := range []string{"retry_after", "retryAfter", "retry-after"} {
		value, present := m[k]
		if !present {
			continue
		}
		switch v := value.(type) {
		case string:
			d.retries = append(d.retries, v)
		case float64:
			d.retries = append(d.retries, strconv.FormatFloat(v, 'f', -1, 64))
		default:
			d.retries = append(d.retries, "invalid")
		}
	}
	for _, k := range []string{"responseHeaders", "headers"} {
		if headers, ok := m[k].(map[string]any); ok {
			for name, value := range headers {
				if strings.EqualFold(name, "retry-after") {
					if v, ok := value.(string); ok {
						d.retries = append(d.retries, v)
					} else {
						d.retries = append(d.retries, "invalid")
					}
				}
			}
		}
	}
	for _, k := range []string{"error", "data", "responseBody"} {
		if nested, ok := m[k]; ok {
			d.read(nested, depth+1)
		}
	}
}

func retryMillis(value string, now time.Time) int64 {
	if seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds >= float64(math.MaxInt64)/1000 {
			return math.MaxInt64
		}
		return int64(math.Ceil(seconds * 1000))
	}
	if at, err := http.ParseTime(value); err == nil {
		if !at.After(now) {
			return 0
		}
		return at.Sub(now).Milliseconds()
	}
	// An invalid supplied delay never silently turns into an immediate retry.
	return math.MaxInt64
}
