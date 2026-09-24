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

func TestDecisionCredentialsEnvironmentAndSession(t *testing.T) {
	calls := 0
	c := &DecisionCredentials{Getenv: func(string) string { return "" }, Prompt: func(context.Context) (string, error) { calls++; return "test-session-key", nil }}
	for range 2 {
		key, err := c.Resolve(t.Context(), nil)
		if err != nil || key != "test-session-key" {
			t.Fatal("session key was not retained")
		}
	}
	if calls != 1 {
		t.Fatalf("prompted %d times", calls)
	}
	c.Getenv = func(string) string { return "test-env-key" }
	key, err := c.Resolve(t.Context(), nil)
	if err != nil || key != "test-env-key" || calls != 1 {
		t.Fatal("environment key should take precedence without prompting")
	}
}

func TestDecisionCredentialsExplicitReplacementOverridesEnvironmentForSession(t *testing.T) {
	calls := 0
	c := &DecisionCredentials{
		Getenv: func(string) string { return "old-environment-key" },
		Prompt: func(context.Context) (string, error) {
			calls++
			return "new-hidden-key", nil
		},
	}
	if err := c.Replace(t.Context()); err != nil {
		t.Fatal(err)
	}
	if key, err := c.Resolve(t.Context(), nil); err != nil || key != "new-hidden-key" || calls != 1 {
		t.Fatal("explicit replacement was not used for later requests")
	}
	c.setupTransport = setupTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer new-hidden-key" {
			t.Fatal("readiness checked the old environment key")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{}}`))}, nil
	})
	if status := c.CheckSetup(t.Context(), config.Defaults(), false); !status.Ready {
		t.Fatal(status)
	}
	c.Prompt = func(context.Context) (string, error) { return "", nil }
	if err := c.Replace(t.Context()); err == nil {
		t.Fatal("blank replacement accepted")
	}
	if key, err := c.Resolve(t.Context(), nil); err != nil || key != "new-hidden-key" {
		t.Fatal("cancelled replacement discarded the current key")
	}
}

func TestDecisionCredentialsRejectUnavailableOrCancelledInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prompt func(context.Context) (string, error)
	}{
		{name: "noninteractive"},
		{name: "blank", prompt: func(context.Context) (string, error) { return "", nil }},
		{name: "input error", prompt: func(context.Context) (string, error) { return "secret-value", errors.New("secret-value") }},
		{name: "invalid", prompt: func(context.Context) (string, error) { return "secret-value\nsecond-line", nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &DecisionCredentials{Prompt: tc.prompt}
			key, err := c.Resolve(t.Context(), nil)
			if err == nil || key != "" || c.key != "" {
				t.Fatal("unavailable input became a cached key")
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatal("error leaked key")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c := &DecisionCredentials{Prompt: func(context.Context) (string, error) { t.Fatal("prompted after cancellation"); return "", nil }}
	if _, err := c.Resolve(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}
