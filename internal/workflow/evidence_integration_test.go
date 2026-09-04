package workflow_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	validationadapter "multiharness-core/internal/adapter/validation"
	gitworkspace "multiharness-core/internal/adapter/workspace/git"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type evidenceImplementer struct{ t *testing.T }

func (agent evidenceImplementer) Implement(_ context.Context, request store.ImplementationRequest) (store.ImplementationResult, error) {
	if !reflect.DeepEqual(request.Repository.PreExistingFiles, []string{"notes.txt"}) {
		agent.t.Fatal("protected context missing")
	}
	err := os.WriteFile(filepath.Join(request.Input.WorkingDir, "result.txt"), []byte("broken\n"), 0644)
	return store.ImplementationResult{Summary: "implemented", ChangedFiles: []string{"invented.txt"}, AgentSessionID: "session"}, err
}
func (agent evidenceImplementer) ApplyReview(_ context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	if request.Validation.Passed || request.Implementation.AgentSessionID != "session" || !strings.Contains(request.Repository.Diff, "+broken") {
		agent.t.Fatal("repair evidence missing")
	}
	err := os.WriteFile(filepath.Join(request.Input.WorkingDir, "result.txt"), []byte("fixed\n"), 0644)
	return store.ImplementationResult{Summary: "repaired", ChangedFiles: []string{"another-invented.txt"}, AgentSessionID: "session"}, err
}

type evidenceReviewer struct{ t *testing.T }

func (reviewer evidenceReviewer) Review(_ context.Context, request store.ReviewRequest) (store.Review, error) {
	if !request.Repository.Complete || len(request.Repository.PreservationViolations) != 0 || !reflect.DeepEqual(request.Implementation.ChangedFiles, []string{"result.txt"}) {
		reviewer.t.Fatalf("untrusted review input: %#v", request)
	}
	if strings.Contains(request.Repository.Diff, "private notes") {
		reviewer.t.Fatal("pre-existing notes attributed to workflow")
	}
	if request.Validation.Passed {
		return store.Review{Approved: true, Summary: "verified"}, nil
	}
	return store.Review{Summary: "check failed", Findings: []store.ReviewFinding{{Severity: store.FindingSeverityError, Blocking: true, Description: "result is broken", Evidence: request.Validation.Checks[0].Output, RequiredAction: "write fixed"}}}, nil
}

func TestServiceUsesRealRepositoryEvidenceAndValidationAcrossRepair(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	dir := t.TempDir()
	runner := process.NewOSRunner()
	git := func(args ...string) {
		t.Helper()
		result, err := runner.Run(t.Context(), process.Command{Name: "git", Dir: dir, Args: append([]string{"-c", "user.name=Tests", "-c", "user.email=tests@example.invalid", "-c", "commit.gpgsign=false"}, args...)})
		if err != nil {
			t.Fatalf("git: %v %s", err, result.Stderr)
		}
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "result.txt"), []byte("before\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "baseline")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("private notes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	workspace, err := gitworkspace.NewWorkspace(runner, gitworkspace.Config{})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := validationadapter.NewValidator(runner, validationadapter.Config{Checks: []validationadapter.Check{{Executable: "sh", Args: []string{"-c", `echo checked; test "$(cat result.txt)" = fixed`}}}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := workflow.NewService(workflow.Dependencies{
		Workspace: workspace, Planner: &fakePlanner{plan: validPlan()}, Implementer: evidenceImplementer{t}, Validator: validator, Reviewer: evidenceReviewer{t},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := service.Run(t.Context(), store.TaskInput{Task: "fix result", WorkingDir: dir, MaxRepairAttempts: 1})
	if output.Status != store.TaskStatusApproved || output.RepairAttempts != 1 {
		t.Fatalf("output: %#v", output)
	}
	if err := output.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.Repository.Diff, "-before") || !strings.Contains(output.Repository.Diff, "+fixed") || strings.Contains(output.Repository.Diff, "broken") {
		t.Fatalf("diff did not retain original baseline: %s", output.Repository.Diff)
	}
	notes, _ := os.ReadFile(filepath.Join(dir, "notes.txt"))
	if string(notes) != "private notes\n" {
		t.Fatal("user notes changed")
	}
	lease, err := workspace.Acquire(t.Context(), dir)
	if err != nil {
		t.Fatalf("service leaked workspace lock: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
}
