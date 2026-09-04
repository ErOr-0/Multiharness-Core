package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/store"
)

// SystemIOMediator defines the mediator interface coordinating interactions
// between workflow execution, the Git workspace (locking, snapshots, diffs),
// and validation test runners.
type SystemIOMediator interface {
	Acquire(ctx context.Context, workingDir string) error
	VerifyIntegrity(ctx context.Context, requireUnchanged bool) error
	DeriveEvidence(ctx context.Context) (*store.RepositoryEvidence, error)
	ExecuteValidation(ctx context.Context, req store.ValidationRequest) (store.ValidationReport, error)
	Close() error
}

// WorkspaceMediator coordinates between Git workspace sessions, repository
// evidence tracking, and deterministic validation execution.
type WorkspaceMediator struct {
	workspace Workspace
	validator Validator
	session   WorkspaceSession
	state     *runState
}

// NewWorkspaceMediator constructs a new mediator binding the workspace and validator
// to the current run state.
func NewWorkspaceMediator(workspace Workspace, validator Validator, state *runState) *WorkspaceMediator {
	return &WorkspaceMediator{
		workspace: workspace,
		validator: validator,
		state:     state,
	}
}

// Acquire validates the target directory and acquires exclusive workspace access,
// initializing the baseline evidence for the run.
func (m *WorkspaceMediator) Acquire(ctx context.Context, workingDir string) error {
	if m.workspace == nil {
		return errors.New("workspace port is not configured")
	}
	if err := m.workspace.Validate(ctx, workingDir); err != nil {
		return err
	}
	lease, err := m.workspace.Acquire(ctx, workingDir)
	if err != nil {
		return err
	}
	if lease == nil {
		return errors.New("workspace returned a nil session")
	}
	m.session = lease
	m.state.workspace = lease

	baseline := lease.Baseline()
	if err := baseline.Validate(); err != nil {
		return err
	}
	if baseline.Baseline != baseline.Current || len(baseline.ChangedFiles) != 0 {
		return errors.New("workspace baseline already contains run changes")
	}
	m.state.repository = baseline.Clone()
	return m.state.checkRepository()
}

// VerifyIntegrity checks that read-only stages leave the repository unchanged
// and that no protected user files have been altered.
func (m *WorkspaceMediator) VerifyIntegrity(ctx context.Context, requireUnchanged bool) error {
	return m.state.inspect(ctx, requireUnchanged)
}

// DeriveEvidence obtains the latest workspace diff and file attribution from Git.
func (m *WorkspaceMediator) DeriveEvidence(ctx context.Context) (*store.RepositoryEvidence, error) {
	if err := m.state.inspect(ctx, false); err != nil {
		return m.state.repository.Clone(), err
	}
	return m.state.repository.Clone(), nil
}

// ExecuteValidation coordinates pre-validation integrity check, deterministic test
// execution, and post-validation evidence inspection.
func (m *WorkspaceMediator) ExecuteValidation(ctx context.Context, req store.ValidationRequest) (store.ValidationReport, error) {
	if err := m.VerifyIntegrity(ctx, true); err != nil {
		return store.ValidationReport{}, err
	}
	if err := req.Validate(); err != nil {
		return store.ValidationReport{}, err
	}
	validation, valErr := m.validator.Validate(ctx, req)
	if validation.Validate() == nil {
		m.state.setValidation(validation)
	}
	inspectionErr := m.VerifyIntegrity(ctx, true)
	if valErr != nil {
		return validation, errors.Join(valErr, inspectionErr)
	}
	if inspectionErr != nil {
		return validation, inspectionErr
	}
	if err := ctx.Err(); err != nil {
		return validation, err
	}
	if err := validation.Validate(); err != nil {
		return validation, fmt.Errorf("invalid validator output: %w", err)
	}
	return validation, nil
}

// Close releases the exclusive workspace lock.
func (m *WorkspaceMediator) Close() error {
	if m.session == nil {
		return nil
	}
	err := m.session.Close()
	m.session = nil
	return err
}

var _ SystemIOMediator = (*WorkspaceMediator)(nil)
