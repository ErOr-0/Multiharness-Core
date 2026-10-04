package contract

import (
	"fmt"
	"strings"
)

// WorkspaceChangedError identifies unstable evidence, not an I/O or permission
// failure. Only preparation before the first mutating agent may retry it.
type WorkspaceChangedError struct {
	During string
	Files  []string
}

func (e *WorkspaceChangedError) Error() string {
	message := "workspace changed " + e.During
	if len(e.Files) > 0 {
		paths := make([]string, 0, min(8, len(e.Files)))
		for _, name := range e.Files[:min(8, len(e.Files))] {
			paths = append(paths, fmt.Sprintf("%q", name))
		}
		message += "; changed paths: " + strings.Join(paths, ", ")
		if len(e.Files) > len(paths) {
			message += fmt.Sprintf(" (and %d more)", len(e.Files)-len(paths))
		}
	}
	return message
}
