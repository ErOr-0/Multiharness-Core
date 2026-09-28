//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package cli

import (
	"context"
	"errors"
	"io"
	"os"

	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func NewTerminalApprover(_ *os.File, _ io.Writer) workflow.BillingApprover { return nil }

func NewTerminalNativeApprover(_ *os.File, _ io.Writer) store.NativeApprover { return nil }

func NewTerminalPermissionResolver(_ *os.File, _ io.Writer) workflow.PermissionResolver { return nil }

func NewTerminalInstaller(_ *os.File, _ io.Writer) setup.Confirmation { return nil }

func terminalSize(io.Writer) (int, bool) { return 0, false }

func terminalDimensions(io.Writer) (int, int, bool) { return 0, 0, false }

func NewTerminalInput(_ *os.File, _ io.Writer) (LineInput, error) {
	return nil, errors.New("interactive magent requires a supported Unix terminal")
}

func NewTerminalWorkspaceApprover(_ *os.File, _ io.Writer) workflow.WorkspaceApprover { return nil }

func NewTerminalValidationApprover(_ *os.File, _ io.Writer) workflow.ValidationApprover { return nil }

func NewTerminalDecisionKeyPrompt(_ *os.File, _ io.Writer) func(context.Context) (string, error) {
	return nil
}
