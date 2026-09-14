package cli

import (
	"multiharness-core/internal/config"
	"reflect"
)

// Session identity is local to a workspace and the selected native agent.
// Switching away and back starts fresh; sessions never cross provider boundaries.
func sameConversation(a, b config.Config) bool {
	// Increasing a deadline after an interrupted turn must not discard context.
	left, right := a.Implementer, b.Implementer
	left.Timeout, right.Timeout = 0, 0
	// Native permissions are applied on every invocation, including resume.
	left.PermissionPolicy, right.PermissionPolicy = "", ""
	left.Sandbox, right.Sandbox = "", ""
	return a.Mode == b.Mode && a.WorkingDir == b.WorkingDir && reflect.DeepEqual(left, right)
}
