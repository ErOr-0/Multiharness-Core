package workflow_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	folder "multiharness-core/internal/adapter/workspace/folder"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type workspaceApproval func(context.Context, store.ExistingWork) (bool, error)

func (f workspaceApproval) ConfirmExistingWork(ctx context.Context, work store.ExistingWork) (bool, error) {
	return f(ctx, work)
}

func TestPreparationRetriesConcurrentEditWithFreshBackupAndConsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("folder leases require Unix")
	}
	for _, mode := range []string{"snapshot", "prompt", "preserve"} {
		t.Run(mode, func(t *testing.T) {
			dir, backups := t.TempDir(), t.TempDir()
			file := filepath.Join(dir, "user.txt")
			if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			scans, approvals := 0, 0
			w, err := folder.NewWorkspaceWithApproval(folder.Config{ExistingWork: mode, RecoveryDir: backups, Observe: func(p folder.ScanProgress) {
				if p.Phase == "listing files (pass 1/2)" && p.Files == 0 {
					scans++
				}
				// Baseline and backup verification succeeded. An external save
				// occurs between the two passes of the pre-implementation check.
				if scans == 3 && p.Phase == "listing files (pass 2/2)" && p.Files == 0 {
					if err := os.WriteFile(file, []byte("new user edit"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}}, workspaceApproval(func(_ context.Context, work store.ExistingWork) (bool, error) {
				approvals++
				data, err := os.ReadFile(filepath.Join(work.RecoveryDirectory, "files", "user.txt"))
				want := "original"
				if approvals == 2 {
					want = "new user edit"
				}
				if err != nil || string(data) != want {
					t.Fatal("consent did not cover fresh backup", string(data), err)
				}
				return true, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			h := newWorkflowHarness(t)
			h.implementer.workspace = nil
			h.implementer.implement = func(_ context.Context, req store.ImplementationRequest) (store.ImplementationResult, error) {
				data, err := os.ReadFile(filepath.Join(req.Repository.RecoveryDirectory, "files", "user.txt"))
				if err != nil || string(data) != "new user edit" || len(req.Repository.ChangedFiles) != 0 {
					t.Fatal("implementer received stale baseline", string(data), err)
				}
				return implementation("done", "result.txt"), os.WriteFile(filepath.Join(dir, "result.txt"), []byte("done"), 0600)
			}
			svc, err := workflow.NewService(workflow.Dependencies{Workspace: w, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, Events: h.events})
			if err != nil {
				t.Fatal(err)
			}
			input := validTask(0)
			input.WorkingDir = dir
			out := svc.Run(t.Context(), input)
			if out.Status != store.TaskStatusApproved || len(h.implementer.implementationCalls) != 1 || out.AgentInvocations != 3 {
				t.Fatalf("retry replayed agents or failed: %+v; failure: %+v", out, out.Failure)
			}
			if mode == "prompt" && approvals != 2 {
				t.Fatalf("stale approval reused: %d", approvals)
			}
			if strings.Contains(out.Repository.Diff, "user edit") {
				t.Fatal("external edit attributed to implementer")
			}
			data, _ := os.ReadFile(file)
			if string(data) != "new user edit" {
				t.Fatal("external work overwritten")
			}
		})
	}
}

func TestPreparationRetryStopsOnIOCancellationAndReleaseError(t *testing.T) {
	for _, mode := range []string{"io", "cancel", "release"} {
		t.Run(mode, func(t *testing.T) {
			h := newWorkflowHarness(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			h.workspace.acquire = func(context.Context, string) error {
				calls++
				s := newFakeWorkspaceSession()
				h.workspace.session = s
				s.inspect = func(context.Context) (store.RepositoryEvidence, error) {
					if mode == "io" {
						return s.current, errors.New("permission denied")
					}
					if mode == "cancel" {
						cancel()
					}
					return s.current, &store.WorkspaceChangedError{During: "between inspection passes", Files: []string{"user.txt"}}
				}
				if mode == "release" {
					s.closeErr = errors.New("release failed")
				}
				return nil
			}
			out := h.service.Run(ctx, validTask(0))
			if calls != 1 || len(h.implementer.implementationCalls) != 0 || out.Status == store.TaskStatusApproved {
				t.Fatal("unsafe retry", calls, out)
			}
		})
	}
}

func TestPreparationDoesNotReuseDeclinedSecondApproval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("folder leases require Unix")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "user.txt")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	approvals := 0
	w, err := folder.NewWorkspaceWithApproval(folder.Config{ExistingWork: "prompt", RecoveryDir: t.TempDir()}, workspaceApproval(func(context.Context, store.ExistingWork) (bool, error) {
		approvals++
		if approvals == 1 {
			return true, os.WriteFile(file, []byte("new edit during consent"), 0600)
		}
		return false, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	h := newWorkflowHarness(t)
	svc, err := workflow.NewService(workflow.Dependencies{Workspace: w, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer})
	if err != nil {
		t.Fatal(err)
	}
	input := validTask(0)
	input.WorkingDir = dir
	out := svc.Run(t.Context(), input)
	if out.Status != store.TaskStatusFailed || approvals != 2 || len(h.implementer.implementationCalls) != 0 || !strings.Contains(out.Failure.Message, "stopped before editing") {
		t.Fatalf("declined fresh approval was bypassed: %+v; approvals %d", out, approvals)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "new edit during consent" {
		t.Fatal("new user work was lost")
	}
}

func TestUnstableEvidenceAfterImplementationNeverRebases(t *testing.T) {
	h := newWorkflowHarness(t)
	acquisitions := 0
	h.workspace.acquire = func(context.Context, string) error { acquisitions++; return nil }
	h.implementer.implement = func(context.Context, store.ImplementationRequest) (store.ImplementationResult, error) {
		s := h.workspace.session
		s.inspect = func(context.Context) (store.RepositoryEvidence, error) {
			return s.current, &store.WorkspaceChangedError{During: "between inspection passes", Files: []string{"user.txt"}}
		}
		return implementation("changed", "user.txt"), nil
	}
	out := h.service.Run(t.Context(), validTask(0))
	if out.Status != store.TaskStatusFailed || acquisitions != 1 || len(h.implementer.implementationCalls) != 1 || !strings.Contains(out.Failure.Message, "user.txt") {
		t.Fatal("post-edit instability was retried or lost diagnostics", out)
	}
}
