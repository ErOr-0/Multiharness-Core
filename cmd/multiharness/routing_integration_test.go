package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// Production composition, HTTP adapter, real process transport, structured
// prompts/parsers and workspace guard; only model responses are fixtures.
func TestThreeWayRoutingIntegration(t *testing.T) {
	for _, tc := range []struct {
		route       store.TaskRoute
		task, calls string
		status      store.TaskStatus
	}{
		{store.RouteAnswer, "fixture answer", "plan\n", store.TaskStatusAnswered},
		{store.RoutePlan, "fixture immediate", "plan\nimplement\ncheck\nreview\n", store.TaskStatusApproved},
		{store.RouteImplement, "fixture immediate", "implement\ncheck\nreview\n", store.TaskStatusApproved},
	} {
		t.Run(string(tc.route), func(t *testing.T) {
			cfg, log := fixtureConfiguration(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Questions map[string]json.RawMessage }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if _, ok := body.Questions["task_routing"]; ok {
					if data, _ := os.ReadFile(log); len(data) != 0 {
						t.Error("agent started before Jev routing")
					}
					fmt.Fprintf(w, `{"answers":{"task_routing":{"choice":%q,"confidence":0.99}}}`, tc.route)
				} else {
					fmt.Fprint(w, `{"answers":{"review_routing":{"choice":"needs_full_review","confidence":0.99}}}`)
				}
			}))
			defer srv.Close()
			cfg.Decision.Enabled = true
			cfg.Decision.Endpoint = srv.URL
			deps, err := buildDependenciesWithDecisionKey(cfg, nil, nil, nil, "fixture-key")
			if err != nil {
				t.Fatal(err)
			}
			service, err := workflow.NewService(deps)
			if err != nil {
				t.Fatal(err)
			}
			out := service.Run(t.Context(), store.TaskInput{Task: tc.task, WorkingDir: cfg.WorkingDir})
			if out.Status != tc.status || out.Routing == nil || out.Routing.Route != tc.route {
				t.Fatalf("wrong result: %+v", out)
			}
			if err := out.Validate(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(log)
			if err != nil || string(data) != tc.calls {
				t.Fatalf("wrong calls: %s %v", data, err)
			}
			if tc.route == store.RouteAnswer {
				data, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "result.txt"))
				if err != nil || string(data) != "before\n" || out.Repository != nil {
					t.Fatal("question modified/acquired workspace", out, err)
				}
				if strings.Contains(string(data), "fixed") {
					t.Fatal("question ran coding agent")
				}
			}
		})
	}
}
