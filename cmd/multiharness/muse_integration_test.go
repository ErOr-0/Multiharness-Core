package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

func fixtureMuse(argument func(string) string) error {
	prompt, err := os.ReadFile(argument("--prompt-file"))
	if err != nil {
		return err
	}
	schema, err := os.ReadFile(argument("--output-schema"))
	if err != nil {
		return err
	}
	role, effort, profile := "plan", "low", ":read-only"
	if bytes.Contains(schema, []byte("changed_files")) {
		role, effort, profile = "implement", "medium", ":ask-me"
	} else if bytes.Contains(schema, []byte("approved")) {
		role, effort = "review", "high"
	}
	if argument("--model") != "fixture-muse-"+role || argument("--reasoning-effort") != effort || argument("--permission-profile") != profile || !slices.Contains(os.Args, "--disable-shell") || slices.Contains(os.Args, "--disable-write") != (role != "implement") {
		return errors.New("Muse role model/effort/permissions lost")
	}
	if argument("--session-id") != "" || slices.Contains(os.Args, "--yolo") {
		return errors.New("Muse changed permissions or reused a session")
	}
	var response any
	switch role {
	case "plan":
		response = fixturePlan(prompt)
	case "implement":
		content := "broken\n"
		if bytes.Contains(prompt, []byte("result is not fixed")) {
			content = "fixed\n"
			role = "repair"
		}
		if err := os.WriteFile("result.txt", []byte(content), 0644); err != nil {
			return err
		}
		response = map[string]any{"schema_version": "1", "summary": "Muse implementation", "changed_files": []string{"invented.txt"}}
	case "review":
		response, err = fixtureReview()
		if err != nil {
			return err
		}
	}
	if err := fixtureLog("muse-" + role); err != nil {
		return err
	}
	data, _ := json.Marshal(response)
	for _, kind := range []string{"run.lifecycle.started", "run.terminal.completed"} {
		terminal := ""
		if strings.HasSuffix(kind, "completed") {
			terminal = "completed"
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"schema_version": 1, "payload_schema_version": 1, "payload_type": kind, "stream": map[string]string{"kind": "session", "id": "fixture-session"}, "payload": map[string]string{"command_id": "fixture-run", "terminal": terminal, "text": string(data)}}); err != nil {
			return err
		}
	}
	return nil
}

func TestMuseTeamRepairIntegration(t *testing.T) {
	cfg, log := fixtureConfiguration(t)
	helper := cfg.Planner.Executable
	cfg.Fallback.Mode = "disabled"
	cfg.Planner = config.DefaultPlanner("muse")
	cfg.Planner.Executable = helper
	cfg.Planner.Model = "fixture-muse-plan"
	cfg.Planner.Reasoning = "low"
	cfg.Implementer = config.DefaultImplementer("muse")
	cfg.Implementer.Executable = helper
	cfg.Implementer.Model = "fixture-muse-implement"
	cfg.Implementer.Reasoning = "medium"
	cfg.Reviewer = config.DefaultPlanner("muse")
	cfg.Reviewer.Executable = helper
	cfg.Reviewer.Model = "fixture-muse-review"
	cfg.Reviewer.Reasoning = "high"
	svc, err := buildWorkflow(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := svc.Run(t.Context(), store.TaskInput{Task: "fixture change", WorkingDir: cfg.WorkingDir, MaxRepairAttempts: 1})
	if result.Status != store.TaskStatusApproved {
		data, _ := json.Marshal(result)
		t.Fatalf("workflow failed: %s", data)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "muse-plan\nmuse-implement\ncheck\nmuse-review\nmuse-repair\ncheck\nmuse-review\n" {
		t.Fatalf("unexpected role handoff %s", calls)
	}
}
