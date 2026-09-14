package directexec

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func TestNativeChildProcess(t *testing.T) {
	if os.Getenv("MAGENT_DIRECT_FIXTURE") != "1" {
		return
	}
	prompt, _ := io.ReadAll(os.Stdin)
	if string(prompt) != "literal $HOME; --not-an-option" {
		os.Exit(9)
	}
	cwd, _ := os.Getwd()
	if cwd != os.Getenv("MAGENT_DIRECT_FIXTURE_DIR") {
		os.Exit(8)
	}
	fmt.Print(fixtures["opencode"])
	os.Exit(0)
}

func TestDirectAdapterAcrossRealProcessBoundary(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("MAGENT_DIRECT_FIXTURE", "1")
	t.Setenv("MAGENT_DIRECT_FIXTURE_DIR", dir)
	// Substitute only the executable invocation. OSRunner still owns stdin,
	// workspace, output streams, exit status, and child-process lifecycle.
	r := runnerFunc(func(ctx context.Context, c process.Command) (process.Result, error) {
		c.Args = []string{"-test.run=^TestNativeChildProcess$"}
		return process.NewOSRunner().Run(ctx, c)
	})
	a, _ := New(r, Config{Harness: "opencode", Executable: executable})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	out, err := a.Execute(ctx, store.TaskInput{Task: "literal $HOME; --not-an-option", WorkingDir: dir})
	if err != nil || out.Text != "Done, with tests." || out.SessionID != "ses_test" {
		t.Fatalf("%+v %v", out, err)
	}
}
