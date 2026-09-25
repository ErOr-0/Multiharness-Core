package schemaexec

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/adapter/process"
)

func TestMuseRolesPinPermissionsAndReasoning(t *testing.T) {
	for _, write := range []bool{false, true} {
		for _, effort := range []string{"low", "medium", "high"} {
			t.Run(effort+map[bool]string{false: "-read", true: "-write"}[write], func(t *testing.T) {
				dir := t.TempDir()
				var promptPath string
				runner := claudeRunnerFunc(func(ctx context.Context, c process.Command) (process.Result, error) {
					arg := func(key string) string {
						i := slices.Index(c.Args, key)
						if i < 0 || i+1 >= len(c.Args) {
							return ""
						}
						return c.Args[i+1]
					}
					if c.Dir != dir || arg("--workspace") != dir || arg("--reasoning-effort") != effort || arg("--provider") != "meta" {
						t.Fatal("lost role context", c.Args)
					}
					profile := ":read-only"
					if write {
						profile = ":ask-me"
					}
					if arg("--permission-profile") != profile || !slices.Contains(c.Args, "--disable-shell") || slices.Contains(c.Args, "--disable-write") == write {
						t.Fatal("lost permission boundary", c.Args)
					}
					for _, bad := range []string{"--yolo", "--disable-sandbox", "--disable-approval", "--approval-mode", "--trust-workspace", "--session-id"} {
						if slices.Contains(c.Args, bad) {
							t.Fatal("unsafe or conflicting flag", bad)
						}
					}
					promptPath = arg("--prompt-file")
					prompt, err := os.ReadFile(promptPath)
					if err != nil || !strings.Contains(string(prompt), "private prompt") {
						t.Fatal("lost prompt", err)
					}
					if strings.Contains(strings.Join(c.Args, " "), "private prompt") {
						t.Fatal("prompt exposed in arguments")
					}
					if schema, err := os.ReadFile(arg("--output-schema")); err != nil || !json.Valid(schema) {
						t.Fatal("invalid output schema", err)
					}
					for _, kind := range []string{"run.lifecycle.started", "run.terminal.completed"} {
						terminal := ""
						if strings.HasSuffix(kind, "completed") {
							terminal = "completed"
						}
						_ = json.NewEncoder(c.Stdout).Encode(map[string]any{"schema_version": 1, "payload_schema_version": 1, "payload_type": kind, "stream": map[string]string{"kind": "session", "id": "session"}, "payload": map[string]string{"command_id": "run", "terminal": terminal, "text": `{"schema_version":"1","summary":"done","changed_files":[]}`}})
					}
					return process.Result{}, nil
				})
				a, err := NewMuse(runner, MuseConfig{Executable: "fixture-muse", Model: "muse-spark-1.3", Reasoning: effort, Timeout: time.Minute, CanWrite: write})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := a.execute(t.Context(), dir, "private prompt", structured.ImplementationSchema()); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
					t.Fatal("private prompt was not removed", err)
				}
			})
		}
	}
}

func TestMuseRejectsUnmanagedConfiguration(t *testing.T) {
	c := MuseConfig{Executable: "fixture", Model: "muse-spark-1.3", Reasoning: "high", Timeout: time.Minute}
	for _, reason := range []string{"", "automatic", "HIGH"} {
		bad := c
		bad.Reasoning = reason
		if bad.Validate() == nil {
			t.Fatal("accepted invalid effort", reason)
		}
	}
	c.ExtraArgs = []string{"--yolo"}
	if c.Validate() == nil {
		t.Fatal("accepted permission override")
	}
}
