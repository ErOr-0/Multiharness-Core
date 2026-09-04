//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package process

import "os/exec"

// configureProcessTree keeps exec.CommandContext's direct-process cancellation
// on platforms where this adapter has no process-group implementation.
func configureProcessTree(_ *exec.Cmd) {}
