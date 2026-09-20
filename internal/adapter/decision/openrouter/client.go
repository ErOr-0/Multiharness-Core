package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"multiharness-core/internal/store"
)

// Config for Jev via the user's own OpenRouter key. No provider token is
// bundled: the operator configures decision.model/endpoint and exports
// OPENROUTER_API_KEY.
type Config struct {
	Enabled             bool
	Model               string
	Endpoint            string
	Timeout             time.Duration
	ConfidenceThreshold float64
	APIKey              string
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("decision model must not be blank")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("decision endpoint must not be blank")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("decision timeout must be positive")
	}
	if math.IsNaN(c.ConfidenceThreshold) || c.ConfidenceThreshold < 0 || c.ConfidenceThreshold > 1 {
		return fmt.Errorf("decision confidence_threshold must be between 0 and 1")
	}
	return nil
}

func DefaultConfig() Config {
	return Config{
		Enabled:             false,
		Model:               "typesafe/jev-1.13",
		Endpoint:            "https://openrouter.ai/api/alpha/decisions",
		Timeout:             10 * time.Second,
		ConfidenceThreshold: 0.75,
		APIKey:              "",
	}
}

// Client implements workflow.DecisionMaker via Jev System One.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// An enabled client without a key is allowed; Decide calls fall back to
	// planning heuristics and full review until a key is provided.
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: http.DefaultTransport,
			Timeout:   cfg.Timeout,
		},
	}, nil
}

// Only the decoded fields below are read; the rest of the provider shape is
// ignored. A chat/completions wrapper is also accepted on decode.
type choiceAnswer struct {
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type systemOneResponse struct {
	Answers map[string]json.RawMessage `json:"answers"`
}

func (a choiceAnswer) describe() string {
	return fmt.Sprintf("jev choice=%s confidence=%.2f", a.Choice, a.Confidence)
}

// lookupChoice extracts one choice answer, backfilling a degenerate
// distribution when the provider omits probabilities.
func lookupChoice(answers map[string]json.RawMessage, key string, allowed ...string) (choiceAnswer, bool) {
	raw, ok := answers[key]
	if !ok {
		return choiceAnswer{}, false
	}
	var ans choiceAnswer
	if err := json.Unmarshal(raw, &ans); err != nil {
		return choiceAnswer{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields["confidence"] == nil || string(fields["confidence"]) == "null" {
		return choiceAnswer{}, false
	}
	validChoice := false
	for _, choice := range allowed {
		validChoice = validChoice || ans.Choice == choice
	}
	if !validChoice || math.IsNaN(ans.Confidence) || ans.Confidence < 0 || ans.Confidence > 1 {
		return choiceAnswer{}, false
	}
	if ans.Probabilities == nil {
		ans.Probabilities = map[string]float64{ans.Choice: 1.0}
	}
	return ans, true
}

// failOpen keeps a positive routing verdict, but any verdict below the
// confidence threshold fails open to true (plan, or review).
func failOpen(positive bool, confidence, threshold float64) bool {
	return positive || confidence < threshold
}

func containsAny(task string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(task, kw) {
			return true
		}
	}
	return false
}

// DecidePlanning routes whether planning is required. Fail-open to needs planning.
func (c *Client) DecidePlanning(ctx context.Context, input store.TaskInput) (store.PlanningDecision, error) {
	if !c.cfg.Enabled || strings.TrimSpace(c.cfg.APIKey) == "" {
		return heuristicPlanning(input, c.cfg), nil
	}
	state := input.Task
	questions := map[string]any{
		"needs_planning": map[string]any{
			"type":         "choice",
			"instructions": "Does this task require a planning phase before implementation? Consider complexity, file count, architecture impact.",
			"criteria": map[string]any{
				"needs_planning":   "Complex multi-file change, new feature, architecture, refactor, design document or API contract needed",
				"direct_implement": "Simple single-file fix, typo, small bug, docs, copy change, no design needed",
			},
		},
	}
	answers, err := c.callSystemOne(ctx, state, questions)
	if err != nil {
		return heuristicPlanning(input, c.cfg), nil
	}
	ans, ok := lookupChoice(answers, "needs_planning", "needs_planning", "direct_implement")
	if !ok {
		return store.PlanningDecision{NeedsPlanning: true, Reason: "invalid planning decision"}, nil
	}
	needsPlanning := failOpen(ans.Choice == "needs_planning", ans.Confidence, c.cfg.ConfidenceThreshold)
	return store.PlanningDecision{
		NeedsPlanning: needsPlanning,
		Confidence:    ans.Confidence,
		Reason:        ans.describe(),
		Model:         c.cfg.Model,
		Probabilities: ans.Probabilities,
	}, nil
}

// DecideReview routes whether full review is required and provides verdict when skipping.
func (c *Client) DecideReview(ctx context.Context, req store.ReviewRequest) (store.ReviewDecision, error) {
	if !c.cfg.Enabled || strings.TrimSpace(c.cfg.APIKey) == "" || !req.Validation.Passed || len(req.Validation.Checks) == 0 {
		return fullReviewDecision(c.cfg), nil
	}
	// Build state as structured object for Jev
	state := map[string]any{
		"task":              req.Input.Task,
		"plan_summary":      req.Plan.Summary,
		"changed_files":     req.Implementation.ChangedFiles,
		"validation_passed": req.Validation.Passed,
		"validation_checks": len(req.Validation.Checks),
	}
	// Include truncated diff summary if available
	if req.Repository != nil {
		state["file_count"] = len(req.Repository.ChangedFiles)
	}
	questions := map[string]any{
		"review_routing": map[string]any{
			"type":         "choice",
			"instructions": "Should this implementation be sent for full code review or auto-approved? Auto-approve only small, low-risk, validation-passed changes.",
			"criteria": map[string]any{
				"needs_full_review": "Complex, multi-file, validation failed or risky change that needs human/expert review",
				"auto_approve":      "Small low-risk change, validation passed, clearly correct, no review needed",
			},
		},
	}
	answers, err := c.callSystemOne(ctx, state, questions)
	if err != nil {
		return fullReviewDecision(c.cfg), nil
	}
	ans, ok := lookupChoice(answers, "review_routing", "needs_full_review", "auto_approve")
	if !ok {
		return fullReviewDecision(c.cfg), nil
	}
	shouldReview := failOpen(ans.Choice == "needs_full_review", ans.Confidence, c.cfg.ConfidenceThreshold)
	approved := !shouldReview && req.Validation.Passed
	return store.ReviewDecision{
		ShouldReview:  shouldReview,
		Approved:      approved,
		Confidence:    ans.Confidence,
		Reason:        ans.describe(),
		Model:         c.cfg.Model,
		Probabilities: ans.Probabilities,
	}, nil
}

func (c *Client) callSystemOne(ctx context.Context, state any, questions map[string]any) (map[string]json.RawMessage, error) {
	// The System One shape is sent as-is; OpenRouter forwards it to the Jev
	// provider. A custom decision.endpoint may serve the same shape directly.
	respData, err := c.postJSON(ctx, map[string]any{
		"model":     c.cfg.Model,
		"state":     state,
		"questions": questions,
	})
	if err != nil {
		return nil, err
	}
	return decodeAnswers(respData)
}

// postJSON sends one decision request and returns the raw body of a
// successful response.
func (c *Client) postJSON(ctx context.Context, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	// OpenRouter recommended headers
	req.Header.Set("HTTP-Referer", "https://github.com/ErOr-0/Multiharness-Core")
	req.Header.Set("X-Title", "Multiharness Core")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jev http %d: %s", resp.StatusCode, string(respData))
	}
	return respData, nil
}

// decodeAnswers accepts the System One shape directly or inside a
// chat/completions wrapper. A chat body decodes into systemOneResponse
// without error (Answers nil), so the wrapper is tried whenever direct
// answers are absent.
func decodeAnswers(respData []byte) (map[string]json.RawMessage, error) {
	var parsed systemOneResponse
	unmarshalErr := json.Unmarshal(respData, &parsed)
	if unmarshalErr == nil && parsed.Answers != nil {
		return parsed.Answers, nil
	}
	var wrapper struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respData, &wrapper); err == nil && len(wrapper.Choices) > 0 {
		var inner systemOneResponse
		if err := json.Unmarshal([]byte(wrapper.Choices[0].Message.Content), &inner); err == nil && inner.Answers != nil {
			return inner.Answers, nil
		}
	}
	if unmarshalErr != nil {
		return nil, fmt.Errorf("decode jev response: %w body=%s", unmarshalErr, string(respData))
	}
	return nil, fmt.Errorf("jev response missing answers: %s", string(respData))
}

