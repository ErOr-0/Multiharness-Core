package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

const fixtureProviderSecret = "PRIVATE_PROVIDER_DIAGNOSTIC"

// Called only inside the fixture subprocess. Emitting exit-zero, pre-session
// errors exercises the provider monitor, adapter and workflow together.
func fixtureProviderFailure(stage string) (bool, error) {
	if os.Getenv("MULTIHARNESS_FIXTURE_FAILURE_STAGE") != stage {
		return false, nil
	}
	limit, err := strconv.Atoi(os.Getenv("MULTIHARNESS_FIXTURE_FAILURE_COUNT"))
	if err != nil {
		return true, err
	}
	calls, err := os.ReadFile(os.Getenv("MULTIHARNESS_FIXTURE_LOG"))
	if err != nil {
		return true, err
	}
	if strings.Count(string(calls), stage+"\n") > limit {
		return false, nil
	}
	if stage == "implement" || stage == "repair" {
		if err := os.WriteFile("result.txt", []byte("partial\n"), 0644); err != nil {
			return true, err
		}
	}
	return true, json.NewEncoder(os.Stdout).Encode(map[string]any{
		"type": "error", "error": map[string]any{
			"code":       os.Getenv("MULTIHARNESS_FIXTURE_FAILURE_CODE"),
			"statusCode": 429, "message": fixtureProviderSecret,
		},
	})
}

func TestProviderFailuresIntegration(t *testing.T) {
	for _, test := range []struct {
		name, stage, code string
		failures, calls   int
		kind              store.ProviderFailureKind // Empty means a read-only retry recovered.
	}{
		{"planning billing", "plan", "insufficient_quota", 1, 1, store.ProviderBillingExhausted},
		{"implementation billing", "implement", "insufficient_quota", 1, 1, store.ProviderBillingExhausted},
		{"review billing", "review", "insufficient_quota", 1, 1, store.ProviderBillingExhausted},
		{"repair billing", "repair", "insufficient_quota", 1, 1, store.ProviderBillingExhausted},
		{"authentication", "plan", "invalid_api_key", 1, 1, store.ProviderAuthentication},
		{"model access", "plan", "model_not_found", 1, 1, store.ProviderAccessDenied},
		{"unknown 429", "plan", "unknown_429", 1, 1, store.ProviderUnknown},
		{"planning retry", "plan", "rate_limit_exceeded", 1, 2, ""},
		{"review retry", "review", "server_is_overloaded", 1, 3, ""},
		{"retry exhaustion", "plan", "server_is_overloaded", 9, 3, store.ProviderOverloaded},
		{"implementation never replayed", "implement", "rate_limit_exceeded", 1, 1, store.ProviderRateLimited},
		{"repair never replayed", "repair", "rate_limit_exceeded", 1, 1, store.ProviderRateLimited},
	} {
		t.Run(
			test.name,
			func(t *testing.T) {
				cfg, log := fixtureConfiguration(t)
				cfg.Fallback.Mode, cfg.MaxRepairAttempts = "disabled", 1
				cfg.Execution.MaxRetries = 2
				cfg.Execution.InitialDelay, cfg.Execution.MaxDelay = config.Duration(time.Millisecond), config.Duration(5*time.Millisecond)
				t.Setenv("MULTIHARNESS_FIXTURE_FAILURE_STAGE", test.stage)
				t.Setenv("MULTIHARNESS_FIXTURE_FAILURE_CODE", test.code)
				t.Setenv("MULTIHARNESS_FIXTURE_FAILURE_COUNT", strconv.Itoa(test.failures))
				service, err := buildWorkflow(cfg, nil)
				if err != nil {
					t.Fatal(err)
				}
				output := service.Run(t.Context(), store.TaskInput{Task: "fixture change", WorkingDir: cfg.WorkingDir, MaxRepairAttempts: 1})
				if err := output.Validate(); err != nil {
					t.Fatal(err)
				}
				calls, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.Count(string(calls), test.stage+"\n"); got != test.calls {
					t.Fatalf("%s calls=%d; want %d", test.stage, got, test.calls)
				}
				if test.kind == "" {
					if output.Status != store.TaskStatusApproved {
						t.Fatalf("retry did not recover: %+v", output.Failure)
					}
				} else {
					if output.Status != store.TaskStatusFailed || output.Failure == nil || output.Failure.Provider == nil || output.Failure.Provider.Kind != test.kind || output.Failure.Provider.Attempts != test.calls {
						t.Fatalf("incorrect provider outcome: %+v", output)
					}
					if !strings.HasSuffix(string(calls), test.stage+"\n") {
						t.Fatalf("work continued after provider failure: %s", calls)
					}
					if (test.stage == "implement" || test.stage == "repair") && (len(output.Repository.ChangedFiles) != 1 || output.Repository.ChangedFiles[0] != "result.txt" || !strings.Contains(output.Repository.Diff, "+partial")) {
						t.Fatal("failed mutation lost independent partial-work evidence")
					}
				}
				data, err := json.Marshal(output)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(data), fixtureProviderSecret) || len(output.AgentSwitches) != 0 {
					t.Fatal("provider diagnostics leaked or switched without consent")
				}
				notes, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "notes.txt"))
				if err != nil || string(notes) != "user notes\n" {
					t.Fatal("provider failure altered existing user work")
				}
			},
		)
	}
}
