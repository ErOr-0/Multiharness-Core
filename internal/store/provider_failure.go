package store

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
)

// ProviderFailure is both the error contract across agent ports and the safe
// public diagnostic. Raw messages, headers, keys and provider response bodies
// must never be placed in it. Transient does NOT authorize replaying mutations.
type ProviderFailure struct {
	Kind             ProviderFailureKind `json:"kind"`
	RetryAfterMillis int64               `json:"retry_after_millis,omitempty"`
	Attempts         int                 `json:"attempts"`
}

func (f ProviderFailure) Validate() error {
	switch f.Kind {
	case ProviderBillingExhausted, ProviderRateLimited, ProviderOverloaded, ProviderAuthentication, ProviderAccessDenied, ProviderUnknown:
	default:
		return invalid("kind", "unsupported provider failure category")
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
	return f.Kind == ProviderRateLimited || f.Kind == ProviderOverloaded
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
	case ProviderOverloaded:
		return "Wait for provider capacity to recover; inspect partial work before restarting."
	default:
		return "Inspect the agent CLI's provider diagnostics securely; no automatic retry is authorized."
	}
}

func (f ProviderFailure) Error() string {
	return "provider failure (" + string(f.Kind) + "): " + f.Action()
}
