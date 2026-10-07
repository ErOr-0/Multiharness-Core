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

	"multiharness-core/internal/contract"
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
	decision, err := disabledClient(t).DecidePlanning(context.Background(), contract.TaskInput{Task: "Fix typo in README"})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.NeedsPlanning {
		t.Fatalf("unavailable routing must preserve assessment: %+v", decision)
	}
}

func TestHeuristicPlanningComplexTaskNeedsPlanning(t *testing.T) {
	decision, err := disabledClient(t).DecidePlanning(context.Background(), contract.TaskInput{Task: "Refactor authentication architecture across multiple services"})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.NeedsPlanning {
		t.Fatalf("complex task must need planning: %+v", decision)
	}
}

// Unknown tasks fail open to planning.
func TestHeuristicPlanningUnknownTaskFailsOpen(t *testing.T) {
	decision, err := disabledClient(t).DecidePlanning(context.Background(), contract.TaskInput{Task: "do the thing"})
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
	decision, err := c.DecideReview(context.Background(), contract.ReviewRequest{
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
	decision, err := disabledClient(t).DecideReview(context.Background(), contract.ReviewRequest{
		Validation: contract.ValidationReport{Passed: false},
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
	decision, err := c.DecidePlanning(context.Background(), contract.TaskInput{Task: "Fix typo"})
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
	decision, err := c.DecideReview(context.Background(), contract.ReviewRequest{
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
	decision, err := c.DecidePlanning(context.Background(), contract.TaskInput{Task: "x"})
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
	decision, err := c.DecidePlanning(context.Background(), contract.TaskInput{Task: "x"})
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
	decision, err := c.DecidePlanning(context.Background(), contract.TaskInput{Task: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Source != contract.DecisionFallback {
		t.Fatalf("server error must fall back to assessment: %+v", decision)
	}
}

func TestDecidePlanningMalformedBodyFailsOpen(t *testing.T) {
	c := liveClient(t, func(r *http.Request) (*http.Response, error) {
		return stubResponse(200, `not json`), nil
	})
	decision, err := c.DecidePlanning(context.Background(), contract.TaskInput{Task: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Source != contract.DecisionFallback {
		t.Fatalf("malformed body must fall back to assessment: %+v", decision)
	}
}

func passedChecks() contract.ValidationReport {
	return contract.ValidationReport{Passed: true, Checks: []contract.ValidationEvidence{{Command: "go test ./...", Passed: true}}}
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
	decision, err := c.DecidePlanning(t.Context(), contract.TaskInput{Task: "Fix typo"})
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
				planning, err := c.DecidePlanning(t.Context(), contract.TaskInput{Task: "Fix typo"})
				if err != nil || !planning.NeedsPlanning {
					t.Fatalf("invalid answer skipped planning: %+v, %v", planning, err)
				}
				review, err := c.DecideReview(t.Context(), contract.ReviewRequest{Validation: passedChecks()})
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
			decision, err := c.DecideReview(t.Context(), contract.ReviewRequest{Validation: passedChecks()})
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
	decision, err := c.DecideReview(t.Context(), contract.ReviewRequest{Validation: contract.ValidationReport{Passed: true}})
	if err != nil || !decision.ShouldReview || decision.Approved {
		t.Fatalf("empty checks bypassed review: %+v, %v", decision, err)
	}
}

func TestThreeWayRoutingContract(t *testing.T) {
	for _, route := range []contract.TaskRoute{contract.RouteAnswer, contract.RoutePlan, contract.RouteImplement} {
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
			d, err := c.DecidePlanning(t.Context(), contract.TaskInput{Task: "Does this README need a typo fix?"})
			if err != nil || d.Validate() != nil || d.Route != route || d.Source != contract.DecisionJev {
				t.Fatal(d, err)
			}
		})
	}
}

func TestFailuresNeverUseKeywordShortcuts(t *testing.T) {
	for _, task := range []string{"Explain the README", "Should we fix this typo?", "Fix typo"} {
		for _, body := range []string{`not json`, `{"answers":{"task_routing":{"choice":"direct_implement","confidence":0.2}}}`, `{"answers":{"task_routing":{"choice":"answer","confidence":0.2}}}`, `{"answers":{"needs_planning":{"choice":"direct_implement","confidence":0.99}}}`} {
			c := liveClient(t, func(*http.Request) (*http.Response, error) { return stubResponse(200, body), nil })
			d, err := c.DecidePlanning(t.Context(), contract.TaskInput{Task: task})
			if err != nil || d.Route != contract.RoutePlan || d.Source != contract.DecisionFallback || d.Validate() != nil {
				t.Fatal(task, d, err)
			}
		}
	}
}

func layaClient(t *testing.T, key string, handler func(*http.Request) (*http.Response, error)) *Client {
	t.Helper()
	model, endpoint, ok := ProviderDefaults(ProviderLaya)
	if !ok {
		t.Fatal("laya provider defaults missing")
	}
	c, err := NewClient(Config{
		Enabled:             true,
		Provider:            ProviderLaya,
		Model:               model,
		Endpoint:            endpoint,
		Timeout:             5 * time.Second,
		ConfidenceThreshold: 0.75,
		APIKey:              key,
	})
	if err != nil {
		t.Fatal(err)
	}
	c.httpClient = &http.Client{Transport: stubRoundTripper{handler: handler}}
	return c
}

// A self-hosted Laya server may run open: no key must still send the request,
// without an empty bearer token or OpenRouter attribution headers.
func TestLayaRunsWithoutKeyAgainstSelfHostedServer(t *testing.T) {
	for _, key := range []string{"", "laya-key"} {
		t.Run("key="+key, func(t *testing.T) {
			c := layaClient(t, key, func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "http://127.0.0.1:8765/v1/systemone" {
					t.Fatal("wrong endpoint", r.URL)
				}
				if r.Header.Get("HTTP-Referer") != "" || r.Header.Get("X-Title") != "" {
					t.Fatal("OpenRouter headers sent to a self-hosted server")
				}
				want := ""
				if key != "" {
					want = "Bearer " + key
				}
				if _, present := r.Header["Authorization"]; r.Header.Get("Authorization") != want || (key == "" && present) {
					t.Fatalf("authorization %q", r.Header.Get("Authorization"))
				}
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "laya" {
					t.Fatal("model not forwarded", body, err)
				}
				return stubResponse(200, `{"model":"laya","answers":{"task_routing":{"type":"choice","choice":"direct_implement","confidence":0.93,"probabilities":{"direct_implement":0.93,"answer":0.05,"needs_planning":0.02}}}}`), nil
			})
			d, err := c.DecidePlanning(t.Context(), contract.TaskInput{Task: "Change teh to the in README.md"})
			if err != nil || d.Validate() != nil || d.Route != contract.RouteImplement || d.Source != contract.DecisionLaya || d.Model != "laya" || !strings.HasPrefix(d.Reason, "laya choice=") {
				t.Fatal(d, err)
			}
		})
	}
}

// Jev still refuses to run without a key; Laya is the only open provider.
func TestProviderKeyRequirementAndValidation(t *testing.T) {
	jev := DefaultConfig()
	jev.Enabled = true
	if !jev.RequiresKey() || jev.Source() != contract.DecisionJev {
		t.Fatal("jev must require a key")
	}
	laya := jev
	laya.Provider = ProviderLaya
	if laya.RequiresKey() || laya.Source() != contract.DecisionLaya {
		t.Fatal("laya must run without a key")
	}
	unknown := jev
	unknown.Provider = "other"
	if err := unknown.Validate(); err == nil || !strings.Contains(err.Error(), "jev or laya") {
		t.Fatal("unknown provider accepted", err)
	}
	if ProviderName(ProviderJev) != "Jev" || ProviderName(ProviderLaya) != "Laya" {
		t.Fatal("provider names")
	}
	c := layaClient(t, "", func(*http.Request) (*http.Response, error) {
		return stubResponse(401, `{"error":"missing key"}`), nil
	})
	d, err := c.DecidePlanning(t.Context(), contract.TaskInput{Task: "Explain the README"})
	if err != nil || d.Source != contract.DecisionFallback || d.Fallback != contract.RoutingUnavailable || d.Validate() != nil {
		t.Fatal("rejected laya request must fall back to assessment", d, err)
	}
}

func TestLayaDefaultEndpointCanBeOverriddenForContainers(t *testing.T) {
	_, original, _ := ProviderDefaults(ProviderLaya)
	t.Cleanup(func() { layaDefaultEndpoint = original })
	SetLayaDefaultEndpoint("  ")
	if _, endpoint, _ := ProviderDefaults(ProviderLaya); endpoint != original {
		t.Fatal("blank override changed the default", endpoint)
	}
	SetLayaDefaultEndpoint("http://host.docker.internal:8765/v1/systemone")
	if _, endpoint, _ := ProviderDefaults(ProviderLaya); endpoint != "http://host.docker.internal:8765/v1/systemone" {
		t.Fatal(endpoint)
	}
	if _, endpoint, _ := ProviderDefaults(ProviderJev); endpoint != "https://openrouter.ai/api/alpha/decisions" {
		t.Fatal("jev default must not change", endpoint)
	}
	if OtherProvider(ProviderJev) != ProviderLaya || OtherProvider(ProviderLaya) != ProviderJev {
		t.Fatal("other provider")
	}
}
