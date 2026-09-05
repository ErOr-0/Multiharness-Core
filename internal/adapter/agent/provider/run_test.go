package provider_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

type runnerFunc func(context.Context, process.Command) (process.Result, error)

func (f runnerFunc) Run(ctx context.Context, c process.Command) (process.Result, error) {
	return f(ctx, c)
}

func TestMonitorPreservesExistingStreamsAndUnclassifiedProcessErrors(t *testing.T) {
	var out, stderr bytes.Buffer
	cause := errors.New("execution failure")
	_, err := provider.Run(t.Context(), runnerFunc(func(_ context.Context, c process.Command) (process.Result, error) {
		_, _ = io.WriteString(c.Stdout, "ordinary output\n")
		_, _ = io.WriteString(c.Stderr, "Error: unexpected failure\n")
		return process.Result{Stderr: "unexpected failure"}, cause
	}), process.Command{Stdout: &out, Stderr: &stderr})
	if !errors.Is(err, cause) || out.String() != "ordinary output\n" || stderr.String() != "Error: unexpected failure\n" {
		t.Fatal("stream or process contract changed")
	}
}

func TestMonitorDelegatesNilContextValidationToRunner(t *testing.T) {
	cause := errors.New("nil context")
	_, err := provider.Run(nil, runnerFunc(func(ctx context.Context, _ process.Command) (process.Result, error) {
		if ctx != nil {
			t.Fatal("nil context silently replaced")
		}
		return process.Result{}, cause
	}), process.Command{})
	if !errors.Is(err, cause) {
		t.Fatal("runner validation lost")
	}
}

func TestProviderFailureCancelsChildAndOverridesExitZero(t *testing.T) {
	for _, body := range []string{
		`{"type":"error","error":{"code":"insufficient_quota","message":"secret"}}`,
		`{"type":"turn.failed","error":{"message":"quota exhausted"}}`,
		`{"type":"error","error":null,"message":"quota exhausted"}`,
	} {
		ctx := t.Context()
		runner := runnerFunc(func(child context.Context, c process.Command) (process.Result, error) {
			for _, b := range []byte(body + "\n") {
				_, _ = c.Stdout.Write([]byte{b})
			}
			if child.Err() != context.Canceled {
				t.Fatal("terminal error did not cancel child immediately")
			}
			return process.Result{ExitCode: 0}, nil
		})
		_, err := provider.Run(ctx, runner, process.Command{})
		var failure *store.ProviderFailure
		if !errors.As(err, &failure) || failure.Kind != store.ProviderBillingExhausted || ctx.Err() != nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("wrong cancellation/failure: %v", err)
		}
	}
}

func TestAmbiguousProviderEventsCannotHideFailure(t *testing.T) {
	for name, body := range map[string]string{
		"overwritten type":  `{"type":"error","error":{"code":"insufficient_quota"},"type":"item.completed"}`,
		"escaped type":      `{"type":"error","error":{"code":"insufficient_quota"},"t\u0079pe":"item.completed"}`,
		"case alias":        `{"type":"error","error":{"code":"insufficient_quota"},"TYPE":"item.completed"}`,
		"noncanonical type": `{"TYPE":"error","error":{"code":"insufficient_quota"}}`,
		"overwritten error": `{"type":"error","error":{"code":"insufficient_quota"},"error":{"code":"rate_limit_exceeded"}}`,
		"nested code":       `{"type":"error","error":{"code":"insufficient_quota","code":"rate_limit_exceeded"}}`,
		"invalid UTF-8":     "{\"type\":\"error\",\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"\xff\"}}",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := provider.Run(t.Context(), runnerFunc(func(child context.Context, c process.Command) (process.Result, error) {
				_, _ = io.WriteString(c.Stdout, body+"\n")
				if child.Err() != context.Canceled {
					t.Fatal("ambiguous control event did not stop the command")
				}
				return process.Result{ExitCode: 0}, nil
			}), process.Command{})
			var failure *store.ProviderFailure
			if !errors.As(err, &failure) || failure.Kind != store.ProviderUnknown || failure.Transient() {
				t.Fatalf("ambiguous event must fail without retries: %v", err)
			}
		})
	}
}

