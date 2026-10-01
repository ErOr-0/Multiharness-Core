package structured_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
)

func testPlan() store.Plan {
	return store.Plan{Action: store.PlanActionImplement, Summary: "do it", Steps: []string{"edit"}, AcceptanceCriteria: []string{"tests pass"}}
}

func testInput() store.TaskInput {
	return store.TaskInput{Task: "do it", WorkingDir: "/w"}
}

func TestImplementationPromptOmitsPreExistingList(t *testing.T) {
	pre := make([]string, 20000)
	for i := range pre {
		pre[i] = "file.txt"
	}
	req := store.ImplementationRequest{Input: testInput(), Plan: testPlan(), Repository: &store.RepositoryEvidence{
		Baseline: store.RepositoryState{Root: "/w", Fingerprint: "b"}, Current: store.RepositoryState{Root: "/w", Fingerprint: "c"},
		Complete: true, PreExistingFiles: pre, Diff: strings.Repeat("x", 1<<20),
	}}
	prompt, err := structured.ImplementationPrompt(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt) > 262144 {
		t.Fatalf("implementation prompt %d bytes exceeds default budget", len(prompt))
	}
	if strings.Contains(prompt, `"pre_existing_files"`) || strings.Contains(prompt, `"diff"`) {
		t.Fatal("implementation prompt carries unbounded evidence")
	}
	if !strings.Contains(prompt, `"pre_existing_file_count":20000`) {
		t.Fatal("pre-existing count lost")
	}
}

func TestReviewOneLineChangeStaysBounded(t *testing.T) {
	large := strings.Repeat("same line\n", 200000) // >4MiB single file content would diff huge under old full-copy
	_ = large
	// One-line change expressed as a small unified hunk, not full before/after copies.
	diff := "--- before/big.go\n+++ after/big.go\n@@ -100000,5 +100000,5 @@\n ctx\n-old\n+new\n ctx2\n ctx3\n"
	req := store.ReviewRequest{Input: testInput(), Plan: testPlan(),
		Implementation: store.ImplementationResult{Summary: "one line", ChangedFiles: []string{"big.go"}},
		Validation:     store.ValidationReport{Passed: true, Checks: []store.ValidationEvidence{{Command: "go test ./...", Passed: true, Output: "ok"}}},
		Repository: &store.RepositoryEvidence{
			Baseline: store.RepositoryState{Root: "/w", Fingerprint: "b"}, Current: store.RepositoryState{Root: "/w", Fingerprint: "c"},
			Complete: true, ChangedFiles: []string{"big.go"}, Diff: diff,
		}}
	prompt, err := structured.ReviewPrompt(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt) > 262144 {
		t.Fatalf("review prompt %d bytes exceeds budget", len(prompt))
	}
	if !strings.Contains(prompt, "@@") {
		t.Fatal("expected unified diff with context")
	}
}

func TestReviewLargeRewriteSplitsChunks(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString("--- before/f.go\n+++ after/f.go\n@@ -1 +1 @@\n-a\n+b\n")
	}
	req := store.ReviewRequest{Input: testInput(), Plan: testPlan(),
		Implementation: store.ImplementationResult{Summary: "rewrite", ChangedFiles: []string{"f.go"}},
		Validation:     store.ValidationReport{Passed: true, Checks: []store.ValidationEvidence{{Command: "go test ./...", Passed: true, Output: "ok"}}},
		Repository: &store.RepositoryEvidence{
			Baseline: store.RepositoryState{Root: "/w", Fingerprint: "b"}, Current: store.RepositoryState{Root: "/w", Fingerprint: "c"},
			Complete: true, ChangedFiles: []string{"f.go"}, Diff: strings.Repeat(b.String(), 200),
		}}
	small := structured.Budget{MaxPromptBytes: 16384, ReviewChunkBytes: 1024}
	if _, err := structured.ReviewPromptWithBudget(req, small); err == nil {
		t.Fatal("expected HandoffTooLargeError for oversized review")
	} else {
		var tooLarge *structured.HandoffTooLargeError
		if !errors.As(err, &tooLarge) {
			t.Fatalf("wrong error type: %T %v", err, err)
		}
	}
	chunks := store.ChunkReviewDiff(req.Repository.Diff, req.Implementation.ChangedFiles, small.ReviewChunkBytes)
	if len(chunks) < 2 {
		t.Fatal("expected multiple chunks")
	}
	for _, c := range chunks {
		p, err := structured.ReviewChunkPrompt(req, c, small)
		if err != nil {
			t.Fatalf("chunk prompt over budget: %v", err)
		}
		if len(p) > small.MaxPromptBytes {
			t.Fatalf("chunk %d bytes exceeds limit", len(p))
		}
	}
}

