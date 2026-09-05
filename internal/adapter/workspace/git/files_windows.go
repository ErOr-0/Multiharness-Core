//go:build windows

package git

import "fmt"

const snapshotReadFlags = 0

func checkWorkspaceAccess(_ string) error {
	// Windows locking alone cannot make a workflow safe: process cancellation
	// still only stops the direct child and interactive consent is unsupported.
	// Do not mutate the checkout or create access probes until those gates pass.
	return fmt.Errorf(
		"%w: native Windows workflows are disabled until process-tree cancellation and terminal consent are verified; use WSL with Linux-installed Git, Codex and OpenCode",
		ErrUnsupported,
	)
}
