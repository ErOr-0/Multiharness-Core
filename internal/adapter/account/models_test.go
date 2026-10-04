package account

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
)

func TestModelsListOnlyWhatEachCLIReports(t *testing.T) {
	ids := func(models []Model) []string {
		var out []string
		for _, m := range models {
			out = append(out, m.ID)
		}
		return out
	}
	failed := fakeRunner(func(context.Context, process.Command) (process.Result, error) {
		return process.Result{ExitCode: 1}, nil
	})
	if _, err := Models(t.Context(), failed, Request{Harness: "muse", Executable: "muse"}); err == nil {
		t.Fatal("failed catalog accepted")
	}

	muse := museRunner(func(_ context.Context, c process.Command) (process.Result, error) {
		input, _ := io.ReadAll(c.Stdin)
		if strings.Contains(string(input), "session/start") || strings.Contains(string(input), "turn/submit") {
			t.Fatal("catalog query started work")
		}
		return process.Result{Stdout: `{"id":1,"result":{"serverInfo":{"name":"muse"}}}` + "\n" +
			`{"id":2,"result":{"models":[{"modelId":"muse-a","description":null},{"modelId":"muse-b","description":"Shared","isDefault":true}]}}`}, nil
	})
	models, err := Models(t.Context(), muse, Request{Harness: "muse", Executable: "muse"})
	if err != nil || !slices.Equal(ids(models), []string{"muse-a", "muse-b"}) || !models[1].Default || models[1].Description != "Shared" {
		t.Fatal(models, err)
	}

	never := fakeRunner(func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("Claude has no catalog command to run")
		return process.Result{}, nil
	})
	models, err = Models(t.Context(), never, Request{Harness: "claude", Executable: "claude"})
	if err != nil || !slices.Contains(ids(models), "sonnet") || !slices.Contains(ids(models), "claude-opus-5-5") {
		t.Fatal(models, err)
	}
}
