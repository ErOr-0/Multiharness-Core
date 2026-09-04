package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
)

// A subprocess fixture speaks both CLI protocols without model calls. The test
// still exercises the real composition root, process runner, agent parsers,
// configuration, Git adapter, validator, service, and CLI serialization.
func TestMain(m *testing.M) {
	if os.Getenv("MULTIHARNESS_ACCEPTANCE_FIXTURE") == "1" {
		if err := acceptanceAgentProcess(); err != nil {
			fmt.Fprintln(os.Stderr, "acceptance fixture failed")
			os.Exit(2)
		}
		os.Exit(0)
	}
	if os.Getenv("MULTIHARNESS_FIXTURE_PROCESS") == "1" {
		if err := fixtureProcess(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fixtureProcess() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("missing fixture operation")
	}
	operation := os.Args[1]
	argument := func(name string) string {
		for i, arg := range os.Args {
			if arg == name && i+1 < len(os.Args) {
				return os.Args[i+1]
			}
		}
		return ""
	}
	if operation == "check" {
		if err := fixtureLog("check"); err != nil {
			return err
		}
		data, err := os.ReadFile("result.txt")
		if err != nil {
			return err
		}
		if string(data) != "fixed\n" {
			fmt.Fprintln(os.Stdout, "result is not fixed")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "result verified")
		return nil
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	if operation == "run" {
		content, call := "broken\n", "implement"
		if session := argument("--session"); session != "" {
			if session != "fixture-session" || !bytes.Contains(prompt, []byte("result is not fixed")) {
				return fmt.Errorf("repair context or session missing")
			}
			content, call = "fixed\n", "repair"
		}
		if err := fixtureLog(call); err != nil {
			return err
		}
		if err := os.WriteFile("result.txt", []byte(content), 0644); err != nil {
			return err
		}
		result := `{"schema_version":"1","summary":"fixture implementation","changed_files":["invented.txt"]}`
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "text", "sessionID": "fixture-session", "part": map[string]string{"type": "text", "text": result}})
	}
	if operation != "exec" {
		return fmt.Errorf("unknown fixture operation")
	}
	if argument("--sandbox") != "read-only" {
		return fmt.Errorf("Codex role is not read-only")
	}
	schema, err := os.ReadFile(argument("--output-schema"))
	if err != nil {
		return err
	}
	var response any
	if bytes.Contains(schema, []byte(`"action"`)) {
		if err := fixtureLog("plan"); err != nil {
			return err
		}
		action, answer := "implement", ""
		steps, criteria := []string{"fix result"}, []string{"result check passes"}
		if bytes.Contains(prompt, []byte("fixture answer")) {
			action, answer = "answer", "Fixture explanation."
			steps, criteria = []string{}, []string{}
		}
		response = map[string]any{"schema_version": "2", "action": action, "answer": answer, "summary": "fixture plan", "steps": steps, "acceptance_criteria": criteria}
	} else {
		if err := fixtureLog("review"); err != nil {
			return err
		}
		data, err := os.ReadFile("result.txt")
		if err != nil {
			return err
		}
		approved := string(data) == "fixed\n"
		findings := []map[string]any{}
		if !approved {
			findings = append(findings, map[string]any{"severity": "error", "blocking": true, "file": "result.txt", "line": 1, "description": "result is broken", "evidence": "result is not fixed", "required_action": "write fixed"})
		}
		response = map[string]any{"schema_version": "1", "approved": approved, "summary": "fixture review", "findings": findings, "suggestions": []string{}}
	}
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return os.WriteFile(argument("--output-last-message"), data, 0600)
}

