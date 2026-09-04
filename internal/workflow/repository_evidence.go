package workflow

import (
	"context"
	"errors"
	"fmt"
)

func (state *runState) checkRepository() error {
	if state.repository == nil {
		return errors.New("repository evidence is required")
	}
	if err := state.repository.Validate(); err != nil {
		return err
	}
	if !state.repository.Complete {
		return errors.New("repository inspection is incomplete")
	}
	if len(state.repository.PreservationViolations) > 0 {
		return fmt.Errorf("protected user work or Git state changed: %v; recovery snapshot: %s",
			state.repository.PreservationViolations, state.repository.RecoveryDirectory)
	}
	return nil
}

// inspect records evidence independently from any implementation claim. A
// read-only stage must leave the exact inspected checkout unchanged.
func (state *runState) inspect(ctx context.Context, requireUnchanged bool) error {
	previous := state.repository.Current.Fingerprint
	evidence, err := state.workspace.Inspect(ctx)
	if err != nil {
		evidence.Complete = false
	}
	if evidence.Validate() == nil && evidence.Baseline == state.repository.Baseline {
		state.repository = evidence.Clone()
		if state.implementation != nil {
			state.implementation.ChangedFiles = append([]string{}, evidence.ChangedFiles...)
		}
	} else {
		state.repository.Complete = false
	}
	if err != nil {
		return err
	}
	if err := state.checkRepository(); err != nil {
		return err
	}
	if requireUnchanged && previous != state.repository.Current.Fingerprint {
		return errors.New("workspace changed during a read-only stage or between stages; evidence is stale")
	}
	return nil
}
