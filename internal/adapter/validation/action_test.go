package validation

import (
	"context"
	"reflect"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func TestValidationActionExecutesExactArgvInWorkspaceWithBounds(t *testing.T) {
	a := store.ValidationAction{Executable: "go", Args: []string{"-C", "module", "test", "./..."}, Reason: "cache write required"}
	var captured process.Command
	v, err := NewValidator(runnerFunc(func(_ context.Context, c process.Command) (process.Result, error) {
		captured = c
		return process.Result{Stdout: "ok"}, nil
	}), Config{DefaultTimeout: time.Minute, OutputLimit: 100})
	if err != nil {
		t.Fatal(err)
	}
	r, err := v.ValidateAction(t.Context(), request(), a)
	if err != nil || !r.Passed || len(r.Checks) != 1 {
		t.Fatal(r, err)
	}
	if captured.Name != "go" || !reflect.DeepEqual(captured.Args, a.Args) || captured.Dir != "/workspace" || captured.Timeout != time.Minute || captured.OutputLimit != 100 {
		t.Fatal(captured)
	}
	if len(v.config.Checks) != 0 {
		t.Fatal("persisted one-time authorization")
	}
}
