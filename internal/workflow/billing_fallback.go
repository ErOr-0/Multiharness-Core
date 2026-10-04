package workflow

import (
	"context"
	"errors"

	"multiharness-core/internal/contract"
)

// BillingApprover is a human-consent boundary. A nil approver disables fallback.
// Implementations must decline on unavailable input and preserve cancellation.
type BillingApprover interface {
	ConfirmFallback(context.Context, contract.AgentSwitch) (bool, error)
}

// BillingFallbacks supplies alternate ports and operator-visible identities.
// No provider SDK, terminal or configuration types enter the workflow core.
type BillingFallbacks struct {
	Planner        Planner
	Implementer    Implementer
	Reviewer       Reviewer
	Planning       contract.AgentSwitch
	Implementation contract.AgentSwitch
	Review         contract.AgentSwitch
	Approver       BillingApprover
}

func (f BillingFallbacks) validate() error {
	for _, route := range []struct {
		enabled bool
		choice  contract.AgentSwitch
		stage   contract.WorkflowStage
	}{
		{f.Planner != nil, f.Planning, contract.WorkflowStagePlanning},
		{f.Implementer != nil, f.Implementation, contract.WorkflowStageImplementation},
		{f.Reviewer != nil, f.Review, contract.WorkflowStageReview},
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

func roleKey(stage contract.WorkflowStage) contract.WorkflowStage {
	if stage == contract.WorkflowStageAnswering {
		return contract.WorkflowStagePlanning
	}
	if stage == contract.WorkflowStageRepair {
		return contract.WorkflowStageImplementation
	}
	return stage
}

func (f BillingFallbacks) choice(stage contract.WorkflowStage) (contract.AgentSwitch, bool) {
	switch stage {
	case contract.WorkflowStagePlanning, contract.WorkflowStageAnswering:
		choice := f.Planning
		choice.Stage = stage
		return choice, f.Planner != nil
	case contract.WorkflowStageReview:
		return f.Review, f.Reviewer != nil
	case contract.WorkflowStageImplementation, contract.WorkflowStageRepair:
		choice := f.Implementation
		choice.Stage = stage
		return choice, f.Implementer != nil
	default:
		return contract.AgentSwitch{}, false
	}
}

// authorizeFallback inspects partial work before prompting, then rechecks after
// consent. Consent cannot authorize overwriting protected files or stale evidence.
func (s *Service) authorizeFallback(ctx context.Context, state *runState, stage contract.WorkflowStage) (bool, error) {
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
	if err := state.inspectAcquired(ctx, !choice.CanWrite); err != nil {
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
	if err := state.inspectAcquired(ctx, true); err != nil {
		return false, err
	}
	if state.alternateRoles == nil {
		state.alternateRoles = make(map[contract.WorkflowStage]bool)
	}
	state.alternateRoles[roleKey(stage)] = true
	state.agentSwitches = append(state.agentSwitches, choice)
	state.events.publish(Event{Type: EventTypeAgentSwitched, Stage: stage, AgentInvocations: state.agentInvocations})
	return true, nil
}
