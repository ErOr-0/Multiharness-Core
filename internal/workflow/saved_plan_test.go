package workflow_test

import (
	"context"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func TestPlanOnlyStopsBeforeImplementationEvenWithDecisionModel(t *testing.T) {
	h := newWorkflowHarness(t)
	h.planner.plan = store.Plan{Action: store.PlanActionPropose, Title: "Export invoices", Tags: []string{"invoices"}, Summary: "Export scoped invoices", HandoffContext: []string{"api.go owns exports"}, Steps: []string{"Add endpoint"}, AcceptanceCriteria: []string{"Tenant test passes"}}
	service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, store.TaskInput) (store.PlanningDecision, error) {
		t.Fatal("plan-only task should bypass implementation routing")
		return store.PlanningDecision{}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	input := validTask(0)
	input.Task, input.PlanOnly = "Plan the invoice export", true
	input.PlanArtifactID, input.CaseArtifactID = "plan_test", "case_test"
	out := service.Run(t.Context(), input)
	if out.Status != store.TaskStatusAnswered || out.Plan == nil || out.Plan.ID != input.PlanArtifactID || out.Plan.CaseID != input.CaseArtifactID || out.Plan.Version != 1 || h.workspace.session != nil || len(h.implementer.implementationCalls) != 0 {
		t.Fatalf("plan-only crossed into coding: %+v", out)
	}
	if err := out.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectedPlanAndImplementationIDsReachReviewer(t *testing.T) {
	h := newWorkflowHarness(t)
	selected := store.Plan{ID: "plan_exact", CaseID: "case_invoice", Version: 2, Action: store.PlanActionPropose, Title: "Invoice export", Tags: []string{"invoices"}, Summary: "Export scoped invoices", HandoffContext: []string{"Keep tenant scope"}, Steps: []string{"Add paginated export"}, AcceptanceCriteria: []string{"Tenant test passes"}}
	service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, store.TaskInput) (store.PlanningDecision, error) {
		return store.PlanningDecision{Route: store.RouteImplement, Source: store.DecisionJev, Confidence: .99}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	input := validTask(0)
	input.Task, input.SelectedPlan, input.ImplementationArtifactID = "Implement this plan", &selected, "impl_exact"
	out := service.Run(t.Context(), input)
	if out.Status != store.TaskStatusApproved || out.Plan.ID != selected.ID || out.Plan.CaseID != selected.CaseID || out.Plan.Version != 2 || out.Implementation.ID != input.ImplementationArtifactID {
		t.Fatalf("lost exact references: %+v", out)
	}
	if len(h.implementer.implementationCalls) != 1 || h.implementer.implementationCalls[0].Plan.Steps[0] != selected.Steps[0] || h.implementer.implementationCalls[0].Input.SelectedPlan != nil {
		t.Fatalf("implementation did not receive one exact plan: %+v", h.implementer.implementationCalls)
	}
	if len(h.reviewer.requests) != 1 || h.reviewer.requests[0].Plan.ID != selected.ID || h.reviewer.requests[0].Implementation.ID != input.ImplementationArtifactID {
		t.Fatalf("reviewer references: %+v", h.reviewer.requests)
	}
	if err := out.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStaleSelectedPlanStopsBeforeEditing(t *testing.T) {
	h := newWorkflowHarness(t)
	selected := store.Plan{ID: "plan_exact", Version: 1, Action: store.PlanActionPropose, Title: "Export", Tags: []string{"export"}, Summary: "Export", Steps: []string{"Edit"}, AcceptanceCriteria: []string{"Pass"}}
	service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, store.TaskInput) (store.PlanningDecision, error) {
		return store.PlanningDecision{Route: store.RouteImplement, Source: store.DecisionJev, Confidence: .99}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	input := validTask(0)
	input.SelectedPlan, input.SelectedPlanStale = &selected, true
	out := service.Run(t.Context(), input)
	if out.Status != store.TaskStatusFailed || out.Failure.Code != store.FailureCodeWorkspace || h.workspace.session != nil || len(h.implementer.implementationCalls) != 0 {
		t.Fatalf("stale plan edited workspace: %+v", out)
	}
}
