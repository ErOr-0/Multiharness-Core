package workflow_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type routingStub struct {
	decide func(context.Context, store.TaskInput) (store.PlanningDecision, error)
}

func (r routingStub) DecidePlanning(ctx context.Context, in store.TaskInput) (store.PlanningDecision, error) {
	return r.decide(ctx, in)
}
func (routingStub) DecideReview(context.Context, store.ReviewRequest) (store.ReviewDecision, error) {
	return store.ReviewDecision{ShouldReview: true}, nil
}

func TestJevRoutesBeforeAnyAgentAndPreservesExecutionBoundaries(t *testing.T) {
	for _, route := range []store.TaskRoute{store.RouteAnswer, store.RoutePlan, store.RouteImplement} {
		t.Run(string(route), func(t *testing.T) {
			h := newWorkflowHarness(t)
			h.planner.run = func(_ context.Context, input store.TaskInput) (store.Plan, error) {
				if route == store.RouteImplement {
					t.Fatal("direct route invoked planner")
				}
				if input.AnswerOnly != (route == store.RouteAnswer) {
					t.Fatal("wrong answer constraint", input.AnswerOnly)
				}
				if route == store.RouteAnswer {
					return answerPlan(), nil
				}
				return validPlan(), nil
			}
			service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, store.TaskInput) (store.PlanningDecision, error) {
				if len(h.calls.snapshot()) != 0 {
					t.Fatal("agent ran before Jev")
				}
				h.calls.record("jev")
				return store.PlanningDecision{Route: route, Source: store.DecisionJev, NeedsPlanning: route == store.RoutePlan, Confidence: .95}, nil
			}}})
			if err != nil {
				t.Fatal(err)
			}
			out := service.Run(t.Context(), validTask(0))
			if err := out.Validate(); err != nil {
				t.Fatal(err, out)
			}
			if out.Routing == nil || out.Routing.Route != route || out.Routing.Source != store.DecisionJev {
				t.Fatal("lost decision", out)
			}
			calls := h.calls.snapshot()
			if route == store.RouteAnswer {
				if out.Status != store.TaskStatusAnswered || out.AgentInvocations != 1 || !reflect.DeepEqual(calls, []string{"jev", "plan"}) || h.workspace.session != nil {
					t.Fatal("question entered coding workflow", out, calls)
				}
			} else if out.Status != store.TaskStatusApproved || len(h.implementer.implementationCalls) != 1 || len(h.validator.requests) != 1 || len(h.reviewer.requests) != 1 {
				t.Fatal("implementation lost validation/review", out, calls)
			}
			decided := false
			for _, event := range h.events.snapshot() {
				if event.Type == workflow.EventTypeRoutingDecided {
					decided = true
					if event.Route != route {
						t.Fatal(event)
					}
				}
				if event.Type == workflow.EventTypeStageStarted && event.Stage != store.WorkflowStageIntake && event.Stage != store.WorkflowStageRouting && !decided {
					t.Fatal("agent stage preceded route", event)
				}
				if route != store.RoutePlan && event.Stage == store.WorkflowStagePlanning {
					t.Fatal("misleading planning stage", event)
				}
				if route == store.RouteAnswer && event.Type == workflow.EventTypeWorkflowCompleted && event.Stage != store.WorkflowStageAnswering {
					t.Fatal("wrong terminal stage", event)
				}
			}
			if !decided {
				t.Fatal("decision was hidden")
			}
		})
	}
}

func TestQuestionRouteCannotEscalateIntoChanges(t *testing.T) {
	h := newWorkflowHarness(t)
	service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, store.TaskInput) (store.PlanningDecision, error) {
		return store.PlanningDecision{Route: store.RouteAnswer, Source: store.DecisionJev, Confidence: .99}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	// The fake planner deliberately disobeys the answer-only instruction.
	out := service.Run(t.Context(), validTask(0))
	if out.Status != store.TaskStatusFailed || out.Failure.Stage != store.WorkflowStageAnswering || out.Failure.Code != store.FailureCodeInvalidOutput || h.workspace.session != nil || len(h.implementer.implementationCalls) != 0 {
		t.Fatal("answer-only escaped", out)
	}
	if err := out.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingFailureInvalidDecisionAndCancellationCannotSkipAssessment(t *testing.T) {
	for _, kind := range []string{"error", "invalid", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			h := newWorkflowHarness(t)
			h.planner.plan = answerPlan()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, store.TaskInput) (store.PlanningDecision, error) {
				if kind == "cancelled" {
					cancel()
				}
				if kind == "error" {
					return store.PlanningDecision{}, errors.New("PRIVATE provider body")
				}
				return store.PlanningDecision{Route: "unknown"}, nil
			}}})
			if err != nil {
				t.Fatal(err)
			}
			out := service.Run(ctx, validTask(0))
			if kind == "cancelled" {
				if out.Status != store.TaskStatusCancelled || len(h.calls.snapshot()) != 0 {
					t.Fatal(out)
				}
				return
			}
			if out.Status != store.TaskStatusAnswered || out.Routing.Source != store.DecisionFallback || out.Routing.Route != store.RoutePlan || len(h.implementer.implementationCalls) != 0 {
				t.Fatal(out)
			}
			if out.Routing.Reason == "PRIVATE provider body" {
				t.Fatal("raw error exposed")
			}
		})
	}
}

func TestQuestionFallbackKeepsAnswerOnlyConstraint(t *testing.T) {
	for _, consent := range []bool{false, true} {
		h := newWorkflowHarness(t)
		h.planner.err = billingError()
		alternate := &fakePlanner{run: func(_ context.Context, in store.TaskInput) (store.Plan, error) {
			if !in.AnswerOnly {
				t.Fatal("fallback lost read-only answer constraint")
			}
			return answerPlan(), nil
		}}
		service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, store.TaskInput) (store.PlanningDecision, error) {
			return store.PlanningDecision{Route: store.RouteAnswer, Source: store.DecisionJev, Confidence: .99}, nil
		}}, Fallbacks: workflow.BillingFallbacks{
			Planner: alternate, Planning: store.AgentSwitch{Stage: store.WorkflowStagePlanning, From: "Codex", To: "OpenCode", Model: "provider/model"},
			Approver: approvalFunc(func(_ context.Context, s store.AgentSwitch) (bool, error) {
				if s.Stage != store.WorkflowStageAnswering || s.CanWrite {
					t.Fatal("wrong fallback role", s)
				}
				return consent, nil
			}),
		}})
		if err != nil {
			t.Fatal(err)
		}
		out := service.Run(t.Context(), validTask(0))
		if err := out.Validate(); err != nil {
			t.Fatal(err, out)
		}
		want := store.TaskStatusFailed
		if consent {
			want = store.TaskStatusAnswered
		}
		if out.Status != want || h.workspace.session != nil || len(h.implementer.implementationCalls) != 0 {
			t.Fatal("question fallback entered coding", out)
		}
	}
}
