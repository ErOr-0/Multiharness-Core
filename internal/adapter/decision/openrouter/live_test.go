package openrouter

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/contract"
)

// This transport observes real requests; it never substitutes provider responses.
// Only status codes are retained, so failures cannot print credentials or bodies.
type liveTransport struct {
	statuses []int
	failed   bool
}

func (r *liveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		r.failed = true
		return nil, err
	}
	r.statuses = append(r.statuses, resp.StatusCode)
	return resp, nil
}

func TestLiveJevDecisions(t *testing.T) {
	if os.Getenv("MULTIHARNESS_JEV_SMOKE") != "1" {
		t.Skip("opt-in: make live-jev with OPENROUTER_API_KEY configured")
	}
	if os.Getenv("CI") != "" {
		t.Fatal("live Jev tests require local opt-in; do not provide credentials to ordinary CI")
	}
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if key == "" {
		t.Fatal("OPENROUTER_API_KEY is required; refusing to substitute heuristics")
	}
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.APIKey = key
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal("invalid live Jev configuration")
	}
	transport := &liveTransport{}
	client.httpClient.Transport = transport
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	assertLive := func(t *testing.T, before int, reason, model string, err error) {
		t.Helper()
		if err != nil || transport.failed || len(transport.statuses) != before+1 {
			t.Fatal("live Jev request failed (details withheld); no fallback is accepted")
		}
		status := transport.statuses[before]
		if status != http.StatusOK || model != cfg.Model || !strings.HasPrefix(reason, "jev choice=") {
			t.Fatalf("live Jev response not accepted: HTTP %d; fallback or invalid response", status)
		}
		t.Logf("authenticated %s: HTTP %d; %s", model, status, reason)
	}
	for _, tc := range []struct {
		name, task string
		route      contract.TaskRoute
	}{
		{"question", "Hi! Do you think the current agent loop integration properly follows industry practice?", contract.RouteAnswer},
		{"question_about_simple_change", "Does the README need a typo fix? Explain without changing any files.", contract.RouteAnswer},
		{"planning", "Design and implement a multi-service database migration with backwards-compatible APIs, rollback, and integration tests.", contract.RoutePlan},
		{"direct_implementation", "In README.md, replace the single misspelling 'teh project' with 'the project'.", contract.RouteImplement},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := len(transport.statuses)
			result, err := client.DecidePlanning(ctx, contract.TaskInput{Task: tc.task})
			assertLive(t, before, result.Reason, result.Model, err)
			if result.Route != tc.route || result.Source != contract.DecisionJev || result.Validate() != nil {
				t.Fatalf("unexpected route: got %s, want %s", result.Route, tc.route)
			}
		})
	}
	t.Run("review", func(t *testing.T) {
		before := len(transport.statuses)
		result, err := client.DecideReview(ctx, contract.ReviewRequest{
			Input:          contract.TaskInput{Task: "Fix a spelling mistake in README.md."},
			Plan:           contract.Plan{Summary: "Correct teh to the in README.md."},
			Implementation: contract.ImplementationResult{ChangedFiles: []string{"README.md"}},
			Repository:     &contract.RepositoryEvidence{Complete: true, ChangedFiles: []string{"README.md"}, Diff: "--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-teh project\n+the project\n"},
			Validation:     contract.ValidationReport{Passed: true, Checks: []contract.ValidationEvidence{{Command: "spelling check", Passed: true}}},
		})
		assertLive(t, before, result.Reason, result.Model, err)
		if result.Approved == result.ShouldReview {
			t.Fatal("inconsistent live review decision")
		}
	})
}

// TestLiveLayaDecisions exercises a self-hosted Laya server (for example the
// Docker container on 127.0.0.1:8765). It is opt-in and never falls back.
func TestLiveLayaDecisions(t *testing.T) {
	if os.Getenv("MULTIHARNESS_LAYA_SMOKE") != "1" {
		t.Skip("opt-in: make live-laya with a Laya server running (LAYA_ENDPOINT, LAYA_MODEL and LAYA_API_KEY optional)")
	}
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Provider = ProviderLaya
	cfg.Model, cfg.Endpoint, _ = ProviderDefaults(ProviderLaya)
	if value := strings.TrimSpace(os.Getenv("LAYA_ENDPOINT")); value != "" {
		cfg.Endpoint = value
	}
	if value := strings.TrimSpace(os.Getenv("LAYA_MODEL")); value != "" {
		cfg.Model = value
	}
	cfg.APIKey = strings.TrimSpace(os.Getenv("LAYA_API_KEY"))
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal("invalid live Laya configuration")
	}
	transport := &liveTransport{}
	client.httpClient.Transport = transport
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	for _, tc := range []struct {
		name, task string
		route      contract.TaskRoute
	}{
		{"question", "Hi! Do you think the current agent loop integration properly follows industry practice?", contract.RouteAnswer},
		{"planning", "Design and implement a multi-service database migration with backwards-compatible APIs, rollback, and integration tests.", contract.RoutePlan},
		{"direct_implementation", "In README.md, replace the single misspelling 'teh project' with 'the project'.", contract.RouteImplement},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := len(transport.statuses)
			result, err := client.DecidePlanning(ctx, contract.TaskInput{Task: tc.task})
			if err != nil || transport.failed || len(transport.statuses) != before+1 || transport.statuses[before] != http.StatusOK {
				t.Fatal("live Laya request failed (details withheld); no fallback is accepted")
			}
			if result.Source != contract.DecisionLaya || !strings.HasPrefix(result.Reason, "laya choice=") || result.Validate() != nil {
				t.Fatalf("live Laya response not accepted: %s", result.Reason)
			}
			t.Logf("%s: HTTP %d; %s", cfg.Model, transport.statuses[before], result.Reason)
			if result.Route != tc.route {
				t.Fatalf("unexpected route: got %s, want %s", result.Route, tc.route)
			}
		})
	}
}
