package musecli

import (
	"context"
	"fmt"
	"io"
	"time"

	"multiharness-core/internal/adapter/process"
)

type LoginRunner interface {
	Run(context.Context, process.Command) (process.Result, error)
}

// Login uses Muse's native device flow without terminal stdin. With a TTY,
// Muse waits for Enter before opening a browser, but provider processes are
// deliberately in their own process groups and cannot read the parent's
// foreground terminal (SIGTTIN). EOF selects native URL-and-poll mode instead.
// Credentials and browser approval remain owned by Muse and the user.
func Login(ctx context.Context, runner LoginRunner, executable, directory string, output, stderr io.Writer) error {
	name, err := Executable(executable)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "\nOpen the Meta sign-in link below in your computer's browser.\nApprove the matching code there; this terminal will continue automatically.\nNo Enter key is needed here. Press Ctrl+C to cancel."); err != nil {
		return err
	}
	_, err = runner.Run(ctx, process.Command{
		Name: name, Args: []string{"login"}, Dir: directory,
		// nil stdin is /dev/null, not the interactive terminal. Keep the normal
		// process-group cancellation and never synthesize a consent response.
		Stdin: nil, Stdout: output, Stderr: stderr, Timeout: 10 * time.Minute,
	})
	return err
}
