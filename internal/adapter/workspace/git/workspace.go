package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

var (
	ErrBusy                 = errors.New("another workflow holds the repository lock")
	ErrUnsupported          = errors.New("unsupported workspace")
	ErrChangedDuringCapture = errors.New("workspace changed while capturing evidence")
)

// Workspace inspects Git checkouts without updating the index or working tree.
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

func (workspace *Workspace) resolve(ctx context.Context, dir string) (string, string, error) {
	if ctx == nil {
		return "", "", fmt.Errorf("workspace context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(dir) == "" {
		return "", "", fmt.Errorf("working directory is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace: %w", err)
	}
	root, err := workspace.command(ctx, abs, false, "rev-parse", "--show-toplevel")
	if err != nil {
		var commandErr *process.RunError
		if errors.As(err, &commandErr) && commandErr.Kind == process.ErrorKindNonZeroExit {
			return "", "", fmt.Errorf("%w: an accessible non-bare Git checkout is required: %w", ErrUnsupported, err)
		}
		return "", "", err
	}
	root, err = filepath.EvalSymlinks(strings.TrimSuffix(root, "\n"))
	if err != nil {
		return "", "", err
	}
	if root != abs {
		return "", "", fmt.Errorf("%w: use repository root %q, not a subdirectory", ErrUnsupported, root)
	}
	if err := checkWorkspaceAccess(root); err != nil {
		return "", "", fmt.Errorf("workspace requires read, write, and traversal permission: %w", err)
	}
	common, err := workspace.command(ctx, root, false, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", err
	}
	return root, strings.TrimSuffix(common, "\n"), nil
}

func (workspace *Workspace) Validate(ctx context.Context, dir string) error {
	_, _, err := workspace.resolve(ctx, dir)
	return err
}

// Acquire serializes all cooperating runs sharing a Git common directory,
// including linked worktrees, and snapshots before any agent is invoked.
func (workspace *Workspace) Acquire(ctx context.Context, dir string) (workflow.WorkspaceSession, error) {
	root, common, err := workspace.resolve(ctx, dir)
	if err != nil {
		return nil, err
	}
	lock, err := acquireLock(filepath.Join(common, "multiharness.lock"))
	if err != nil {
		return nil, err
	}
	baseline, err := workspace.stableCapture(ctx, root, nil)
	if err != nil {
		return nil, errors.Join(err, lock.Close())
	}
	return &session{workspace: workspace, root: root, baseline: baseline, lock: lock}, nil
}

type session struct {
	mu        sync.Mutex
	workspace *Workspace
	root      string
	baseline  snapshot
	lock      *os.File
	recovery  string
}

func (session *session) Baseline() store.RepositoryEvidence {
	return store.RepositoryEvidence{Baseline: session.baseline.state, Current: session.baseline.state, Complete: true,
		ChangedFiles: []string{}, PreExistingFiles: append([]string{}, session.baseline.dirty...), PreservationViolations: []string{}}
}

func (session *session) Inspect(ctx context.Context) (store.RepositoryEvidence, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	evidence := session.Baseline()
	evidence.Complete = false
	if session.lock == nil {
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
	if session.baseline.index != current.index {
		evidence.PreservationViolations = append(evidence.PreservationViolations, "[Git index]")
	}
	if session.baseline.state.Head != current.state.Head || session.baseline.ref != current.ref {
		evidence.PreservationViolations = append(evidence.PreservationViolations, "[Git HEAD]")
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
		session.recovery, err = saveRecovery(session.baseline)
		cause = errors.Join(cause, err)
	}
	evidence.RecoveryDirectory = session.recovery
	return evidence, cause
}

func (session *session) Close() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.lock == nil {
		return nil
	}
	err := session.lock.Close()
	session.lock = nil
	return err
}

var _ workflow.Workspace = (*Workspace)(nil)
