package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/store"
)

// stubRoundTripper serves canned HTTP responses without opening sockets, so
// the live-decision paths stay hermetic.
type stubRoundTripper struct {
	handler func(*http.Request) (*http.Response, error)
}

func (s stubRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return s.handler(r)
}

func stubResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func disabledClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func liveClient(t *testing.T, handler func(*http.Request) (*http.Response, error)) *Client {
	t.Helper()
	c, err := NewClient(Config{
		Enabled:             true,
		Model:               "typesafe/jev-1.13",
		Endpoint:            "https://jev.test/v1/systemone",
		Timeout:             5 * time.Second,
		ConfidenceThreshold: 0.75,
		APIKey:              "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	c.httpClient = &http.Client{Transport: stubRoundTripper{handler: handler}}
	return c
}

func requireAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer test-key" {
		t.Fatalf("missing bearer auth: %v", r.Header)
	}
}

func TestConfigValidation(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("default config must validate: %v", err)
	}
	enabled := DefaultConfig()
	enabled.Enabled = true
	enabled.Model = ""
	if err := enabled.Validate(); err == nil {
		t.Fatal("blank model must fail validation when enabled")
	}
	enabled = DefaultConfig()
	enabled.Enabled = true
	enabled.ConfidenceThreshold = 1.5
	if err := enabled.Validate(); err == nil {
		t.Fatal("threshold above 1 must fail validation when enabled")
	}
}

// A short simple task must bypass planning: length must not stomp the
// keyword verdict below the confidence threshold.
func TestHeuristicPlanningShortSimpleTaskSkipsPlanning(t *testing.T) {
	decision, err := disabledClient(t).DecidePlanning(context.Background(), store.TaskInput{Task: "Fix typo in README"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.NeedsPlanning {
		t.Fatalf("short simple task must skip planning: %+v", decision)
	}
}

func TestHeuristicPlanningComplexTaskNeedsPlanning(t *testing.T) {
	decision, err := disabledClient(t).DecidePlanning(context.Background(), store.TaskInput{Task: "Refactor authentication architecture across multiple services"})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.NeedsPlanning {
		t.Fatalf("complex task must need planning: %+v", decision)
	}
}

// Unknown tasks fail open to planning.
func TestHeuristicPlanningUnknownTaskFailsOpen(t *testing.T) {
	decision, err := disabledClient(t).DecidePlanning(context.Background(), store.TaskInput{Task: "do the thing"})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.NeedsPlanning {
		t.Fatalf("unknown task must fail open to planning: %+v", decision)
	}
}

// Small validation-passed changes must auto-approve with default settings.
func TestHeuristicReviewSmallPassedChangeAutoApproves(t *testing.T) {
	decision, err := disabledClient(t).DecideReview(context.Background(), store.ReviewRequest{
		Validation: store.ValidationReport{Passed: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.ShouldReview || !decision.Approved {
		t.Fatalf("small passed change must auto-approve: %+v", decision)
	}
}

func TestHeuristicReviewFailedValidationNeedsReview(t *testing.T) {
	decision, err := disabledClient(t).DecideReview(context.Background(), store.ReviewRequest{
		Validation: store.ValidationReport{Passed: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.ShouldReview || decision.Approved {
		t.Fatalf("failed validation must need review: %+v", decision)
	}
}

func TestDecidePlanningLiveDirectImplement(t *testing.T) {
	c := liveClient(t, func(r *http.Request) (*http.Response, error) {
		requireAuth(t, r)
		return stubResponse(200, `{"model":"typesafe/jev-1.13","answers":{"needs_planning":{"type":"choice","choice":"direct_implement","confidence":0.95,"probabilities":{"direct_implement":0.95}}}}`), nil
	})
	decision, err := c.DecidePlanning(context.Background(), store.TaskInput{Task: "Fix typo"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.NeedsPlanning || decision.Confidence != 0.95 {
		t.Fatalf("live direct_implement must skip planning: %+v", decision)
	}
}

func TestDecideReviewLiveAutoApprove(t *testing.T) {
	c := liveClient(t, func(r *http.Request) (*http.Response, error) {
		requireAuth(t, r)
		return stubResponse(200, `{"model":"typesafe/jev-1.13","answers":{"review_routing":{"type":"choice","choice":"auto_approve","confidence":0.95}}}`), nil
	})
	decision, err := c.DecideReview(context.Background(), store.ReviewRequest{
		Validation: store.ValidationReport{Passed: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.ShouldReview || !decision.Approved {
		t.Fatalf("live auto_approve must approve: %+v", decision)
	}
}

// Low confidence fails open to planning.
func TestDecidePlanningLowConfidenceFailsOpen(t *testing.T) {
	c := liveClient(t, func(r *http.Request) (*http.Response, error) {
		return stubResponse(200, `{"model":"typesafe/jev-1.13","answers":{"needs_planning":{"type":"choice","choice":"direct_implement","confidence":0.3}}}`), nil
	})
	decision, err := c.DecidePlanning(context.Background(), store.TaskInput{Task: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.NeedsPlanning {
		t.Fatalf("low confidence must fail open to planning: %+v", decision)
	}
}

// A chat/completions wrapper body must decode: the wrapper shape unmarshals
// into the System One shape without error, so it has to be tried whenever
// direct answers are absent.
func TestDecidePlanningChatCompletionsWrapper(t *testing.T) {
	inner := `{"model":"typesafe/jev-1.13","answers":{"needs_planning":{"type":"choice","choice":"needs_planning","confidence":0.9}}}`
	content, err := json.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	c := liveClient(t, func(r *http.Request) (*http.Response, error) {
		return stubResponse(200, `{"choices":[{"message":{"content":`+string(content)+`}}]}`), nil
	})
	decision, err := c.DecidePlanning(context.Background(), store.TaskInput{Task: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.NeedsPlanning || decision.Confidence != 0.9 {
		t.Fatalf("chat wrapper must route to planning: %+v", decision)
	}
}

// Transport and protocol failures fail open to heuristic with nil error.
func TestDecidePlanningServerErrorFailsOpen(t *testing.T) {
	c := liveClient(t, func(r *http.Request) (*http.Response, error) {
		return stubResponse(500, `boom`), nil
	})
	decision, err := c.DecidePlanning(context.Background(), store.TaskInput{Task: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Reason != "heuristic fallback" {
		t.Fatalf("server error must fall back to heuristic: %+v", decision)
	}
}

func TestDecidePlanningMalformedBodyFailsOpen(t *testing.T) {
	c := liveClient(t, func(r *http.Request) (*http.Response, error) {
		return stubResponse(200, `not json`), nil
	})
	decision, err := c.DecidePlanning(context.Background(), store.TaskInput{Task: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Reason != "heuristic fallback" {
		t.Fatalf("malformed body must fall back to heuristic: %+v", decision)
	}
}
