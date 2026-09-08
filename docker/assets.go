// Package dockerassets contains the audited delivery policy used by the host launcher.
package dockerassets

import _ "embed"

//go:embed seccomp.json
var Seccomp []byte
