package workflow_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"multiharness-core/internal/contract"
)

func TestRunReturnsStructuredPortFailuresWithAvailableEvidence(t *testing.T) {
	portError := errors.New("port failed")
	tests := []struct {
		name               string
		input              contract.TaskInput
		setup              func(*workflowHarness)
		wantStage          contract.WorkflowStage
		wantCode           contract.FailureCode
		wantCalls          []string
		wantPlan           bool
		wantImplementation bool
		wantValidation     bool
		wantReview         bool
	}{
		{
			name:      "workspace",
			input:     validTask(0),
			setup:     func(harness *workflowHarness) { harness.workspace.acquireErr = portError },
			wantStage: contract.WorkflowStageImplementation,
			wantCode:  contract.FailureCodeWorkspace,
			wantCalls: []string{"plan", "workspace"},
			wantPlan:  true,
		},
		{
			name:      "planner",
			input:     validTask(0),
			setup:     func(harness *workflowHarness) { harness.planner.err = portError },
			wantStage: contract.WorkflowStagePlanning,
			wantCode:  contract.FailureCodeAgent,
			wantCalls: []string{"plan"},
		},
		{
			name:      "initial implementer",
			input:     validTask(0),
			setup:     func(harness *workflowHarness) { harness.implementer.initialErr = portError },
			wantStage: contract.WorkflowStageImplementation,
			wantCode:  contract.FailureCodeAgent,
			wantCalls: []string{"plan", "workspace", "implement"},
			wantPlan:  true,
		},
		{
			name:               "validator",
			input:              validTask(0),
			setup:              func(harness *workflowHarness) { harness.validator.err = portError },
			wantStage:          contract.WorkflowStageValidation,
			wantCode:           contract.FailureCodeValidation,
			wantCalls:          []string{"plan", "workspace", "implement", "validate"},
			wantPlan:           true,
			wantImplementation: true,
		},
		{
			name:               "reviewer",
			input:              validTask(0),
			setup:              func(harness *workflowHarness) { harness.reviewer.err = portError },
			wantStage:          contract.WorkflowStageReview,
			wantCode:           contract.FailureCodeAgent,
			wantCalls:          []string{"plan", "workspace", "implement", "validate", "review"},
			wantPlan:           true,
			wantImplementation: true,
			wantValidation:     true,
		},
		{
			name:  "repair implementer",
			input: validTask(1),
			setup: func(harness *workflowHarness) {
				harness.reviewer.reviews = []contract.Review{rejectedReview("repair required")}
				harness.implementer.repairErr = portError
			},
			wantStage:          contract.WorkflowStageRepair,
			wantCode:           contract.FailureCodeAgent,
			wantCalls:          []string{"plan", "workspace", "implement", "validate", "review", "repair"},
			wantPlan:           true,
			wantImplementation: true,
			wantValidation:     true,
			wantReview:         true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newWorkflowHarness(t)
			test.setup(harness)

			output := harness.service.Run(t.Context(), test.input)

			if output.Status != contract.TaskStatusFailed {
				t.Fatalf("Run() status = %q, want %q", output.Status, contract.TaskStatusFailed)
			}
			if output.Failure == nil {
				t.Fatal("Run() failure = nil, want structured failure")
			}
			if output.Failure.Stage != test.wantStage || output.Failure.Code != test.wantCode {
				t.Fatalf("Run() failure = %#v, want stage %q and code %q", output.Failure, test.wantStage, test.wantCode)
			}
			if !strings.Contains(output.Failure.Message, portError.Error()) {
				t.Fatalf("Run() failure message = %q, want underlying error", output.Failure.Message)
			}
			if err := output.Validate(); err != nil {
				t.Fatalf("Run() output validation error = %v", err)
			}
			if got := harness.calls.snapshot(); !reflect.DeepEqual(got, test.wantCalls) {
				t.Fatalf("call order = %v, want %v", got, test.wantCalls)
			}
			if (output.Plan != nil) != test.wantPlan ||
				(output.Implementation != nil) != test.wantImplementation ||
				(output.Validation != nil) != test.wantValidation ||
				(output.LastReview != nil) != test.wantReview {
				t.Fatalf(
					"evidence presence = plan:%t implementation:%t validation:%t review:%t",
					output.Plan != nil,
					output.Implementation != nil,
					output.Validation != nil,
					output.LastReview != nil,
				)
			}
		})
	}
}

