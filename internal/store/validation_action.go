package store

import "strings"

// ValidationAction is a proposed command, never authorization to execute it.
// It runs in the selected workspace with the CLI user's permissions after consent.
type ValidationAction struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	Reason     string   `json:"reason"`
}

func (a ValidationAction) Validate() error {
	if strings.TrimSpace(a.Executable) != a.Executable || a.Executable == "" || strings.HasPrefix(a.Executable, "-") || strings.ContainsAny(a.Executable, "\x00\r\n") {
		return invalid("executable", "must be an executable name or path")
	}
	if strings.TrimSpace(a.Reason) == "" || len(a.Reason) > 4096 || a.Args == nil || len(a.Args) > 128 {
		return invalid("validation_action", "requires a bounded reason and arguments")
	}
	size := len(a.Executable)
	for _, arg := range a.Args {
		size += len(arg)
		if strings.ContainsRune(arg, 0) {
			return invalid("args", "must not contain NUL")
		}
	}
	if size > 16384 {
		return invalid("validation_action", "command is too large")
	}
	return nil
}
