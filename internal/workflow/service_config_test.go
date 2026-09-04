package workflow_test

import (
	"errors"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func TestNewServiceRequiresEveryOutboundPort(t *testing.T) {
	tests := []struct {
		name    string
		missing string
		remove  func(*workflow.Dependencies)
	}{
		{name: "workspace", missing: "workspace", remove: func(dependencies *workflow.Dependencies) {
			dependencies.Workspace = nil
		}},
		{name: "planner", missing: "planner", remove: func(dependencies *workflow.Dependencies) {
			dependencies.Planner = nil
		}},
		{name: "implementer", missing: "implementer", remove: func(dependencies *workflow.Dependencies) {
			dependencies.Implementer = nil
		}},
		{name: "validator", missing: "validator", remove: func(dependencies *workflow.Dependencies) {
			dependencies.Validator = nil
		}},
		{name: "reviewer", missing: "reviewer", remove: func(dependencies *workflow.Dependencies) {
			dependencies.Reviewer = nil
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dependencies := completeDependencies()
			test.remove(&dependencies)

			service, err := workflow.NewService(dependencies)
			if service != nil {
				t.Fatal("NewService() returned a service with a missing dependency")
			}
			var dependencyError *workflow.DependencyError
			if !errors.As(err, &dependencyError) {
				t.Fatalf("NewService() error = %T %v, want *workflow.DependencyError", err, err)
			}
			if dependencyError.Name != test.missing {
				t.Fatalf("DependencyError.Name = %q, want %q", dependencyError.Name, test.missing)
			}
		})
	}
}

func TestNewServiceUsesDiscardSinkWhenEventsAreOmitted(t *testing.T) {
	dependencies := completeDependencies()
	dependencies.Events = nil

	service, err := workflow.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	output := service.Run(t.Context(), validTask(0))
	if err := output.Validate(); err != nil {
		t.Fatalf("Run() output validation error = %v", err)
	}
}

func completeDependencies() workflow.Dependencies {
	return workflow.Dependencies{
		Workspace:   &fakeWorkspace{},
		Planner:     &fakePlanner{plan: validPlan()},
		Implementer: &fakeImplementer{initial: implementation("implemented", "service.go")},
		Validator:   &fakeValidator{reports: []store.ValidationReport{passingValidation()}},
		Reviewer:    &fakeReviewer{reviews: []store.Review{approvedReview("approved")}},
	}
}
