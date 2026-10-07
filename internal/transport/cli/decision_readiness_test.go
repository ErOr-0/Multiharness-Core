package cli

import (
	"context"
	"encoding/json"
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

func TestLayaReadinessProbesTheSelfHostedServer(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "team"
	cfg.Decision = layaDecision()
	for _, tc := range []struct {
		name   string
		key    string
		status int
		body   string
		err    error
		ready  bool
		detail string
	}{
		{"open server", "", 200, `{"model":"laya","answers":{"probe":{"type":"choice","choice":"ready","confidence":0.99}}}`, nil, true, "without authentication"},
		{"keyed server", "PRIVATE_TOKEN", 200, `{"answers":{"probe":{"choice":"ready","confidence":0.99}}}`, nil, true, "answered at 127.0.0.1:8765"},
		{"needs key", "", 401, `{"error":"PRIVATE_TOKEN"}`, nil, false, "set LAYA_API_KEY or use /login laya"},
		{"rejected key", "PRIVATE_TOKEN", 403, `PRIVATE_TOKEN`, nil, false, "rejected the key"},
		{"wrong path", "", 404, `not found`, nil, false, "/v1/systemone"},
		{"server error", "", 500, `PRIVATE_TOKEN`, nil, false, "HTTP 500"},
		{"no answers", "", 200, `{"choices":[]}`, nil, false, "without System One answers"},
		{"redirect", "", 302, ``, nil, false, "HTTP 302"},
		{"unreachable", "", 0, ``, errors.New("dial tcp PRIVATE_TOKEN"), false, "unreachable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &DecisionCredentials{Getenv: func(name string) string {
				if name == "LAYA_API_KEY" {
					return tc.key
				}
				return "OPENROUTER_PRIVATE_TOKEN"
			}}
			c.setupTransport = setupTransportFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "POST" || r.URL.String() != "http://127.0.0.1:8765/v1/systemone" || r.Header.Get("Content-Type") != "application/json" {
					t.Fatal("wrong probe request", r.Method, r.URL)
				}
				if _, present := r.Header["Authorization"]; (tc.key == "" && present) || (tc.key != "" && r.Header.Get("Authorization") != "Bearer "+tc.key) {
					t.Fatalf("authorization %q", r.Header.Get("Authorization"))
				}
				var body struct {
					Model     string                    `json:"model"`
					Questions map[string]map[string]any `json:"questions"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "laya" || body.Questions["probe"]["type"] != "choice" {
					t.Fatal("probe body", body, err)
				}
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Location": []string{"https://untrusted.invalid"}}}, nil
			})
			status := c.CheckSetup(t.Context(), cfg, true)
			if status.Ready != tc.ready || !strings.Contains(status.Detail, tc.detail) || strings.Contains(status.Detail, "PRIVATE_TOKEN") {
				t.Fatal(status)
			}
		})
	}
	// A Laya key entered this session is dropped once the server rejects it.
	c := &DecisionCredentials{Prompt: func(context.Context) (string, error) { return "stale", nil }, setupTransport: setupTransportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}
	if err := c.Replace(t.Context(), cfg.Decision); err != nil {
		t.Fatal(err)
	}
	if status := c.CheckSetup(t.Context(), cfg, false); status.Ready || c.key != "" {
		t.Fatal("rejected laya key retained", status)
	}
	cfg.Decision.Endpoint = "http://user:secret@127.0.0.1:8765/v1/systemone"
	if status := c.CheckSetup(t.Context(), cfg, false); status.Ready || !strings.Contains(status.Detail, "embedded credentials") || strings.Contains(status.Detail, "secret") {
		t.Fatal(status)
	}
}

func TestJevMissingKeyHintOffersLaya(t *testing.T) {
	c := &DecisionCredentials{}
	cfg := config.Defaults()
	cfg.Mode = "team"
	cfg.Decision.Enabled = true
	for _, prompt := range []bool{false, true} {
		status := c.CheckSetup(t.Context(), cfg, prompt)
		if status.Ready || !strings.Contains(status.Detail, "/set decision-provider laya") || !strings.Contains(status.Detail, "/login jev") {
			t.Fatal(prompt, status)
		}
	}
}
