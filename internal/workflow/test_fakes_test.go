package workflow_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

var errUnexpectedFakeCall = errors.New("unexpected fake call")

type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (log *callLog) record(call string) {
	if log == nil {
		return
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	log.calls = append(log.calls, call)
}

func (log *callLog) snapshot() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.calls...)
}

type fakeWorkspace struct {
	session    *fakeWorkspaceSession
	acquireErr error
	calls      *callLog
	acquire    func(context.Context, string) error
}

func (fake *fakeWorkspace) Acquire(ctx context.Context, workingDir string) (workflow.WorkspaceSession, error) {
	fake.calls.record("workspace")
	if fake.acquire != nil {
		if err := fake.acquire(ctx, workingDir); err != nil {
			return nil, err
		}
	}
	if fake.acquireErr != nil {
		return nil, fake.acquireErr
	}
	if fake.session == nil {
		fake.session = newFakeWorkspaceSession()
	}
	return fake.session, nil
}

type fakeWorkspaceSession struct {
	baseline  store.RepositoryEvidence
	current   store.RepositoryEvidence
	inspect   func(context.Context) (store.RepositoryEvidence, error)
	closed    bool
	closeErr  error
	closeHook func()
}

func newFakeWorkspaceSession() *fakeWorkspaceSession {
	state := store.RepositoryState{Root: "/workspace/project", Fingerprint: "baseline"}
	evidence := store.RepositoryEvidence{
		Baseline:               state,
		Current:                state,
		Complete:               true,
		ChangedFiles:           []string{},
		PreExistingFiles:       []string{},
		PreservationViolations: []string{},
	}
	return &fakeWorkspaceSession{baseline: evidence, current: evidence}
}
func (fake *fakeWorkspaceSession) Baseline() store.RepositoryEvidence { return *fake.baseline.Clone() }
func (fake *fakeWorkspaceSession) Inspect(ctx context.Context) (store.RepositoryEvidence, error) {
	if fake.inspect != nil {
		return fake.inspect(ctx)
	}
	return *fake.current.Clone(), nil
}
func (fake *fakeWorkspaceSession) Close() error {
	fake.closed = true
	if fake.closeHook != nil {
		fake.closeHook()
	}
	return fake.closeErr
}

func (fake *fakeImplementer) recordFiles(result store.ImplementationResult, err error) {
	if fake.workspace == nil || fake.workspace.session == nil || err != nil {
		return
	}
	fake.workspace.session.current.Current.Fingerprint = "implemented:" + result.Summary
	fake.workspace.session.current.ChangedFiles = append([]string{}, result.ChangedFiles...)
}

type fakePlanner struct {
	calls *callLog
	plan  store.Plan
	err   error
	run   func(context.Context, store.TaskInput) (store.Plan, error)
}

func (fake *fakePlanner) Plan(ctx context.Context, input store.TaskInput) (store.Plan, error) {
	fake.calls.record("plan")
	if fake.run != nil {
		return fake.run(ctx, input)
	}
	return fake.plan, fake.err
}

type fakeImplementer struct {
	workspace           *fakeWorkspace
	calls               *callLog
	initial             store.ImplementationResult
	initialErr          error
	implement           func(context.Context, store.ImplementationRequest) (store.ImplementationResult, error)
	repairs             []store.ImplementationResult
	repairErr           error
	repair              func(context.Context, store.RepairRequest) (store.ImplementationResult, error)
	implementationCalls []store.ImplementationRequest
	repairCalls         []store.RepairRequest
}

func (fake *fakeImplementer) Implement(
	ctx context.Context,
	request store.ImplementationRequest,
) (result store.ImplementationResult, err error) {
	defer func() { fake.recordFiles(result, err) }()
	fake.calls.record("implement")
	fake.implementationCalls = append(fake.implementationCalls, request)
	if fake.implement != nil {
		return fake.implement(ctx, request)
	}
	return fake.initial, fake.initialErr
}

