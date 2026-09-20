package openrouter

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/store"
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
	t.Run("planning", func(t *testing.T) {
		before := len(transport.statuses)
		result, err := client.DecidePlanning(ctx, store.TaskInput{Task: "Design and implement a multi-service database migration with backwards-compatible APIs, rollback, and integration tests."})
		assertLive(t, before, result.Reason, result.Model, err)
		if !result.NeedsPlanning {
			t.Fatal("complex migration incorrectly bypassed planning")
		}
	})
	t.Run("review", func(t *testing.T) {
		before := len(transport.statuses)
		result, err := client.DecideReview(ctx, store.ReviewRequest{
			Input:          store.TaskInput{Task: "Fix a spelling mistake in README.md."},
			Plan:           store.Plan{Summary: "Correct teh to the in README.md."},
			Implementation: store.ImplementationResult{ChangedFiles: []string{"README.md"}},
			Repository:     &store.RepositoryEvidence{Complete: true, ChangedFiles: []string{"README.md"}, Diff: "--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-teh project\n+the project\n"},
			Validation:     store.ValidationReport{Passed: true, Checks: []store.ValidationEvidence{{Command: "spelling check", Passed: true}}},
		})
		assertLive(t, before, result.Reason, result.Model, err)
		if result.Approved == result.ShouldReview {
			t.Fatal("inconsistent live review decision")
		}
	})
}
