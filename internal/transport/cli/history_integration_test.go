package cli_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func TestPlanOnlySurvivesRestartAndReachesNextAgent(t *testing.T) {
	workspace, settings := t.TempDir(), filepath.Join(t.TempDir(), "magent", "config.json")
	var stdout, stderr bytes.Buffer
	planID := ""
	first := newTeamHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(_ context.Context, input store.TaskInput) store.TaskOutput {
			if !input.PlanOnly || input.SelectedPlan != nil || len(input.RecentTurns) != 0 {
				t.Fatalf("plan-only input: %+v", input)
			}
			planID = input.PlanArtifactID
			plan := store.Plan{ID: planID, CaseID: input.CaseArtifactID, Version: 1, Action: store.PlanActionPropose, Title: "Invoice export", Tags: []string{"invoice", "export"}, Summary: "Add a tenant-scoped export", HandoffContext: []string{"Existing endpoint in api.go"}, Steps: []string{"Add the export"}, AcceptanceCriteria: []string{"Tenant test passes"}}
			return store.TaskOutput{Status: store.TaskStatusAnswered, Summary: plan.Display(), Plan: &plan, AgentInvocations: 1}
		}), nil
	}, &stdout, &stderr, workspace, nil)
	first.SetReadiness(func(context.Context, account.Request) account.Status {
		return account.Status{Ready: true, Detail: "signed in"}
	}, nil)
	if code := first.Interactive(t.Context(), &promptLines{lines: []string{"/plan add invoice export", "/quit"}}, settings); code != 0 {
		t.Fatalf("first code=%d: %s", code, stdout.String())
	}
	if planID == "" || !strings.Contains(stdout.String(), planID) {
		t.Fatalf("plan ID not shown: %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	second := newTeamHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(_ context.Context, input store.TaskInput) store.TaskOutput {
			if input.PlanOnly || input.SelectedPlan == nil || input.SelectedPlan.ID != planID || input.SelectedPlan.Version != 1 || input.SelectedPlan.Steps[0] != "Add the export" || len(input.RecentTurns) != 1 {
				t.Fatalf("restart lost plan or conversation: %+v", input)
			}
			plan := store.Plan{Action: store.PlanActionAnswer, Summary: "Selected saved plan", Answer: "I found the saved plan."}
			return store.TaskOutput{Status: store.TaskStatusAnswered, Summary: plan.Answer, Plan: &plan, AgentInvocations: 1}
		}), nil
	}, &stdout, &stderr, workspace, nil)
	second.SetReadiness(func(context.Context, account.Request) account.Status {
		return account.Status{Ready: true, Detail: "signed in"}
	}, nil)
	if code := second.Interactive(t.Context(), &promptLines{lines: []string{"implement this plan", "/quit"}}, settings); code != 0 {
		t.Fatalf("second code=%d: %s", code, stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := cli.ContextGet([]string{"get", planID, "--section", "steps"}, workspace, settings, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Add the export") {
		t.Fatalf("read-only retrieval: code=%d out=%s err=%s", code, stdout.String(), stderr.String())
	}
}

func TestGreetingBetweenPlanAndLaterQuestionKeepsCase(t *testing.T) {
	var stdout, stderr bytes.Buffer
	workspace, settings := t.TempDir(), filepath.Join(t.TempDir(), "magent", "config.json")
	calls := 0
	h := newTeamHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(_ context.Context, input store.TaskInput) store.TaskOutput {
			calls++
			switch calls {
			case 1:
				plan := store.Plan{ID: input.PlanArtifactID, CaseID: input.CaseArtifactID, Version: 1, Action: store.PlanActionPropose, Title: "Invoice export", Tags: []string{"invoice"}, Summary: "Export", Steps: []string{"Edit export"}, AcceptanceCriteria: []string{"Pass"}}
				return store.TaskOutput{Status: store.TaskStatusAnswered, Summary: plan.Display(), Plan: &plan}
			case 2:
				if input.Task != "hello" || len(input.RecentTurns) != 0 || input.SelectedPlan != nil {
					t.Fatalf("greeting dragged in history: %+v", input)
				}
			case 3:
				if input.SelectedPlan == nil || input.SelectedPlan.Title != "Invoice export" || len(input.RecentTurns) != 2 || input.RecentTurns[0].User != "add invoice export" {
					t.Fatalf("later case was lost: %+v", input)
				}
			default:
				t.Fatal("unexpected turn")
			}
			answer := store.Plan{Action: store.PlanActionAnswer, Summary: "Answer", Answer: "I remember the plan."}
			return store.TaskOutput{Status: store.TaskStatusAnswered, Summary: answer.Answer, Plan: &answer}
		}), nil
	}, &stdout, &stderr, workspace, nil)
	h.SetReadiness(func(context.Context, account.Request) account.Status {
		return account.Status{Ready: true, Detail: "signed in"}
	}, nil)
	if code := h.Interactive(t.Context(), &promptLines{lines: []string{"/plan add invoice export", "hello", "what did we plan?", "/quit"}}, settings); code != 0 || calls != 3 {
		t.Fatalf("code=%d calls=%d out=%s", code, calls, stdout.String())
	}
}