func (fake *fakeImplementer) ApplyReview(
	ctx context.Context,
	request store.RepairRequest,
) (result store.ImplementationResult, err error) {
	defer func() { fake.recordFiles(result, err) }()
	fake.calls.record("repair")
	fake.repairCalls = append(fake.repairCalls, request)
	if fake.repair != nil {
		return fake.repair(ctx, request)
	}
	if fake.repairErr != nil {
		return store.ImplementationResult{}, fake.repairErr
	}
	if len(fake.repairs) == 0 {
		return store.ImplementationResult{}, errUnexpectedFakeCall
	}
	result = fake.repairs[0]
	fake.repairs = fake.repairs[1:]
	return result, nil
}

type fakeValidator struct {
	calls    *callLog
	reports  []store.ValidationReport
	err      error
	validate func(context.Context, store.ValidationRequest) (store.ValidationReport, error)
	requests []store.ValidationRequest
}

func (fake *fakeValidator) Validate(
	ctx context.Context,
	request store.ValidationRequest,
) (store.ValidationReport, error) {
	fake.calls.record("validate")
	fake.requests = append(fake.requests, request)
	if fake.validate != nil {
		return fake.validate(ctx, request)
	}
	if fake.err != nil {
		return store.ValidationReport{}, fake.err
	}
	if len(fake.reports) == 0 {
		return store.ValidationReport{}, errUnexpectedFakeCall
	}
	report := fake.reports[0]
	fake.reports = fake.reports[1:]
	return report, nil
}

type fakeReviewer struct {
	calls    *callLog
	reviews  []store.Review
	err      error
	review   func(context.Context, store.ReviewRequest) (store.Review, error)
	requests []store.ReviewRequest
}

func (fake *fakeReviewer) Review(
	ctx context.Context,
	request store.ReviewRequest,
) (store.Review, error) {
	fake.calls.record("review")
	fake.requests = append(fake.requests, request)
	if fake.review != nil {
		return fake.review(ctx, request)
	}
	if fake.err != nil {
		return store.Review{}, fake.err
	}
	if len(fake.reviews) == 0 {
		return store.Review{}, errUnexpectedFakeCall
	}
	review := fake.reviews[0]
	fake.reviews = fake.reviews[1:]
	return review, nil
}

type eventCollector struct {
	mu     sync.Mutex
	events []workflow.Event
}

func (collector *eventCollector) Publish(event workflow.Event) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	collector.events = append(collector.events, event)
}

func (collector *eventCollector) snapshot() []workflow.Event {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return append([]workflow.Event(nil), collector.events...)
}

type workflowHarness struct {
	service     *workflow.Service
	calls       *callLog
	workspace   *fakeWorkspace
	planner     *fakePlanner
	implementer *fakeImplementer
	validator   *fakeValidator
	reviewer    *fakeReviewer
	events      *eventCollector
}

func newWorkflowHarness(t *testing.T) *workflowHarness {
	t.Helper()

	calls := &callLog{}
	harness := &workflowHarness{
		calls:       calls,
		workspace:   &fakeWorkspace{calls: calls},
		planner:     &fakePlanner{calls: calls, plan: validPlan()},
		implementer: &fakeImplementer{calls: calls, initial: implementation("initial implementation", "service.go")},
		validator:   &fakeValidator{calls: calls, reports: []store.ValidationReport{passingValidation()}},
		reviewer:    &fakeReviewer{calls: calls, reviews: []store.Review{approvedReview("approved by review")}},
		events:      &eventCollector{},
	}

	harness.implementer.workspace = harness.workspace
	service, err := workflow.NewService(workflow.Dependencies{
		Workspace:   harness.workspace,
		Planner:     harness.planner,
		Implementer: harness.implementer,
		Validator:   harness.validator,
		Reviewer:    harness.reviewer,
		Events:      harness.events,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	harness.service = service
	return harness
}
