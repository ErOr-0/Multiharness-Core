// Package structured owns versioned cross-agent wire schemas and prompts.
// Domain contracts remain provider-neutral in store.
package structured

import "fmt"

const (
	rolePlanning = "planning"
	roleReview   = "review"
)

type OutputError struct {
	Role  string
	Cause error
}

func (e *OutputError) Error() string { return fmt.Sprintf("invalid %s response: %v", e.Role, e.Cause) }
func (e *OutputError) Unwrap() error { return e.Cause }
