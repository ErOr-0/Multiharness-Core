package sessionexec

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func TestOpenCodeReadOnlyRolesUseFreshRestrictedSessions(t *testing.T) {
	for _, role := range []string{"planning", "review"} {
		t.Run(
			role,
			func(t *testing.T) {
				var lastName string
				runner := &fakeProcessRunner{run: func(_ context.Context, c process.Command) (process.Result, error) {
					if slices.Contains(c.Args, "--session") || slices.Contains(c.Args, "--auto") || !slices.Contains(c.Args, "--pure") {
						t.Fatal("unsafe read-only command")
					}
					i := slices.Index(c.Args, "--agent")
					if i < 0 {
						t.Fatal("no restricted agent")
					}
					name := c.Args[i+1]
					if !strings.HasPrefix(name, "multiharness-readonly-") || name == lastName {
						t.Fatal("reused agent configuration")
					}
					lastName = name
					var settings struct {
						Agent map[string]struct{ Permission map[string]string }
					}
					if json.Unmarshal([]byte(c.EnvOverrides["OPENCODE_CONFIG_CONTENT"]), &settings) != nil || settings.Agent[name].Permission["*"] != "deny" || settings.Agent[name].Permission["read"] != "allow" {
						t.Fatal("missing deny-by-default policy")
					}
					response := `{"schema_version":"2","action":"answer","answer":"explanation","summary":"answer","steps":[],"acceptance_criteria":[]}`
					if role == "review" {
						response = `{"schema_version":"1","approved":true,"summary":"approved","findings":[],"suggestions":[]}`
					}
					line, _ := json.Marshal(map[string]any{"type": "text", "sessionID": "new-session", "part": map[string]string{"type": "text", "text": response}})
					writeOutput(t, c, string(line)+"\n")
					return process.Result{}, nil
				}}
				a, err := NewReadOnlyAgent(runner, DefaultConfig())
				if err != nil {
					t.Fatal(err)
				}
				for range 2 {
					if role == "planning" {
						_, err = a.Plan(t.Context(), validImplementationRequest(t).Input)
					} else {
						r := validRepairRequest(t)
						_, err = a.Review(
							t.Context(),
							store.ReviewRequest{Input: r.Input, Plan: r.Plan, Implementation: r.Implementation, Validation: r.Validation},
						)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
			},
		)
	}
}

func TestOpenCodeReadOnlyFailsClosed(t *testing.T) {
	t.Setenv("OPENCODE_CONFIG_CONTENT", "")
	runner := &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		return process.Result{}, context.Canceled
	}}
	if _, err := NewReadOnlyAgent(nil, DefaultConfig()); err == nil {
		t.Fatal("nil runner accepted")
	}
	for _, cfg := range []Config{
		{PermissionPolicy: PermissionAutoApprove},
		{ExtraArgs: []string{"--pure=false"}},
		{ExtraArgs: []string{"--agent=build"}},
	} {
		if _, err := NewReadOnlyAgent(runner, cfg); err == nil {
			t.Fatal("unsafe config accepted")
		}
	}
	a, _ := NewReadOnlyAgent(runner, DefaultConfig())
	if _, err := a.Plan(nil, validImplementationRequest(t).Input); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := a.Plan(t.Context(), validImplementationRequest(t).Input); !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation")
	}
	for _, line := range []string{
		`{"type":"step_finish","sessionID":"s","part":{"type":"step-finish"}}`,
		`{"type":"text","sessionID":"s","part":{"type":"text","text":"not json"}}`,
		`{"type":"error","error":{"code":"insufficient_quota"}}`,
	} {
		runner.run = func(_ context.Context, c process.Command) (process.Result, error) {
			writeOutput(t, c, line+"\n")
			return process.Result{}, nil
		}
		if _, err := a.Plan(t.Context(), validImplementationRequest(t).Input); err == nil {
			t.Fatal("invalid output accepted")
		}
	}
}

func TestReadOnlyOverlayPreservesInheritedConfigWithoutLeakingErrors(t *testing.T) {
	const original = `{"model":"provider/model","provider":{"token":"synthetic-secret"},"agent":{"existing":{"mode":"primary"}}}`
	data, err := readOnlyConfig(original, "fresh-agent")
	if err != nil {
		t.Fatal(err)
	}
	var merged map[string]json.RawMessage
	_ = json.Unmarshal(data, &merged)
	if !strings.Contains(string(merged["provider"]), "synthetic-secret") || !strings.Contains(string(merged["agent"]), "existing") || !strings.Contains(string(merged["agent"]), "fresh-agent") {
		t.Fatal("inherited settings were discarded")
	}
	for _, raw := range []string{`null`, `{synthetic-secret`, `{"agent":null}`, `{"agent":"synthetic-secret"}`} {
		if _, err := readOnlyConfig(raw, "fresh"); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("unsafe inline configuration handling")
		}
	}
}
