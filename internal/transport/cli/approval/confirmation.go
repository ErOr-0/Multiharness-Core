// Package approval renders the yes/no and multiple-choice prompts a person
// answers during a run: CLI installation, existing-work
// edits, validation commands, native permissions and permission recovery.
// Every prompt fails closed: only an explicit answer from a real input
// surface grants anything. The WithProgress wrappers pause live progress
// output while a prompt is on screen.
package approval

import (
	"context"
	"io"
)

// ConfirmationInput abstracts a context-aware human input surface. Production
// supplies a terminal, not piped stdin or model-generated text.
type ConfirmationInput interface {
	ReadConfirmation(context.Context) (string, error)
}

// writeText reports a short write as an error so a prompt is never half shown.
func writeText(w io.Writer, value string) error {
	n, err := io.WriteString(w, value)
	if err == nil && n != len(value) {
		err = io.ErrShortWrite
	}
	return err
}