func TestProviderNeverStartedWhenOverBudget(t *testing.T) {
	started := false
	agent := structured.Agent{
		Execute: func(_ context.Context, _ structured.Invocation) (structured.Response, error) {
			started = true
			return structured.Response{}, errors.New("must not execute")
		},
		CanWrite: true,
		Budget:   structured.Budget{MaxPromptBytes: 64, ReviewChunkBytes: 32},
	}
	req := store.ImplementationRequest{Input: testInput(), Plan: testPlan()}
	if _, err := agent.Implement(context.Background(), req); err == nil {
		t.Fatal("expected budget failure")
	} else {
		var tooLarge *structured.HandoffTooLargeError
		if !errors.As(err, &tooLarge) {
			t.Fatalf("wrong error: %T %v", err, err)
		}
	}
	if started {
		t.Fatal("provider execution started despite over-budget prompt")
	}
}

func TestPlanningPromptEnforcesBudgetBeforeExecution(t *testing.T) {
	started := false
	agent := structured.Agent{
		Execute: func(_ context.Context, _ structured.Invocation) (structured.Response, error) {
			started = true
			return structured.Response{}, errors.New("must not execute")
		},
		Budget: structured.Budget{MaxPromptBytes: 64, ReviewChunkBytes: 32},
	}
	if _, err := agent.Plan(context.Background(), testInput()); err == nil {
		t.Fatal("expected budget failure")
	} else {
		var tooLarge *structured.HandoffTooLargeError
		if !errors.As(err, &tooLarge) {
			t.Fatalf("wrong error: %T %v", err, err)
		}
	}
	if started {
		t.Fatal("provider execution started despite over-budget planning prompt")
	}
}

func TestRepairPromptIsDeltaOriented(t *testing.T) {
	diff := "--- before/a.go\n+++ after/a.go\n@@ -1 +1 @@\n-a\n+b\n--- before/other.go\n+++ after/other.go\n@@ -1 +1 @@\n-x\n+y\n"
	repair := store.RepairRequest{
		Input: testInput(), Plan: testPlan(),
		Implementation: store.ImplementationResult{Summary: "impl", ChangedFiles: []string{"a.go", "other.go"}},
		Validation: store.ValidationReport{Passed: false, Checks: []store.ValidationEvidence{
			{Command: "go test ./...", Passed: false, ExitCode: 1, Output: "fail detail"},
			{Command: "go vet ./...", Passed: true, Output: "ok"},
		}},
		Review: store.Review{Approved: false, Summary: "bad", Findings: []store.ReviewFinding{
			{Severity: store.FindingSeverityError, Blocking: true, File: "a.go", Description: "fix a", RequiredAction: "edit a"},
			{Severity: store.FindingSeverityInfo, Blocking: false, Description: "nit"},
		}},
		Repository: &store.RepositoryEvidence{
			Baseline: store.RepositoryState{Root: "/w", Fingerprint: "b"}, Current: store.RepositoryState{Root: "/w", Fingerprint: "c"},
			Complete: true, ChangedFiles: []string{"a.go", "other.go"}, Diff: diff,
		},
	}
	prompt, err := structured.RepairPrompt(repair)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "other.go\n-x") || strings.Contains(prompt, `"blocking":false`) {
		t.Fatal("repair prompt carries cumulative/non-blocking evidence")
	}
	if !strings.Contains(prompt, "a.go") || !strings.Contains(prompt, "fail detail") {
		t.Fatal("repair prompt missing blocking context")
	}
}
