//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package console

import (
	"context"
	"errors"
	"io"
	"os"

	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

var errUnsupported = errors.New("interactive magent requires a supported Unix terminal")

// Reader has no implementation on this platform: prompts are never available
// and every approver constructor returns nil, which callers treat as refusal.
type Reader struct{}

func (*Reader) ReadLine(context.Context, int) (string, error) { return "", errUnsupported }

func NewReader(_ *os.File, _ io.Writer) *Reader { return &Reader{} }

func NewInput(_ *os.File, _ io.Writer) (*Reader, error) { return nil, errUnsupported }

func NewApprover(_ *os.File, _ io.Writer) workflow.BillingApprover { return nil }

func NewNativeApprover(_ *os.File, _ io.Writer) contract.NativeApprover { return nil }

func NewPermissionResolver(_ *os.File, _ io.Writer) workflow.PermissionResolver { return nil }

func NewInstaller(_ *os.File, _ io.Writer) setup.Confirmation { return nil }

func NewWorkspaceApprover(_ *os.File, _ io.Writer) workflow.WorkspaceApprover { return nil }

func NewValidationApprover(_ *os.File, _ io.Writer) workflow.ValidationApprover { return nil }

func NewDecisionKeyPrompt(_ *os.File, _ io.Writer) func(context.Context) (string, error) {
	return nil
}
