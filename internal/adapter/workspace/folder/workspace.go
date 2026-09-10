package folder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

var (
	ErrBusy                 = errors.New("another workflow holds an overlapping workspace lock")
	ErrUnsupported          = errors.New("unsupported workspace")
	ErrChangedDuringCapture = errors.New("workspace changed while capturing evidence")
)

// Workspace observes only files beneath the selected folder; no VCS is required.
type Workspace struct {
	config   Config
	approver workflow.WorkspaceApprover
}

func NewWorkspace(config Config) (*Workspace, error) {
	return NewWorkspaceWithApproval(config, nil)
}

func NewWorkspaceWithApproval(config Config, approver workflow.WorkspaceApprover) (*Workspace, error) {
	config, err := config.defaults()
	if err != nil {
		return nil, err
	}
	return &Workspace{config: config, approver: approver}, nil
}

func (workspace *Workspace) resolve(ctx context.Context, dir string) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("workspace context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("working directory is required")
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	for _, component := range strings.Split(filepath.ToSlash(abs), "/") {
		if strings.EqualFold(component, ".git") {
			return "", fmt.Errorf("%w: select project files, not Git metadata", ErrUnsupported)
		}
	}

	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("workspace must be an accessible folder")
	}
	if err := checkWorkspaceAccess(abs); err != nil {
		return "", fmt.Errorf("workspace requires read, write, and traversal permission: %w", err)
	}
	return abs, nil
}

// Acquire excludes overlapping folders before implementation begins.
func (workspace *Workspace) Acquire(ctx context.Context, dir string) (workflow.WorkspaceSession, error) {
	root, err := workspace.resolve(ctx, dir)
	if err != nil {
		return nil, err
	}
	locks, err := acquireFolderLocks(root)
	if err != nil {
		return nil, err
	}
	baseline, err := workspace.stableCapture(ctx, root, nil)
	if err != nil {
		return nil, errors.Join(err, closeLocks(locks))
	}
	s := &session{workspace: workspace, root: root, baseline: baseline, locks: locks}
	if err := s.prepareExistingWork(ctx); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	return s, nil
}

type session struct {
	mu           sync.Mutex
	workspace    *Workspace
	root         string
	baseline     snapshot
	locks        []*os.File
	recovery     string
	editExisting bool
}

func (session *session) Baseline() store.RepositoryEvidence {
	return store.RepositoryEvidence{
		Baseline:               session.baseline.state,
		Current:                session.baseline.state,
		Complete:               true,
		ChangedFiles:           []string{},
		PreExistingFiles:       append([]string{}, session.baseline.existingFiles...),
		PreservationViolations: []string{},
		RecoveryDirectory:      session.recovery,
		ExistingWorkAuthorized: session.editExisting,
	}
}

func (session *session) Inspect(ctx context.Context) (store.RepositoryEvidence, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	evidence := session.Baseline()
	evidence.Complete = false
	if session.locks == nil {
		return evidence, fmt.Errorf("workspace session is closed")
	}
	current, err := session.workspace.stableCapture(ctx, session.root, session.baseline.files)
	if err != nil {
		return session.recoverEvidence(evidence, err)
	}
	evidence.Current = current.state
	evidence.ChangedFiles = changedFiles(session.baseline.files, current.files)
	for _, name := range session.baseline.existingFiles {
		if !session.editExisting && !sameFile(session.baseline.files[name], current.files[name]) {
			evidence.PreservationViolations = append(evidence.PreservationViolations, name)
		}
	}
	evidence.Diff, err = session.workspace.diff(ctx, session.baseline.files, current.files, evidence.ChangedFiles)
	if err != nil {
		return session.recoverEvidence(evidence, err)
	}
	evidence.Complete = true
	if len(evidence.PreservationViolations) > 0 {
		return session.recoverEvidence(evidence, nil)
	}
	return evidence, nil
}

func (session *session) recoverEvidence(evidence store.RepositoryEvidence, cause error) (store.RepositoryEvidence, error) {
	if session.recovery == "" {
		var err error
		session.recovery, err = session.workspace.saveRecovery(context.Background(), session.baseline, session.root)
		cause = errors.Join(cause, err)
	}
	evidence.RecoveryDirectory = session.recovery
	return evidence, cause
}

func (session *session) Close() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.locks == nil {
		return nil
	}
	err := closeLocks(session.locks)
	session.locks = nil
	return err
}

func closeLocks(locks []*os.File) error {
	var err error
	for i := len(locks) - 1; i >= 0; i-- {
		err = errors.Join(err, locks[i].Close())
	}
	return err
}

var _ workflow.Workspace = (*Workspace)(nil)
