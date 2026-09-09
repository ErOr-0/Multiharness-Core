package git

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

// Workspace snapshots folders and uses Git metadata when repositories exist.
type Workspace struct {
	runner ProcessRunner
	config Config
}

func NewWorkspace(runner ProcessRunner, config Config) (*Workspace, error) {
	if runner == nil {
		return nil, fmt.Errorf("Git process runner is required")
	}
	config, err := config.defaults()
	if err != nil {
		return nil, err
	}
	return &Workspace{runner: runner, config: config}, nil
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

// Acquire excludes overlapping folders and shared Git common directories,
// including linked worktrees, before an implementation agent is invoked.
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
	commons := map[string]bool{}
	for _, repo := range baseline.repositories {
		commons[repo.Common] = true
	}
	for _, common := range sortedNames(commons) {
		lock, err := acquireLock(filepath.Join(common, "multiharness.lock"))
		if err != nil {
			return nil, errors.Join(err, closeLocks(locks))
		}
		locks = append(locks, lock)
	}
	// Metadata may have changed while its common-directory lock was acquired.
	confirmed, err := workspace.stableCapture(ctx, root, nil)
	if err == nil && confirmed.state.Fingerprint != baseline.state.Fingerprint {
		err = ErrChangedDuringCapture
	}
	if err != nil {
		return nil, errors.Join(err, closeLocks(locks))
	}
	return &session{workspace: workspace, root: root, baseline: confirmed, locks: locks}, nil
}

type session struct {
	mu        sync.Mutex
	workspace *Workspace
	root      string
	baseline  snapshot
	locks     []*os.File
	recovery  string
}

func (session *session) Baseline() store.RepositoryEvidence {
	return store.RepositoryEvidence{
		Baseline:               session.baseline.state,
		Current:                session.baseline.state,
		Complete:               true,
		ChangedFiles:           []string{},
		PreExistingFiles:       append([]string{}, session.baseline.dirty...),
		PreservationViolations: []string{},
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
	for _, name := range session.baseline.dirty {
		if !sameFile(session.baseline.files[name], current.files[name]) {
			evidence.PreservationViolations = append(evidence.PreservationViolations, name)
		}
	}
	evidence.PreservationViolations = append(evidence.PreservationViolations, repositoryViolations(session.baseline, current)...)
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
		session.recovery, err = saveRecovery(session.baseline)
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
