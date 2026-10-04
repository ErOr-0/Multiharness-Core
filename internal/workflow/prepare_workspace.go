package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/contract"
)

var errNilWorkspaceSession = errors.New("workspace returned a nil session")

// No mutating agent has run at this boundary. Reacquisition takes a fresh backup
// and repeats the adapter's existing-work approval; it never carries consent
// from an obsolete snapshot into a new one. Later stages must fail on stale data.
func (service *Service) prepareWorkspace(ctx context.Context, state *runState) error {
	for attempt := 0; ; attempt++ {
		err := service.acquireAndInspect(ctx, state)
		if err == nil {
			return nil
		}
		var changed *contract.WorkspaceChangedError
		if !errors.As(err, &changed) || ctx.Err() != nil {
			return err
		}
		if attempt == 2 {
			return fmt.Errorf("workspace kept changing during 3 preparation attempts; pause other edits and retry the saved plan; no implementation agent ran: %w", err)
		}
		if state.workspace != nil {
			if closeErr := state.workspace.Close(); closeErr != nil {
				return errors.Join(err, closeErr)
			}
			state.workspace = nil
		}
		state.events.publish(Event{Type: EventTypeWorkspaceRetry, Stage: contract.WorkflowStageImplementation, RetryAttempt: attempt + 1})
	}
}

func (service *Service) acquireAndInspect(ctx context.Context, state *runState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lease, err := service.workspace.Acquire(ctx, state.input.WorkingDir)
	if err != nil {
		return err
	}
	if lease == nil {
		return errNilWorkspaceSession
	}
	state.workspace = lease
	baseline := lease.Baseline()
	if err := baseline.Validate(); err != nil {
		return err
	}
	if baseline.Baseline != baseline.Current || len(baseline.ChangedFiles) != 0 {
		return errors.New("workspace baseline already contains run changes")
	}
	state.repository = baseline.Clone()
	if err := state.checkRepository(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return state.inspect(ctx, true)
}