func TestProviderMonitorHandlesStderrEOFAndErrorPriority(t *testing.T) {
	for _, tc := range []struct {
		out, stderr string
		want        store.ProviderFailureKind
	}{
		{stderr: "Error: quota exhausted", want: store.ProviderBillingExhausted},
		{
			out:  `{"type":"error","error":{"code":"rate_limit_exceeded"}}` + "\n" + `{"type":"error","error":{"code":"insufficient_quota"}}`,
			want: store.ProviderBillingExhausted,
		},
		{out: `{"type":"error","error":{"message":"novel provider error"}}`, want: store.ProviderUnknown},
	} {
		_, err := provider.Run(t.Context(), runnerFunc(func(_ context.Context, c process.Command) (process.Result, error) {
			_, _ = io.WriteString(c.Stdout, tc.out)
			_, _ = io.WriteString(c.Stderr, tc.stderr)
			return process.Result{}, nil
		}), process.Command{})
		var failure *store.ProviderFailure
		if !errors.As(err, &failure) || failure.Kind != tc.want {
			t.Fatalf("failure=%v", err)
		}
	}
}

func TestProviderMonitorIgnoresTaskToolAndOversizedText(t *testing.T) {
	_, err := provider.Run(t.Context(), runnerFunc(func(ctx context.Context, c process.Command) (process.Result, error) {
		_, _ = io.WriteString(c.Stdout, `{"type":"item.completed","item":{"text":"Error: quota exhausted"}}`+"\n")
		_, _ = io.WriteString(c.Stdout, `{"type":"item.completed","metadata":{"CamelCase":1,"x-header":2},"item":{"text":"{\"type\":\"error\",\"type\":\"done\"}"}}`+"\n")
		_, _ = io.WriteString(c.Stderr, "{ordinary CLI diagnostic, not a JSON event}\n")
		_, _ = io.WriteString(c.Stderr, `{"level":"info","Message":"Starting command","metadata":{"Type":"custom"}}`+"\n")
		_, _ = io.WriteString(c.Stderr, "discussing quota exhausted in a test\n")
		_, _ = io.WriteString(c.Stdout, strings.Repeat("x", 2<<20)+"\n")
		if ctx.Err() != nil {
			t.Fatal("normal output caused cancellation")
		}
		return process.Result{}, nil
	}), process.Command{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUserCancellationAndTimeoutTakePrecedenceOverFallback(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		_, err := provider.Run(ctx, runnerFunc(func(_ context.Context, c process.Command) (process.Result, error) {
			if cancelParent {
				_, _ = io.WriteString(c.Stdout, `{"type":"error","error":{"code":"insufficient_quota"}}`+"\n")
				cancel()
			}
			return process.Result{Stderr: "rate limit exceeded"}, context.DeadlineExceeded
		}), process.Command{})
		want := context.DeadlineExceeded
		if cancelParent {
			want = context.Canceled
		}
		if !errors.Is(err, want) {
			t.Fatalf("got %v want %v", err, want)
		}
	}
}

func TestNonzeroStderrProducesSafeBillingDiagnostic(t *testing.T) {
	_, err := provider.Run(t.Context(), runnerFunc(func(context.Context, process.Command) (process.Result, error) {
		return process.Result{ExitCode: 1, Stderr: "secret-token: insufficient_quota"}, errors.New("command failed")
	}), process.Command{})
	var failure *store.ProviderFailure
	if !errors.As(err, &failure) || failure.Kind != store.ProviderBillingExhausted || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("failure=%v", err)
	}
}

func TestProviderFixtureProcess(t *testing.T) {
	if os.Getenv("MULTIHARNESS_PROVIDER_FIXTURE") != "1" {
		return
	}
	fmt.Println(`{"type":"error","error":{"code":"credit_balance_exhausted"}}`)
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestTerminalBillingStopsRealProcessWithoutWaitingForTimeout(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = provider.Run(
		t.Context(),
		process.NewOSRunner(),
		process.Command{
			Name:         executable,
			Args:         []string{"-test.run=^TestProviderFixtureProcess$"},
			Timeout:      10 * time.Second,
			EnvOverrides: map[string]string{"MULTIHARNESS_PROVIDER_FIXTURE": "1"},
		},
	)
	var failure *store.ProviderFailure
	if !errors.As(err, &failure) || failure.Kind != store.ProviderBillingExhausted {
		t.Fatalf("failure=%v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("waited for timeout instead of cancelling on billing error")
	}
}
