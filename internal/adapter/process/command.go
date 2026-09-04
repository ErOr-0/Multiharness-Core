package process

import (
	"io"
	"strings"
	"time"
)

// DefaultOutputLimit is the maximum number of trailing bytes retained for
// each output stream when a command does not specify its own limit.
const DefaultOutputLimit = 1024 * 1024

// Command describes one direct executable invocation. It is deliberately not
// a shell command: Name and Args are passed separately to os/exec.
type Command struct {
	Name         string
	Args         []string
	Dir          string
	Timeout      time.Duration
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	EnvOverrides map[string]string
	// EnvUnset removes selected inherited variables before applying overrides.
	EnvUnset    []string
	OutputLimit int
}

func (command Command) validate() error {
	if strings.TrimSpace(command.Name) == "" {
		return errInvalidCommandName
	}
	if command.Timeout < 0 {
		return errInvalidTimeout
	}
	if command.OutputLimit < 0 {
		return errInvalidOutputLimit
	}
	return nil
}

func (command Command) outputLimit() int {
	if command.OutputLimit > 0 {
		return command.OutputLimit
	}
	return DefaultOutputLimit
}

// Result contains bounded output and execution metadata. ExitCode is -1 when
// the process did not produce a conventional exit status.
type Result struct {
	Stdout          string
	Stderr          string
	StdoutTruncated bool
	StderrTruncated bool
	ExitCode        int
	Duration        time.Duration
}
