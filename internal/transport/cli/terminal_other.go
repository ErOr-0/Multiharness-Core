//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package cli

import (
	"io"
	"os"

	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/workflow"
)

func NewTerminalApprover(_ *os.File, _ io.Writer) workflow.BillingApprover { return nil }

func NewTerminalInstaller(_ *os.File, _ io.Writer) setup.Confirmation { return nil }

func terminalSize(io.Writer) (int, bool) { return 0, false }