func TestRunRejectsMalformedOrContradictoryPortOutput(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*workflowHarness)
		wantStage contract.WorkflowStage
	}{
		{
			name:      "planner output",
			setup:     func(harness *workflowHarness) { harness.planner.plan = contract.Plan{} },
			wantStage: contract.WorkflowStagePlanning,
		},
		{
			name: "implementer output",
			setup: func(harness *workflowHarness) {
				harness.implementer.initial = contract.ImplementationResult{}
			},
			wantStage: contract.WorkflowStageImplementation,
		},
		{
			name: "validator output",
			setup: func(harness *workflowHarness) {
				harness.validator.reports = []contract.ValidationReport{{Passed: false}}
			},
			wantStage: contract.WorkflowStageValidation,
		},
		{
			name: "reviewer output",
			setup: func(harness *workflowHarness) {
				harness.reviewer.reviews = []contract.Review{{Summary: "rejected without a finding"}}
			},
			wantStage: contract.WorkflowStageReview,
		},
		{
			name: "approval contradicts failed validation",
			setup: func(harness *workflowHarness) {
				harness.validator.reports = []contract.ValidationReport{failingValidation()}
				harness.reviewer.reviews = []contract.Review{approvedReview("incorrect approval")}
			},
			wantStage: contract.WorkflowStageReview,
		},
		{
			name: "repair output",
			setup: func(harness *workflowHarness) {
				harness.reviewer.reviews = []contract.Review{rejectedReview("repair required")}
				harness.implementer.repairs = []contract.ImplementationResult{{}}
			},
			wantStage: contract.WorkflowStageRepair,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newWorkflowHarness(t)
			test.setup(harness)

			output := harness.service.Run(t.Context(), validTask(1))

			if output.Status != contract.TaskStatusFailed || output.Failure == nil {
				t.Fatalf("Run() output = %#v, want failed output", output)
			}
			if output.Failure.Stage != test.wantStage {
				t.Fatalf("Run() failure stage = %q, want %q", output.Failure.Stage, test.wantStage)
			}
			if output.Failure.Code != contract.FailureCodeInvalidOutput {
				t.Fatalf("Run() failure code = %q, want %q", output.Failure.Code, contract.FailureCodeInvalidOutput)
			}
			if err := output.Validate(); err != nil {
				t.Fatalf("Run() output validation error = %v", err)
			}
		})
	}
}

func TestRunRejectsInvalidInputBeforeCallingPorts(t *testing.T) {
	harness := newWorkflowHarness(t)
	input := validTask(0)
	input.Task = "  "

	output := harness.service.Run(t.Context(), input)

	if output.Status != contract.TaskStatusFailed || output.Failure == nil {
		t.Fatalf("Run() output = %#v, want failed output", output)
	}
	if output.Failure.Stage != contract.WorkflowStageIntake || output.Failure.Code != contract.FailureCodeInvalidInput {
		t.Fatalf("Run() failure = %#v, want intake invalid-input failure", output.Failure)
	}
	if got := harness.calls.snapshot(); len(got) != 0 {
		t.Fatalf("port calls = %v, want none", got)
	}
}

func TestRunRejectsNilContextAsInternalFailure(t *testing.T) {
	harness := newWorkflowHarness(t)

	//lint:ignore SA1012 a nil context must be rejected, not dereferenced
	output := harness.service.Run(nil, validTask(0))

	if output.Status != contract.TaskStatusFailed || output.Failure == nil {
		t.Fatalf("Run() output = %#v, want failed output", output)
	}
	if output.Failure.Stage != contract.WorkflowStageIntake || output.Failure.Code != contract.FailureCodeInternal {
		t.Fatalf("Run() failure = %#v, want intake internal failure", output.Failure)
	}
	if got := harness.calls.snapshot(); len(got) != 0 {
		t.Fatalf("port calls = %v, want none", got)
	}
}
