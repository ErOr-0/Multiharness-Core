package route

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"multiharness-core/internal/cli"
	"multiharness-core/internal/testkit"
)

func route(t *testing.T, args ...string) Decision {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := CmdRoute(args, strings.NewReader(""), &out, &errOut); code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var d Decision
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	return d
}

func TestRouteLaya(t *testing.T) {
	srv, seen := testkit.DecisionServer(t, 200, `{"answers":{"task_routing":{"choice":"direct_implement","confidence":0.92,"probabilities":{"direct_implement":0.92,"answer":0.05,"bogus":0.03}}}}`)
	t.Setenv("LAYA_ENDPOINT", srv.URL)
	t.Setenv("LAYA_API_KEY", "")
	d := route(t, "--provider", "laya", "--prompt", "fix the typo in README.md")
	if d.Route != "direct_implement" || d.Confidence != 0.92 || d.Fallback != "" || d.Model != "laya" || len(d.Probabilities) != 2 {
		t.Fatalf("%+v", d)
	}
	if seen.Auth != "" || seen.Body["state"] != "fix the typo in README.md" || seen.Body["model"] != "laya" {
		t.Fatalf("request %+v", seen)
	}
	if q, _ := seen.Body["questions"].(map[string]any); q["task_routing"] == nil {
		t.Fatalf("questions %v", seen.Body["questions"])
	}
}

func TestRouteJevReview(t *testing.T) {
	srv, seen := testkit.DecisionServer(t, 200, `{"choices":[{"message":{"content":"{\"answers\":{\"review_routing\":{\"choice\":\"auto_approve\",\"confidence\":0.9}}}"}}]}`)
	t.Setenv("OPENROUTER_API_KEY", "sk-test")
	d := route(t, "--provider", "jev", "--endpoint", srv.URL, "--review", "--prompt", "1 file, tests pass")
	if d.Route != "auto_approve" || d.Question != "review" || d.Model != jevModel {
		t.Fatalf("%+v", d)
	}
	if seen.Auth != "Bearer sk-test" || seen.Title != "harness-bridge" {
		t.Fatalf("headers %+v", seen)
	}
}

func TestRouteFallsBackSafely(t *testing.T) {
	for _, tc := range []struct {
		name, reply, fallback string
		status                int
	}{
		{"low confidence", `{"answers":{"task_routing":{"choice":"answer","confidence":0.4}}}`, "low_confidence", 200},
		{"server error", `boom`, "unavailable", 500},
		{"unknown choice", `{"answers":{"task_routing":{"choice":"yolo","confidence":0.99}}}`, "invalid_response", 200},
		{"no confidence", `{"answers":{"task_routing":{"choice":"answer"}}}`, "invalid_response", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := testkit.DecisionServer(t, tc.status, tc.reply)
			d := route(t, "--provider", "laya", "--endpoint", srv.URL, "--prompt", "x")
			if d.Route != "needs_planning" || d.Fallback != tc.fallback || d.Error == "" {
				t.Fatalf("%+v", d)
			}
		})
	}
	t.Setenv("OPENROUTER_API_KEY", "")
	if d := route(t, "--provider", "jev", "--review", "--prompt", "x"); d.Route != "needs_full_review" || d.Error != "OPENROUTER_API_KEY is not set" {
		t.Fatalf("%+v", d)
	}
}

func TestRouteRejects(t *testing.T) {
	t.Setenv("HARNESS_BRIDGE_ROUTER", "")
	for _, args := range [][]string{{"--prompt", "x"}, {"--provider", "gpt", "--prompt", "x"}, {"--provider", "laya", "--threshold", "2", "--prompt", "x"}} {
		if _, _, err := parseRoute(args, strings.NewReader("")); err == nil {
			t.Errorf("%q was accepted", args)
		}
	}
}
