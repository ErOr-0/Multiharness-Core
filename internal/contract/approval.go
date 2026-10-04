package contract

import "context"

// NativeApproval contains only choices offered by the running harness. Choice
// IDs are opaque; the adapter owns their exact protocol response and scope.
type NativeApproval struct {
	Harness, Action, Detail string
	Choices                 []ApprovalChoice
}

type ApprovalChoice struct {
	ID, Label, Scope, Rule string
}

// NativeApprover must honor cancellation (including withdrawn native requests).
// An empty choice means deny. It never implies a wider permission grant.
type NativeApprover interface {
	ApproveNative(context.Context, NativeApproval) (string, error)
}
