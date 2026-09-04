package workflow

import (
	"context"
	"errors"

	"multiharness-core/internal/store"
)

// BillingApprover is a human-consent boundary. A nil approver disables fallback.
// Implementations must decline on unavailable input and preserve cancellation.
type BillingApprover interface {
	ConfirmFallback(context.Context, store.AgentSwitch) (bool, error)
}

// BillingFallbacks supplies alternate ports and operator-visible identities.
// No provider SDK, terminal or configuration types enter the workflow core.
type BillingFallbacks struct {
	Planner        Planner
	Implementer    Implementer
	Reviewer       Reviewer
	Planning       store.AgentSwitch
	Implementation store.AgentSwitch
	Review         store.AgentSwitch
	Approver       BillingApprover
}

func (f BillingFallbacks) validate() error {
	for _, route := range []struct {
		enabled bool
		choice  store.AgentSwitch
		stage   store.WorkflowStage
	}{
		{f.Planner != nil, f.Planning, store.WorkflowStagePlanning},
		{f.Implementer != nil, f.Implementation, store.WorkflowStageImplementation},
		{f.Reviewer != nil, f.Review, store.WorkflowStageReview},
	} {
		if route.enabled {
			if err := route.choice.Validate(); err != nil {
				return err
			}
			if route.choice.Stage != route.stage {
				return errors.New("fallback route has the wrong stage")
			}
		}
	}
	return nil
}

func roleKey(stage store.WorkflowStage) store.WorkflowStage {
	if stage == store.WorkflowStageRepair {
		return store.WorkflowStageImplementation
	}
	return stage
}

func (f BillingFallbacks) choice(stage store.WorkflowStage) (store.AgentSwitch, bool) {
	switch stage {
	case store.WorkflowStagePlanning:
		return f.Planning, f.Planner != nil
	case store.WorkflowStageReview:
		return f.Review, f.Reviewer != nil
	default:
		choice := f.Implementation
		choice.Stage = stage
		return choice, f.Implementer != nil
	}
}

// authorizeFallback inspects partial work before prompting, then rechecks after
// consent. Consent cannot authorize overwriting protected files or stale evidence.
func (s *Service) authorizeFallback(ctx context.Context, state *runState, stage store.WorkflowStage) (bool, error) {
	if s.fallbacks.Approver == nil || state.alternateRoles[roleKey(stage)] {
		return false, nil
	}
	choice, enabled := s.fallbacks.choice(stage)
	if !enabled {
		return false, nil
	}
	if state.agentInvocations >= s.execution.MaxAgentInvocations {
		return false, nil
	}
	if err := state.inspect(ctx, !choice.CanWrite); err != nil {
		return false, err
	}
	yes, err := s.fallbacks.Approver.ConfirmFallback(ctx, choice)
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !yes {
		return false, nil
	}
	if err := state.inspect(ctx, true); err != nil {
		return false, err
	}
	if state.alternateRoles == nil {
		state.alternateRoles = make(map[store.WorkflowStage]bool)
	}
	state.alternateRoles[roleKey(stage)] = true
	state.agentSwitches = append(state.agentSwitches, choice)
	state.events.publish(Event{Type: EventTypeAgentSwitched, Stage: stage, AgentInvocations: state.agentInvocations})
	return true, nil
}
