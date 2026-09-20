package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"multiharness-core/internal/config"
)

type setupTransportFunc func(*http.Request) (*http.Response, error)

func (f setupTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestJevReadiness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		ready  bool
	}{
		{"valid", 200, `{"data":{"limit_remaining":10}}`, true},
		{"unlimited", 200, `{"data":{"limit_remaining":null}}`, true},
		{"exhausted", 200, `{"data":{"limit_remaining":0}}`, false},
		{"invalid", 401, `PRIVATE_TOKEN`, false},
		{"forbidden", 403, `PRIVATE_TOKEN`, false},
		{"unavailable", 503, `PRIVATE_TOKEN`, false},
		{"redirect", 302, `PRIVATE_TOKEN`, false},
		{"malformed", 200, `PRIVATE_TOKEN`, false},
		{"empty", 200, `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &DecisionCredentials{key: "PRIVATE_TOKEN", setupTransport: setupTransportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://openrouter.ai/api/v1/key" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer PRIVATE_TOKEN" {
					t.Fatal("wrong auth request")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Location": []string{"https://untrusted.invalid"}}}, nil
			})}
			status := c.CheckSetup(t.Context(), config.Defaults(), false)
			if status.Ready != tc.ready || strings.Contains(status.Detail, "PRIVATE_TOKEN") {
				t.Fatal(status)
			}
			if (tc.status == 401 || tc.status == 403) && c.key != "" {
				t.Fatal("rejected key retained")
			}
		})
	}
}
func TestJevSetupPromptAndCancellation(t *testing.T) {
	prompted := 0
	c := &DecisionCredentials{Prompt: func(context.Context) (string, error) { prompted++; return "", nil }}
	cfg := config.Defaults()
	if c.CheckSetup(t.Context(), cfg, false).Ready || prompted != 0 {
		t.Fatal("read-only configuration prompted")
	}
	if c.CheckSetup(t.Context(), cfg, true).Ready || prompted != 1 {
		t.Fatal("missing key passed")
	}
	c.key = "PRIVATE_TOKEN"
	c.setupTransport = setupTransportFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("PRIVATE_TOKEN") })
	if s := c.CheckSetup(t.Context(), cfg, false); s.Ready || strings.Contains(s.Detail, "PRIVATE_TOKEN") {
		t.Fatal(s)
	}
	cfg.Decision.Endpoint = "https://custom.invalid/decisions"
	c.setupTransport = setupTransportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("custom key sent to OpenRouter"); return nil, nil })
	if s := c.CheckSetup(t.Context(), cfg, false); !s.Ready || !strings.Contains(s.Detail, "not verified") {
		t.Fatal("custom authentication status must be explicit")
	}
}
