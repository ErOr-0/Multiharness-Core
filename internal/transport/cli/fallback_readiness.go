package cli

import (
	"context"
	"fmt"
	"io"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/screen"
	"multiharness-core/internal/transport/cli/term"
	"multiharness-core/internal/workflow"
)

// FallbackReadiness checks an optional account only after the user accepts a
// specific switch. An unavailable alternate leaves the original failure intact.
type FallbackReadiness struct {
	Config   config.Config
	Approver workflow.BillingApprover
	Check    func(context.Context, account.Request) account.Status
	Output   io.Writer
}

func (f FallbackReadiness) ConfirmFallback(ctx context.Context, choice contract.AgentSwitch) (bool, error) {
	if f.Approver == nil {
		return false, nil
	}
	yes, err := f.Approver.ConfirmFallback(ctx, choice)
	if err != nil || !yes {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	role := ""
	switch choice.Stage {
	case contract.WorkflowStagePlanning, contract.WorkflowStageAnswering:
		role = "fallback planner"
	case contract.WorkflowStageReview:
		role = "fallback reviewer"
	case contract.WorkflowStageImplementation, contract.WorkflowStageRepair:
		role = "fallback implementer"
	}
	for _, item := range optionalFallbacks(f.Config) {
		if item.Role != role {
			continue
		}
		request := account.Request{Harness: item.Agent.Harness, Executable: item.Agent.Executable, Model: item.Agent.Model, Directory: f.Config.WorkingDir, InstallMode: f.Config.InstallMode}
		if choice.To != screen.HarnessName(request.Harness) {
			return false, fmt.Errorf("fallback selection does not match configuration")
		}
		if f.Check == nil {
			return false, fmt.Errorf("fallback account check unavailable")
		}
		status := f.Check(ctx, request)
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if status.Ready {
			return true, nil
		}
		if f.Output != nil {
			detail := status.Detail
			if request.Harness == "opencode" && request.Model == "" {
				option := "fallback-planner-model"
				if role == "fallback reviewer" {
					option = "fallback-opencode-reviewer-model"
				}
				detail = "Choose the alternate provider/model with /set " + option + " provider/model."
			}
			if err := term.Write(f.Output, "\nOptional fallback was not started. "+terminalText(detail)+" Use /login "+request.Harness+" to configure its account. Your task will not restart automatically.\n"); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("fallback is not enabled for this role")
}