func fixtureLog(call string) error {
	file, err := os.OpenFile(os.Getenv("MULTIHARNESS_FIXTURE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintln(file, call)
	return errors.Join(writeErr, file.Close())
}

func TestCommandWithRealAdaptersAndFixtureProcesses(t *testing.T) {
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, task string
		limit      int
		status     store.TaskStatus
		exit       int
		calls      string
	}{
		{"repair", "fixture change", 1, store.TaskStatusApproved, 0, "plan\nimplement\ncheck\nreview\nrepair\ncheck\nreview\n"},
		{"limit", "fixture change", 0, store.TaskStatusRepairLimitReached, 3, "plan\nimplement\ncheck\nreview\n"},
		{"answer without other executables", "fixture answer", 3, store.TaskStatusAnswered, 0, "plan\n"},
		{"missing planner", "fixture change", 0, store.TaskStatusFailed, 1, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := t.TempDir()
			command := func(args ...string) {
				t.Helper()
				result, err := process.NewOSRunner().Run(t.Context(), process.Command{Name: "git", Dir: repo, Args: append([]string{"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false"}, args...)})
				if err != nil {
					t.Fatalf("Git: %v; %s", err, result.Stderr)
				}
			}
			command("init", "-q")
			for name, content := range map[string]string{"result.txt": "before\n", "notes.txt": "notes\n"} {
				if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			command("add", ".")
			command("commit", "-qm", "baseline")
			if err := os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("user notes\n"), 0644); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(t.TempDir(), "calls")
			t.Setenv("MULTIHARNESS_FIXTURE_PROCESS", "1")
			t.Setenv("MULTIHARNESS_FIXTURE_LOG", log)
			cfg := config.Defaults()
			cfg.WorkingDir = repo
			cfg.MaxRepairAttempts = test.limit
			cfg.Timeout = config.Duration(time.Minute)
			cfg.Planner.Executable = helper
			cfg.Reviewer.Executable = helper
			cfg.Implementer.Executable = helper
			cfg.Planner.Timeout = config.Duration(10 * time.Second)
			cfg.Reviewer.Timeout = cfg.Planner.Timeout
			cfg.Implementer.Timeout = cfg.Planner.Timeout
			cfg.Validation.Checks = []config.Check{{Executable: helper, Args: []string{"check"}}}
			if test.status == store.TaskStatusAnswered {
				cfg.Implementer.Executable = filepath.Join(repo, "missing-opencode")
				cfg.Reviewer.Executable = filepath.Join(repo, "missing-reviewer")
			}
			if test.name == "missing planner" {
				cfg.Planner.Executable = filepath.Join(repo, "missing-codex")
			}
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(file, data, 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			// Avoid inheriting unrelated MULTIHARNESS settings from the developer's
			// shell: this is the same production factory with explicit input sources.
			handler, err := cli.NewHandler(buildWorkflow, &stdout, &stderr, t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if code := handler.Run(t.Context(), []string{"--config", file, "--task", test.task}); code != test.exit {
				t.Fatalf("exit=%d; stdout=%s; stderr=%s", code, stdout.String(), stderr.String())
			}
			var output store.TaskOutput
			decoder := json.NewDecoder(&stdout)
			if err := decoder.Decode(&output); err != nil {
				t.Fatal(err)
			}
			if err := decoder.Decode(&struct{}{}); err != io.EOF {
				t.Fatal("stdout contained extra data")
			}
			if err := output.Validate(); err != nil {
				t.Fatal(err)
			}
			if output.Status != test.status {
				t.Fatalf("output=%#v", output)
			}
			calls, _ := os.ReadFile(log)
			if string(calls) != test.calls {
				t.Fatalf("calls=%q", calls)
			}
			notes, _ := os.ReadFile(filepath.Join(repo, "notes.txt"))
			if string(notes) != "user notes\n" {
				t.Fatal("user notes changed")
			}
			if output.Implementation != nil && (len(output.Implementation.ChangedFiles) != 1 || output.Implementation.ChangedFiles[0] != "result.txt") {
				t.Fatal("trusted agent-reported files")
			}
			if output.Status == store.TaskStatusApproved && (!strings.Contains(output.Repository.Diff, "-before") || !strings.Contains(output.Repository.Diff, "+fixed")) {
				t.Fatal("lost baseline-relative diff")
			}
		})
	}
}
