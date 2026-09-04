package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"multiharness-core/internal/adapter/process"
)

// ProcessRunner is owned by this adapter, not the workflow domain.
type ProcessRunner interface {
	Run(context.Context, process.Command) (process.Result, error)
}

func (workspace *Workspace) command(ctx context.Context, dir string, allowOne bool, args ...string) (string, error) {
	// Do not let inherited Git routing variables select another checkout/index,
	// or let status/diff invoke an external filesystem monitor or diff program.
	unset := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			unset = append(unset, key)
		}
	}
	flags := []string{"--no-pager", "-c", "core.fsmonitor=false", "-c", "core.ignoreStat=false", "-c", "core.untrackedCache=false", "-c", "color.ui=false"}
	result, err := workspace.runner.Run(ctx, process.Command{
		Name: workspace.config.Executable, Args: append(flags, args...), Dir: dir,
		Timeout: workspace.config.Timeout, OutputLimit: workspace.config.MaxOutputBytes,
		EnvUnset: unset, EnvOverrides: map[string]string{"GIT_OPTIONAL_LOCKS": "0", "LC_ALL": "C", "GIT_TERMINAL_PROMPT": "0"},
	})
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if result.StdoutTruncated || result.StderrTruncated {
		return "", fmt.Errorf("Git %s output exceeds configured limit", args[0])
	}
	if err != nil {
		var runErr *process.RunError
		if !(allowOne && result.ExitCode == 1 && errors.As(err, &runErr) && runErr.Kind == process.ErrorKindNonZeroExit) {
			return "", fmt.Errorf("Git %s: %w", args[0], err)
		}
	} else if result.ExitCode != 0 && !(allowOne && result.ExitCode == 1) {
		return "", fmt.Errorf("Git %s exited with code %d", args[0], result.ExitCode)
	}
	return result.Stdout, nil
}
