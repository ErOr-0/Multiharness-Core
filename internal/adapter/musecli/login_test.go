package musecli

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
)

type loginRunnerFunc func(context.Context, process.Command) (process.Result, error)

func (f loginRunnerFunc) Run(ctx context.Context, cmd process.Command) (process.Result, error) {
	return f(ctx, cmd)
}

func TestLoginUsesDeviceFlowWithoutTerminalInput(t *testing.T) {
	var output, stderr bytes.Buffer
	ctx := context.Background()
	wantErr := errors.New("login rejected")
	runner := loginRunnerFunc(func(gotCtx context.Context, cmd process.Command) (process.Result, error) {
		if gotCtx != ctx || cmd.Name != "/pinned/muse" || cmd.Dir != "/workspace" || !reflect.DeepEqual(cmd.Args, []string{"login"}) {
			t.Fatalf("incorrect login invocation: %+v", cmd)
		}
		if cmd.Stdin != nil {
			t.Fatal("device login must use EOF, not terminal stdin or synthetic consent")
		}
		if cmd.Timeout != 10*time.Minute || cmd.Stdout != &output || cmd.Stderr != &stderr {
			t.Fatalf("login must remain bounded and forward its browser link/errors: %+v", cmd)
		}
		return process.Result{}, wantErr
	})
	if err := Login(ctx, runner, "/pinned/muse", "/workspace", &output, &stderr); !errors.Is(err, wantErr) {
		t.Fatalf("login error was lost: %v", err)
	}
	for _, instruction := range []string{"computer's browser", "matching code", "No Enter key", "Ctrl+C"} {
		if !strings.Contains(output.String(), instruction) {
			t.Errorf("missing device flow instruction %q", instruction)
		}
	}
}

func TestLoginCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	err := Login(ctx, process.NewOSRunner(), "/unused/muse", "", &output, &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was lost: %v", err)
	}
}
