//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package cli

import (
	"errors"
	"io"
	"os"

	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/workflow"
)

func NewTerminalApprover(_ *os.File, _ io.Writer) workflow.BillingApprover { return nil }

func NewTerminalInstaller(_ *os.File, _ io.Writer) setup.Confirmation { return nil }

func terminalSize(io.Writer) (int, bool) { return 0, false }

func NewTerminalInput(_ *os.File, _ io.Writer) (LineInput, error) {
	return nil, errors.New("interactive magent requires a supported Unix terminal")
}

func NewTerminalWorkspaceApprover(_ *os.File, _ io.Writer) workflow.WorkspaceApprover { return nil }
