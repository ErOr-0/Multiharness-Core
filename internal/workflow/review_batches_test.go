package workflow_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func TestStageCompletedEventsCarryHandoffDiagnostics(t *testing.T) {
	harness := newWorkflowHarness(t)
	output := harness.service.Run(t.Context(), validTask(0))
	if output.Status != store.TaskStatusApproved {
		t.Fatalf("Run() status = %q", output.Status)
	}
	var reviewEvent *workflow.Event
	for _, event := range harness.events.snapshot() {
		if event.Type == workflow.EventTypeStageCompleted && event.Stage == store.WorkflowStageReview {
			event := event
			reviewEvent = &event
		}
	}
	if reviewEvent == nil {
		t.Fatal("no review completion event")
	}
	if reviewEvent.ReviewChunkCount != 1 {
		t.Fatalf("ReviewChunkCount = %d, want 1", reviewEvent.ReviewChunkCount)
	}
}

func rebuildService(t *testing.T, harness *workflowHarness, chunkBytes, maxPrompt int) *workflow.Service {
	t.Helper()
	svc, err := workflow.NewService(workflow.Dependencies{
		Workspace: harness.workspace, Planner: harness.planner,
		Implementer: harness.implementer, Validator: harness.validator,
		Reviewer: harness.reviewer, Events: harness.events,
		Execution: workflow.ExecutionPolicy{MaxAgentInvocations: 64, MaxPromptBytes: maxPromptForTest(maxPrompt), ReviewChunkBytes: chunkBytes},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func maxPromptForTest(n int) int {
	if n <= 0 {
		return 262144
	}
	if n < 2048 {
		return 2048
	}
	return n
}

func TestReviewBatchesAssignEveryHunkExactlyOnce(t *testing.T) {
	diff := ""
	files := []string{"a.go", "b.go"}
	for _, f := range files {
		diff += "--- before/" + f + "\n+++ after/" + f + "\n@@ -1,2 +1,2 @@\n ctx\n-old\n+new\n"
	}
	req := store.ReviewRequest{
		Input: validTask(0), Plan: validPlan(),
		Implementation: store.ImplementationResult{Summary: "s", ChangedFiles: files},
		Validation:     passingValidation(),
		Repository: &store.RepositoryEvidence{
			Baseline: store.RepositoryState{Root: "/w", Fingerprint: "b"},
			Current:  store.RepositoryState{Root: "/w", Fingerprint: "c"},
			Complete: true, ChangedFiles: files, Diff: diff,
		},
	}
	batches := workflow.BuildReviewBatches(req, 80)
	if len(batches) < 2 {
		t.Fatalf("expected multiple batches, got %d", len(batches))
	}
	seen := map[string]int{}
	for _, b := range batches {
		if len(b.DiffChunk) > 80 {
			t.Fatal("oversized chunk")
		}
		for _, f := range b.ChangedFiles {
			seen[f]++
		}
		if b.ChunkCount != len(batches) || !b.Complete {
			t.Fatal("bad completeness markers")
		}
		if b.WorkspaceRoot != "/w" || b.WorkspaceFingerprint != "c" {
			t.Fatal("workspace identity lost")
		}
	}
	for _, f := range files {
		if seen[f] != 1 {
			t.Fatalf("file %s in %d batches", f, seen[f])
		}
	}
}

func TestBatchedReviewApprovesOnlyWhenAllChunksPass(t *testing.T) {
	harness := newWorkflowHarness(t)
	harness.service = rebuildService(t, harness, 60, 64)
	// Force a two-chunk diff via the implementer, mirroring real workspace evidence.
	twoFileDiff := "--- before/a.go\n+++ after/a.go\n@@ -1 +1 @@\n-a\n+b\n" +
		"--- before/b.go\n+++ after/b.go\n@@ -1 +1 @@\n-c\n+d\n"
	harness.implementer.implement = func(_ context.Context, _ store.ImplementationRequest) (store.ImplementationResult, error) {
		harness.workspace.session.current.Current.Fingerprint = "implemented:two files"
		harness.workspace.session.current.ChangedFiles = []string{"a.go", "b.go"}
		harness.workspace.session.current.Diff = twoFileDiff
		return store.ImplementationResult{Summary: "initial implementation", ChangedFiles: []string{"a.go", "b.go"}}, nil
	}
	harness.reviewer.reviews = []store.Review{
		{Approved: true, Summary: "chunk one ok"},
		{Approved: false, Summary: "chunk two bad", Findings: []store.ReviewFinding{{
			Severity: store.FindingSeverityError, Blocking: true, File: "b.go",
			Description: "defect", Evidence: "wrong", RequiredAction: "fix",
		}}},
		{Approved: true, Summary: "synthesis approves anyway"},
	}
	output := harness.service.Run(t.Context(), validTask(0))
	if output.Status == store.TaskStatusApproved {
		t.Fatalf("approved despite a blocking chunk finding: %+v", output)
	}
	if len(harness.reviewer.requests) < 2 {
		t.Fatalf("expected chunked reviewer calls, got %d", len(harness.reviewer.requests))
	}
}

func TestReviewStopsBeforeExecutionWhenChunksExceedPolicy(t *testing.T) {
	harness := newWorkflowHarness(t)
	svc, err := workflow.NewService(workflow.Dependencies{
		Workspace: harness.workspace, Planner: harness.planner,
		Implementer: harness.implementer, Validator: harness.validator,
		Reviewer: harness.reviewer, Events: harness.events,
		Execution: workflow.ExecutionPolicy{MaxAgentInvocations: 3, MaxPromptBytes: 2048, ReviewChunkBytes: 64},
	})
	if err != nil {
		t.Fatal(err)
	}
	harness.service = svc
	bigDiff := strings.Repeat("--- before/a.go\n+++ after/a.go\n@@ -1 +1 @@\n-a\n+b\n", 50)
	harness.implementer.implement = func(_ context.Context, _ store.ImplementationRequest) (store.ImplementationResult, error) {
		harness.workspace.session.current.Current.Fingerprint = "implemented:big"
		harness.workspace.session.current.ChangedFiles = []string{"a.go"}
		harness.workspace.session.current.Diff = bigDiff
		return implementation("initial implementation", "a.go"), nil
	}
	// Tight invocation budget: chunking would need many calls.
	callsBefore := len(harness.reviewer.requests)
	output := harness.service.Run(t.Context(), validTask(0))
	if len(harness.reviewer.requests) != callsBefore {
		t.Fatal("provider execution started despite exceeding invocation policy")
	}
	if output.Status == store.TaskStatusApproved {
		t.Fatal("approved without reviewing every chunk")
	}
}

type batchFakeReviewer struct {
	*fakeReviewer
	chunks    []store.ReviewChunk
	collected [][]store.ReviewFinding
}

func (f *batchFakeReviewer) ReviewChunk(ctx context.Context, request store.ReviewRequest, chunk store.ReviewChunk) (store.Review, error) {
	f.chunks = append(f.chunks, chunk)
	return f.Review(ctx, request)
}

func (f *batchFakeReviewer) ReviewSynthesis(ctx context.Context, request store.ReviewRequest, findings []store.ReviewFinding, _ []string) (store.Review, error) {
	f.collected = append(f.collected, findings)
	return f.Review(ctx, request)
}

// A BatchReviewer sees chunk positions and a synthesis over collected
// findings; synthesis approval cannot override a chunk rejection, and a
// synthesis echo of a chunk finding is not duplicated for repair.
func TestBatchReviewerSynthesisCannotOverrideChunkRejection(t *testing.T) {
	harness := newWorkflowHarness(t)
	reviewer := &batchFakeReviewer{fakeReviewer: harness.reviewer}
	svc, err := workflow.NewService(workflow.Dependencies{
		Workspace: harness.workspace, Planner: harness.planner,
		Implementer: harness.implementer, Validator: harness.validator,
		Reviewer: reviewer, Events: harness.events,
		Execution: workflow.ExecutionPolicy{MaxAgentInvocations: 64, MaxPromptBytes: maxPromptForTest(64), ReviewChunkBytes: 60},
	})
	if err != nil {
		t.Fatal(err)
	}
	harness.implementer.implement = func(_ context.Context, _ store.ImplementationRequest) (store.ImplementationResult, error) {
		harness.workspace.session.current.Current.Fingerprint = "implemented:two files"
		harness.workspace.session.current.ChangedFiles = []string{"a.go", "b.go"}
		harness.workspace.session.current.Diff = "--- \"before/a.go\"\n+++ \"after/a.go\"\n@@ -1 +1 @@\n-a\n+b\n" +
			"--- \"before/b.go\"\n+++ \"after/b.go\"\n@@ -1 +1 @@\n-c\n+d\n"
		return store.ImplementationResult{Summary: "initial implementation", ChangedFiles: []string{"a.go", "b.go"}}, nil
	}
	defect := store.ReviewFinding{Severity: store.FindingSeverityError, Blocking: true, File: "b.go", Description: "defect", Evidence: "wrong", RequiredAction: "fix"}
	harness.reviewer.reviews = []store.Review{
		{Approved: true, Summary: "chunk one ok"},
		{Approved: false, Summary: "chunk two bad", Findings: []store.ReviewFinding{defect}},
		{Approved: true, Summary: "synthesis approves anyway", Findings: []store.ReviewFinding{}},
	}
	result := svc.Run(t.Context(), validTask(0))
	if result.Status == store.TaskStatusApproved {
		t.Fatalf("approved despite a blocking chunk finding: %+v", result)
	}
	if len(reviewer.chunks) != 2 || reviewer.chunks[1].ChunkIndex != 1 || reviewer.chunks[1].ChunkCount != 2 ||
		!reflect.DeepEqual(reviewer.chunks[1].ChangedFiles, []string{"b.go"}) {
		t.Fatalf("chunks not attributed: %+v", reviewer.chunks)
	}
	if len(reviewer.collected) != 1 || len(reviewer.collected[0]) != 1 || reviewer.collected[0][0].Description != "defect" {
		t.Fatalf("synthesis did not receive collected findings: %+v", reviewer.collected)
	}
}
