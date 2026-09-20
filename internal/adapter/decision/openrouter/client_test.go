package openrouter

import (
	"context"
	"encoding/json"
	"errors"
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

// Without a valid Jev decision, even a simple task must retain assessment.
func TestUnavailableRoutingNeverSkipsPlanning(t *testing.T) {
	decision, err := disabledClient(t).DecidePlanning(context.Background(), store.TaskInput{Task: "Fix typo in README"})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.NeedsPlanning {
		t.Fatalf("unavailable routing must preserve assessment: %+v", decision)
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

// Missing credentials must preserve independent review.
func TestReviewWithoutKeyRequiresFullReview(t *testing.T) {
	c := disabledClient(t)
	c.cfg.Enabled = true
	decision, err := c.DecideReview(context.Background(), store.ReviewRequest{
		Validation: passedChecks(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.ShouldReview || decision.Approved {
		t.Fatalf("missing key must require review: %+v", decision)
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
		return stubResponse(200, `{"model":"typesafe/jev-1.13","answers":{"task_routing":{"type":"choice","choice":"direct_implement","confidence":0.95,"probabilities":{"direct_implement":0.95}}}}`), nil
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
		Validation: passedChecks(),
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
		return stubResponse(200, `{"model":"typesafe/jev-1.13","answers":{"task_routing":{"type":"choice","choice":"direct_implement","confidence":0.3}}}`), nil
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
	inner := `{"model":"typesafe/jev-1.13","answers":{"task_routing":{"type":"choice","choice":"needs_planning","confidence":0.9}}}`
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

// Transport and protocol failures retain assessment with a visible fallback.
func TestDecidePlanningServerErrorFailsOpen(t *testing.T) {
	c := liveClient(t, func(r *http.Request) (*http.Response, error) {
		return stubResponse(500, `boom`), nil
	})
	decision, err := c.DecidePlanning(context.Background(), store.TaskInput{Task: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Source != store.DecisionFallback {
		t.Fatalf("server error must fall back to assessment: %+v", decision)
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
	if decision.Source != store.DecisionFallback {
		t.Fatalf("malformed body must fall back to assessment: %+v", decision)
	}
}

func passedChecks() store.ValidationReport {
	return store.ValidationReport{Passed: true, Checks: []store.ValidationEvidence{{Command: "go test ./...", Passed: true}}}
}

func TestDecisionRequestUsesOpenRouterDecisionsContract(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.APIKey = "test-key"
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.httpClient.Transport = stubRoundTripper{handler: func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://openrouter.ai/api/alpha/decisions" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		requireAuth(t, r)
		var body struct {
			Model     string
			State     json.RawMessage
			Questions map[string]json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != cfg.Model || len(body.State) == 0 || body.Questions["task_routing"] == nil {
			t.Fatalf("invalid decision body: %+v", body)
		}
		return stubResponse(200, `{"answers":{"task_routing":{"choice":"direct_implement","confidence":0.95}}}`), nil
	}}
	decision, err := c.DecidePlanning(t.Context(), store.TaskInput{Task: "Fix typo"})
	if err != nil || decision.NeedsPlanning {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
}

func TestInvalidChoicesCannotSkipStages(t *testing.T) {
	for _, answer := range []string{
		`{"choice":"unexpected","confidence":0.99}`,
		`{"confidence":0.99}`,
		`{"choice":null,"confidence":0.99}`,
		`{"choice":"SKIP","confidence":2}`,
		`{"choice":"SKIP","confidence":-1}`,
		`{"choice":"SKIP"}`,
		`{"choice":"SKIP","confidence":null}`,
	} {
		t.Run(answer, func(t *testing.T) {
			for _, threshold := range []float64{0, 0.75} {
				c := liveClient(t, func(r *http.Request) (*http.Response, error) {
					return stubResponse(200, `{"answers":{"task_routing":`+strings.ReplaceAll(answer, "SKIP", "direct_implement")+`,"review_routing":`+strings.ReplaceAll(answer, "SKIP", "auto_approve")+`}}`), nil
				})
				c.cfg.ConfidenceThreshold = threshold
				planning, err := c.DecidePlanning(t.Context(), store.TaskInput{Task: "Fix typo"})
				if err != nil || !planning.NeedsPlanning {
					t.Fatalf("invalid answer skipped planning: %+v, %v", planning, err)
				}
				review, err := c.DecideReview(t.Context(), store.ReviewRequest{Validation: passedChecks()})
				if err != nil || !review.ShouldReview || review.Approved {
					t.Fatalf("invalid answer skipped review: %+v, %v", review, err)
				}
			}
		})
	}
}

func TestReviewFailuresPreserveFullReview(t *testing.T) {
	for _, failure := range []string{"transport", "http", "malformed", "missing answers", "low confidence"} {
		t.Run(failure, func(t *testing.T) {
			c := liveClient(t, func(r *http.Request) (*http.Response, error) {
				switch failure {
				case "transport":
					return nil, errors.New("offline")
				case "http":
					return stubResponse(500, "unavailable"), nil
				case "malformed":
					return stubResponse(200, "not json"), nil
				case "missing answers":
					return stubResponse(200, `{}`), nil
				default:
					return stubResponse(200, `{"answers":{"review_routing":{"choice":"auto_approve","confidence":0.2}}}`), nil
				}
			})
			decision, err := c.DecideReview(t.Context(), store.ReviewRequest{Validation: passedChecks()})
			if err != nil || !decision.ShouldReview || decision.Approved {
				t.Fatalf("failure bypassed review: %+v, %v", decision, err)
			}
		})
	}
}

func TestReviewWithoutChecksPreservesFullReview(t *testing.T) {
	c := liveClient(t, func(r *http.Request) (*http.Response, error) {
		t.Fatal("must not request auto-approval without checks")
		return nil, errors.New("unexpected request")
	})
	decision, err := c.DecideReview(t.Context(), store.ReviewRequest{Validation: store.ValidationReport{Passed: true}})
	if err != nil || !decision.ShouldReview || decision.Approved {
		t.Fatalf("empty checks bypassed review: %+v, %v", decision, err)
	}
}

func TestThreeWayRoutingContract(t *testing.T) {
	for _, route := range []store.TaskRoute{store.RouteAnswer, store.RoutePlan, store.RouteImplement} {
		t.Run(string(route), func(t *testing.T) {
			c := liveClient(t, func(r *http.Request) (*http.Response, error) {
				var body struct {
					State     string
					Questions map[string]struct{ Criteria map[string]string }
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				criteria := body.Questions["task_routing"].Criteria
				if len(criteria) != 3 || criteria["answer"] == "" || criteria["needs_planning"] == "" || criteria["direct_implement"] == "" || body.State != "Does this README need a typo fix?" {
					t.Fatal("wrong classification contract", body)
				}
				return stubResponse(200, `{"answers":{"task_routing":{"choice":"`+string(route)+`","confidence":0.95}}}`), nil
			})
			d, err := c.DecidePlanning(t.Context(), store.TaskInput{Task: "Does this README need a typo fix?"})
			if err != nil || d.Validate() != nil || d.Route != route || d.Source != store.DecisionJev {
				t.Fatal(d, err)
			}
		})
	}
}

func TestFailuresNeverUseKeywordShortcuts(t *testing.T) {
	for _, task := range []string{"Explain the README", "Should we fix this typo?", "Fix typo"} {
		for _, body := range []string{`not json`, `{"answers":{"task_routing":{"choice":"direct_implement","confidence":0.2}}}`, `{"answers":{"task_routing":{"choice":"answer","confidence":0.2}}}`, `{"answers":{"needs_planning":{"choice":"direct_implement","confidence":0.99}}}`} {
			c := liveClient(t, func(*http.Request) (*http.Response, error) { return stubResponse(200, body), nil })
			d, err := c.DecidePlanning(t.Context(), store.TaskInput{Task: task})
			if err != nil || d.Route != store.RoutePlan || d.Source != store.DecisionFallback || d.Validate() != nil {
				t.Fatal(task, d, err)
			}
		}
	}
}
