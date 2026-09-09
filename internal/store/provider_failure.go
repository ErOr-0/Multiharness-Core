package store

import "fmt"

// ProviderFailureKind is provider-neutral. Billing/quota exhaustion is never
// classified as transient merely because a provider used HTTP 429.
type ProviderFailureKind string

const (
	ProviderBillingExhausted ProviderFailureKind = "billing_exhausted"
	ProviderRateLimited      ProviderFailureKind = "rate_limited"
	ProviderOverloaded       ProviderFailureKind = "overloaded"
	ProviderAuthentication   ProviderFailureKind = "authentication_failed"
	ProviderAccessDenied     ProviderFailureKind = "access_denied"
	ProviderUnknown          ProviderFailureKind = "unknown"
	ProviderConnection       ProviderFailureKind = "connection_failed"
	ProviderContextLimit     ProviderFailureKind = "context_limit"
	ProviderInvalidRequest   ProviderFailureKind = "invalid_request"
)

// ProviderFailure is both the error contract across agent ports and the safe
// public diagnostic. Raw messages, headers, keys and provider response bodies
// must never be placed in it. Transient does NOT authorize replaying mutations.
type ProviderFailure struct {
	Kind             ProviderFailureKind `json:"kind"`
	Source           string              `json:"source,omitempty"`
	HTTPStatus       int                 `json:"http_status,omitempty"`
	Reason           string              `json:"reason,omitempty"`
	RetryAfterMillis int64               `json:"retry_after_millis,omitempty"`
	Attempts         int                 `json:"attempts"`
}

func (f ProviderFailure) Validate() error {
	switch f.Kind {
	case ProviderBillingExhausted, ProviderRateLimited, ProviderOverloaded, ProviderAuthentication, ProviderAccessDenied, ProviderUnknown, ProviderConnection, ProviderContextLimit, ProviderInvalidRequest:
	default:
		return invalid("kind", "unsupported provider failure category")
	}
	switch f.Source {
	case "", "error", "turn.failed", "stderr":
	default:
		return invalid("source", "unsupported provider error source")
	}
	switch f.Reason {
	case "", "stream_disconnected", "connection_timeout", "connection_refused", "dns_failure", "tls_failure", "connection_reset", "context_length_exceeded", "invalid_request", "malformed_error_event", "unrecognized_error":
	default:
		return invalid("reason", "unsupported provider error reason")
	}
	if f.HTTPStatus != 0 && (f.HTTPStatus < 100 || f.HTTPStatus > 599) {
		return invalid("http_status", "must be an HTTP status")
	}
	if f.RetryAfterMillis < 0 || f.Attempts < 1 {
		return invalid("provider_failure", "delay must be nonnegative and attempts positive")
	}
	if !f.Transient() && f.RetryAfterMillis != 0 {
		return invalid("retry_after_millis", "terminal provider failures cannot request a retry")
	}
	return nil
}

func (f ProviderFailure) Transient() bool {
	return f.Kind == ProviderRateLimited || f.Kind == ProviderOverloaded || f.Kind == ProviderConnection
}

func (f ProviderFailure) Action() string {
	switch f.Kind {
	case ProviderBillingExhausted:
		return "Check the provider's credits and enforced spending or usage limits before starting another run."
	case ProviderAuthentication:
		return "Authenticate the configured agent CLI with the intended provider account before starting another run."
	case ProviderAccessDenied:
		return "Check the configured model, project and account permissions before starting another run."
	case ProviderRateLimited:
		return "Wait for the provider's retry window and reduce request concurrency; inspect partial work before restarting."
	case ProviderConnection:
		return "The provider connection failed. Check container network, proxy and TLS access; inspect partial work before restarting."
	case ProviderContextLimit:
		return "The provider rejected the context size. Reduce the request or select a model with sufficient context."
	case ProviderInvalidRequest:
		return "The provider rejected the request. Check the model and supported request settings."
	case ProviderOverloaded:
		return "Wait for provider capacity to recover; inspect partial work before restarting."
	default:
		return "Review the retained provider diagnostic record; the error was not recognized and no automatic retry is authorized."
	}
}

func (f ProviderFailure) Error() string {
	detail := string(f.Kind)
	if f.Reason != "" {
		detail += "; " + f.Reason
	}
	if f.HTTPStatus != 0 {
		detail += fmt.Sprintf("; HTTP %d", f.HTTPStatus)
	}
	if f.Source != "" {
		detail += "; source=" + f.Source
	}
	return "provider failure (" + detail + "): " + f.Action()
}
