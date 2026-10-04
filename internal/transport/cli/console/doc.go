// Package console reads from a real terminal: bounded lines for consent
// prompts, the raw-mode command editor with completion, hidden key entry, and
// the key and mouse input that opens the failure view during a run. Its
// constructors return nil or an error when stdin and the output are not both
// terminals, so piped input can never answer a prompt.
package console
