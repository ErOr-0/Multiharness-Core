package account

import (
	"context"
	"io"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
)

type museRunner func(context.Context, process.Command) (process.Result, error)

func (f museRunner) Run(ctx context.Context, c process.Command) (process.Result, error) {
	return f(ctx, c)
}

func TestMuseReadinessQueriesCatalogWithoutRunningTask(t *testing.T) {
	for _, listed := range []bool{true, false} {
		runner := museRunner(func(_ context.Context, c process.Command) (process.Result, error) {
			input, _ := io.ReadAll(c.Stdin)
			if c.Args[0] != "serve" || strings.Contains(string(input), "turn/submit") || strings.Contains(string(input), "session/start") || !strings.Contains(string(input), "model/list") {
				t.Fatal("readiness started work")
			}
			model := "other"
			if listed {
				model = "muse-spark-1.3"
			}
			return process.Result{Stdout: `{"id":1,"result":{"serverInfo":{"name":"muse"}}}` + "\n" + `{"id":2,"result":{"models":[{"modelId":"` + model + `"}]}}`}, nil
		})
		status := Check(t.Context(), runner, Request{Harness: "muse", Executable: "fixture", Model: "muse-spark-1.3"})
		if status.Ready != listed {
			t.Fatal(status)
		}
	}
}
