package workflow_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

type routingStub struct {
	decide func(context.Context, contract.TaskInput) (contract.PlanningDecision, error)
}

func (r routingStub) DecidePlanning(ctx context.Context, in contract.TaskInput) (contract.PlanningDecision, error) {
	return r.decide(ctx, in)
}
func (routingStub) DecideReview(context.Context, contract.ReviewRequest) (contract.ReviewDecision, error) {
	return contract.ReviewDecision{ShouldReview: true}, nil
}

func TestJevRoutesBeforeAnyAgentAndPreservesExecutionBoundaries(t *testing.T) {
	for _, route := range []contract.TaskRoute{contract.RouteAnswer, contract.RoutePlan, contract.RouteImplement} {
		t.Run(string(route), func(t *testing.T) {
			h := newWorkflowHarness(t)
			h.planner.run = func(_ context.Context, input contract.TaskInput) (contract.Plan, error) {
				if route == contract.RouteImplement {
					t.Fatal("direct route invoked planner")
				}
				if input.AnswerOnly != (route == contract.RouteAnswer) {
					t.Fatal("wrong answer constraint", input.AnswerOnly)
				}
				if route == contract.RouteAnswer {
					return answerPlan(), nil
				}
				return validPlan(), nil
			}
			service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, contract.TaskInput) (contract.PlanningDecision, error) {
				if len(h.calls.snapshot()) != 0 {
					t.Fatal("agent ran before Jev")
				}
				h.calls.record("jev")
				return contract.PlanningDecision{Route: route, Source: contract.DecisionJev, NeedsPlanning: route == contract.RoutePlan, Confidence: .95}, nil
			}}})
			if err != nil {
				t.Fatal(err)
			}
			out := service.Run(t.Context(), validTask(0))
			if err := out.Validate(); err != nil {
				t.Fatal(err, out)
			}
			if out.Routing == nil || out.Routing.Route != route || out.Routing.Source != contract.DecisionJev {
				t.Fatal("lost decision", out)
			}
			calls := h.calls.snapshot()
			if route == contract.RouteAnswer {
				if out.Status != contract.TaskStatusAnswered || out.AgentInvocations != 1 || !reflect.DeepEqual(calls, []string{"jev", "plan"}) || h.workspace.session != nil {
					t.Fatal("question entered coding workflow", out, calls)
				}
			} else if out.Status != contract.TaskStatusApproved || len(h.implementer.implementationCalls) != 1 || len(h.validator.requests) != 1 || len(h.reviewer.requests) != 1 {
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
				if event.Type == workflow.EventTypeStageStarted && event.Stage != contract.WorkflowStageIntake && event.Stage != contract.WorkflowStageRouting && !decided {
					t.Fatal("agent stage preceded route", event)
				}
				if route != contract.RoutePlan && event.Stage == contract.WorkflowStagePlanning {
					t.Fatal("misleading planning stage", event)
				}
				if route == contract.RouteAnswer && event.Type == workflow.EventTypeWorkflowCompleted && event.Stage != contract.WorkflowStageAnswering {
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
	service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, contract.TaskInput) (contract.PlanningDecision, error) {
		return contract.PlanningDecision{Route: contract.RouteAnswer, Source: contract.DecisionJev, Confidence: .99}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	// The fake planner deliberately disobeys the answer-only instruction.
	out := service.Run(t.Context(), validTask(0))
	if out.Status != contract.TaskStatusFailed || out.Failure.Stage != contract.WorkflowStageAnswering || out.Failure.Code != contract.FailureCodeInvalidOutput || h.workspace.session != nil || len(h.implementer.implementationCalls) != 0 {
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
			service, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events, DecisionMaker: routingStub{decide: func(context.Context, contract.TaskInput) (contract.PlanningDecision, error) {
				if kind == "cancelled" {
					cancel()
				}
				if kind == "error" {
					return contract.PlanningDecision{}, errors.New("PRIVATE provider body")
				}
				return contract.PlanningDecision{Route: "unknown"}, nil
			}}})
			if err != nil {
				t.Fatal(err)
			}
			out := service.Run(ctx, validTask(0))
			if kind == "cancelled" {
				if out.Status != contract.TaskStatusCancelled || len(h.calls.snapshot()) != 0 {
					t.Fatal(out)
				}
				return
			}
			if out.Status != contract.TaskStatusAnswered || out.Routing.Source != contract.DecisionFallback || out.Routing.Route != contract.RoutePlan || len(h.implementer.implementationCalls) != 0 {
				t.Fatal(out)
			}
			if out.Routing.Reason == "PRIVATE provider body" {
				t.Fatal("raw error exposed")
			}
		})
	}
}