// Heuristic fallbacks
func heuristicPlanning(input store.TaskInput, cfg Config) store.PlanningDecision {
	task := strings.ToLower(input.Task)
	needs := false
	confidence := 0.6
	// A simple signal wins over a complex one. Length is only a signal when
	// no keyword matched, so short simple tasks keep their confidence.
	complexKeywords := []string{"refactor", "architect", "design", "feature", "endpoint", "migration", "multi", "system", "implement", "workflow", "integration"}
	simpleKeywords := []string{"typo", "fix typo", "docs", "readme", "comment", "rename", "small bug", "one line"}
	simple := containsAny(task, simpleKeywords)
	complex := containsAny(task, complexKeywords)
	switch {
	case simple:
		confidence = 0.8
	case complex:
		needs = true
		confidence = 0.85
	case len(task) > 500:
		needs = true
		confidence = 0.7
	case len(task) < 50:
		confidence = 0.65
	}
	needs = failOpen(needs, confidence, cfg.ConfidenceThreshold)
	probabilities := map[string]float64{
		"needs_planning":   1 - confidence,
		"direct_implement": confidence,
	}
	if needs {
		probabilities["needs_planning"] = confidence
		probabilities["direct_implement"] = 1 - confidence
	}
	return store.PlanningDecision{
		NeedsPlanning: needs,
		Confidence:    confidence,
		Reason:        "heuristic fallback",
		Model:         cfg.Model + "(heuristic)",
		Probabilities: probabilities,
	}
}

// Unavailable or invalid routing must preserve independent review.
func fullReviewDecision(cfg Config) store.ReviewDecision {
	return store.ReviewDecision{
		ShouldReview: true,
		Reason:       "full review fallback",
		Model:        cfg.Model,
	}
}
