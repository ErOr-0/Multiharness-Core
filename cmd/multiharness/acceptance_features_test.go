package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

const acceptanceSecret = "SYNTHETIC_SECRET_MUST_NOT_APPEAR"

type acceptanceState struct {
	Stage, Code, Transport                         string
	Failures, RetryAfter                           int
	Partial                                        bool
	AnswerOnly, AlwaysBroken, ReviewAlwaysApproves bool
	OutputStage, OutputIssue                       string
}

// This fixture is a local executable speaking the real CLI protocols, not a
// mock workflow. Every Gherkin scenario exercises the production composition,
// actual process runner, Git evidence, validation, config, parsers and delivery.
func acceptanceAgentProcess() error {
	directory := os.Getenv("MULTIHARNESS_ACCEPTANCE_STATE")
	data, err := os.ReadFile(filepath.Join(directory, "state.json"))
	if err != nil {
		return err
	}
	var state acceptanceState
	if err = json.Unmarshal(data, &state); err != nil {
		return err
	}
	if len(os.Args) < 2 {
		return fmt.Errorf("missing operation")
	}
	arg := func(name string) string {
		for i := 1; i < len(os.Args)-1; i++ {
			if os.Args[i] == name {
				return os.Args[i+1]
			}
		}
		return ""
	}
	stage := "validation"
	var prompt []byte
	if os.Args[1] != "check" {
		prompt, err = io.ReadAll(io.LimitReader(os.Stdin, 8<<20))
		if err != nil {
			return err
		}
	}
	if os.Args[1] == "run" {
		stage = "implementation"
		if arg("--session") != "" {
			stage = "repair"
		}
		if strings.HasPrefix(arg("--agent"), "multiharness-readonly-") {
			stage = "review"
			if bytes.Contains(prompt, []byte("planning stage")) {
				stage = "planning"
			}
		}
	}
	if os.Args[1] == "exec" {
		schema, err := os.ReadFile(arg("--output-schema"))
		if err != nil {
			return err
		}
		stage = "review"
		if bytes.Contains(schema, []byte(`"action"`)) {
			stage = "planning"
		}
		if bytes.Contains(schema, []byte(`"changed_files"`)) {
			stage = "implementation"
			if bytes.Contains(prompt, []byte("Repair request:")) {
				stage = "repair"
			}
		}
	}
	logPath := filepath.Join(directory, "calls.log")
	prior, _ := os.ReadFile(logPath)
	calls := strings.Count(string(prior), stage+"\n") + 1
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(log, stage)
	closeErr := log.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if stage == "validation" {
		content, err := os.ReadFile("result.txt")
		if err != nil {
			return err
		}
		if string(content) != "fixed\n" {
			fmt.Println("result needs repair")
			os.Exit(1)
		}
		fmt.Println("result verified")
		return nil
	}
	// Private telemetry must remain usable as activity without exposing its data.
	var progress any = map[string]any{"type": "step_start", "sessionID": "acceptance-session", "part": map[string]string{"type": "step-start", "private": acceptanceSecret}}
	if os.Args[1] == "exec" {
		progress = map[string]any{"type": "item.started", "item": map[string]string{"type": "command_execution", "status": "in_progress", "command": acceptanceSecret}}
	}
	if err := json.NewEncoder(os.Stdout).Encode(progress); err != nil {
		return err
	}
	if stage == "repair" {
		_, payload, found := strings.Cut(string(prompt), "Repair request:\n")
		var request struct {
			Input            store.TaskInput        `json:"input"`
			Plan             store.Plan             `json:"plan"`
			Validation       store.ValidationReport `json:"validation"`
			BlockingFindings []store.ReviewFinding  `json:"blocking_findings"`
		}
		if !found || json.NewDecoder(strings.NewReader(payload)).Decode(&request) != nil || (os.Args[1] == "run" && arg("--session") != "acceptance-session") || request.Input.Task != "fix result.txt" || len(request.Plan.Steps) == 0 || request.Validation.Passed || len(request.BlockingFindings) == 0 {
			return fmt.Errorf("repair did not receive original task/plan, failed validation, blocking feedback and session")
		}
	}
	emit := func(data []byte) error {
		if stage == state.OutputStage {
			switch state.OutputIssue {
			case "duplicate key":
				data = append(bytes.TrimSuffix(data, []byte("}")), []byte(`,"summary":"second summary"}`)...)
			case "wrong key case":
				data = bytes.Replace(data, []byte(`"summary"`), []byte(`"SUMMARY"`), 1)
			}
		}
		if os.Args[1] == "exec" {
			return os.WriteFile(arg("--output-last-message"), data, 0600)
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "text", "sessionID": "acceptance-session", "part": map[string]string{"type": "text", "text": string(data)}})
	}
	if stage == state.Stage && calls <= state.Failures {
		if state.Partial {
			if err := os.WriteFile("result.txt", []byte("partial\n"), 0644); err != nil {
				return err
			}
		}
		if state.Transport == "stderr" {
			fmt.Fprintln(os.Stderr, "Error:", state.Code, acceptanceSecret)
			os.Exit(1)
		}
		errorBody := map[string]any{"code": state.Code, "message": acceptanceSecret, "statusCode": 429}
		if state.RetryAfter > 0 {
			errorBody["retry_after"] = state.RetryAfter
		}
		event := map[string]any{"type": "error", "error": errorBody}
		if state.Transport != "no_session" {
			event["sessionID"] = "acceptance-session"
		}
		if err := json.NewEncoder(os.Stdout).Encode(event); err != nil {
			return err
		}
		if state.Transport == "hanging" {
			time.Sleep(time.Minute)
		}
		if state.Transport == "nonzero" {
			os.Exit(1)
		}
		if state.Transport != "error_then_success" {
			return nil
		}
	}
	if stage == "implementation" || stage == "repair" {
		contents := "fixed\n"
		if state.AlwaysBroken || (stage == "implementation" && state.Stage == "repair") {
			contents = "broken\n"
		}
		if err := os.WriteFile("result.txt", []byte(contents), 0644); err != nil {
			return err
		}
		return emit([]byte(`{"schema_version":"1","summary":"fixture implementation","changed_files":["spoofed-file"]}`))
	}
	var response any
	if stage == "planning" {
		response = map[string]any{"schema_version": "2", "action": "implement", "answer": "", "summary": "fix result", "steps": []string{"fix result.txt"}, "acceptance_criteria": []string{"result is fixed"}}
		if state.AnswerOnly {
			response = map[string]any{"schema_version": "2", "action": "answer", "answer": "The workflow coordinates planning, implementation and review.", "summary": "workflow explanation", "steps": []string{}, "acceptance_criteria": []string{}}
		}
	} else {
		content, err := os.ReadFile("result.txt")
		if err != nil {
			return err
		}
		approved := state.ReviewAlwaysApproves || string(content) == "fixed\n"
		findings := []map[string]any{}
		if !approved {
			findings = append(findings, map[string]any{"severity": "error", "blocking": true, "file": "result.txt", "line": 1, "description": "result needs repair", "evidence": "validation failed", "required_action": "write fixed"})
		}
		response = map[string]any{"schema_version": "1", "approved": approved, "summary": "fixture review", "findings": findings, "suggestions": []string{}}
	}
	data, err = json.Marshal(response)
	if err != nil {
		return err
	}
	return emit(data)
}

type providerScenario struct {
	confirmation    *string
	t               *testing.T
	repo, directory string
	cfg             config.Config
	state           acceptanceState
	result          cli.Result
	stdout, stderr  bytes.Buffer
	exit            int
	ran             bool
}

func (s *providerScenario) isolatedRepository() error {
	s.repo = s.t.TempDir()
	s.directory = s.t.TempDir()
	s.cfg = config.Defaults()
	s.cfg.WorkingDir = s.repo
	s.cfg.Timeout = config.Duration(20 * time.Second)
	s.cfg.LogFormat = "json"
	s.cfg.MaxRepairAttempts = 1
	s.cfg.Execution.InitialDelay = config.Duration(time.Millisecond)
	s.cfg.Execution.MaxDelay = config.Duration(5 * time.Millisecond)
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	s.cfg.Planner.Executable = executable
	s.cfg.Reviewer.Executable = executable
	s.cfg.Implementer.Executable = executable
	s.cfg.Fallback.CodexImplementer.Executable = executable
	s.cfg.Fallback.OpenCodePlanner.Executable = executable
	s.cfg.Fallback.OpenCodeReviewer.Executable = executable
	s.cfg.Planner.Timeout = config.Duration(5 * time.Second)
	s.cfg.Reviewer.Timeout = s.cfg.Planner.Timeout
	s.cfg.Implementer.Timeout = s.cfg.Planner.Timeout
	s.cfg.Validation.Checks = []config.Check{{Executable: executable, Args: []string{"check"}}}
	for name, content := range map[string]string{"result.txt": "before\n", "notes.txt": "original\n"} {
		if err := os.WriteFile(filepath.Join(s.repo, name), []byte(content), 0644); err != nil {
			return err
		}
	}
	unset := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			unset = append(unset, key)
		}
	}
	for _, args := range [][]string{{"init", "-q", "--template="}, {"add", "."}, {"commit", "-qm", "acceptance baseline"}} {
		_, err := process.NewOSRunner().Run(s.t.Context(), process.Command{Name: "git", Dir: s.repo, Args: append([]string{"-c", "user.name=Acceptance", "-c", "user.email=acceptance@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...), EnvUnset: unset, EnvOverrides: map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"}, Timeout: 10 * time.Second})
		if err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(s.repo, "notes.txt"), []byte("user notes\n"), 0644)
}

func (s *providerScenario) run() error {
	for name, value := range map[string]any{"state.json": s.state, "config.json": s.cfg} {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(s.directory, name), data, 0600); err != nil {
			return err
		}
	}
	s.t.Setenv("MULTIHARNESS_ACCEPTANCE_STATE", s.directory)
	factory := buildWorkflow
	if s.confirmation != nil {
		factory = func(cfg config.Config, events workflow.EventSink) (cli.Runner, error) {
			return buildWorkflowWithApproval(cfg, events, cli.BillingConfirmation{Input: scenarioConfirmation(*s.confirmation), Output: &s.stderr})
		}
	}
	handler, err := cli.NewHandler(factory, &s.stdout, &s.stderr, s.directory, nil)
	if err != nil {
		return err
	}
	s.exit = handler.Run(s.t.Context(), []string{"--config", filepath.Join(s.directory, "config.json"), "--task", "fix result.txt"})
	s.ran = true
	decoder := json.NewDecoder(&s.stdout)
	if err := decoder.Decode(&s.result); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("stdout contained extra data")
	}
	return s.result.Validate()
}

func (s *providerScenario) callCount(stage string) int {
	data, _ := os.ReadFile(filepath.Join(s.directory, "calls.log"))
	return strings.Count(string(data), stage+"\n")
}

type scenarioConfirmation string

func (answer scenarioConfirmation) ReadConfirmation(context.Context) (string, error) {
	if answer == "EOF" {
		return "yes", io.EOF
	}
	return string(answer), nil
}

func TestAcceptanceFeatures(t *testing.T) {
	t.Setenv("MULTIHARNESS_ACCEPTANCE_FIXTURE", "1")
	t.Setenv("GORACE", "atexit_sleep_ms=0") // Real fixture children remain race-instrumented, without exit sleeps.
	var s *providerScenario
	suite := godog.TestSuite{Name: "commercial-workflow", ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			s = &providerScenario{t: t}
			return ctx, nil
		})
		sc.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
			if s.ran {
				content, err := os.ReadFile(filepath.Join(s.repo, "notes.txt"))
				if err != nil || string(content) != "user notes\n" {
					return ctx, fmt.Errorf("pre-existing user notes changed")
				}
				encoded, _ := json.Marshal(s.result)
				if strings.Contains(string(encoded)+s.stderr.String(), acceptanceSecret) {
					return ctx, fmt.Errorf("provider secret leaked into public result/log")
				}
			}
			return ctx, nil
		})
		sc.Step(`^an isolated repository with pre-existing user notes$`, func() error { return s.isolatedRepository() })
		sc.Step(`^the operator answers "([^"]*)" to billing fallback$`, func(answer string) { s.confirmation = &answer })
		sc.Step(`^fallback is disabled$`, func() { s.cfg.Fallback.Mode = "disabled" })
		sc.Step(`^the switch records "([^"]*)" to "([^"]*)" during "([^"]*)"$`, func(from, to, stage string) error {
			if len(s.result.AgentSwitches) != 1 {
				return fmt.Errorf("missing single confirmed switch")
			}
			choice := s.result.AgentSwitches[0]
			if choice.From != from || choice.To != to || string(choice.Stage) != stage {
				return fmt.Errorf("wrong switch attribution")
			}
			return nil
		})
		sc.Step(`^(\d+) billing prompts were shown$`, func(n int) error {
			if strings.Count(s.stderr.String(), "Type yes to continue") != n {
				return fmt.Errorf("wrong prompt count")
			}
			return nil
		})
		sc.Step(`^no switch is recorded$`, func() error {
			if len(s.result.AgentSwitches) != 0 {
				return fmt.Errorf("switched without consent")
			}
			return nil
		})
		sc.Step(`^the planner answers without coding$`, func() { s.state.AnswerOnly = true })
		sc.Step(`^stderr uses "([^"]*)" logs with "([^"]*)" progress and "([^"]*)" color$`, func(format, progress, color string) {
			s.cfg.LogFormat, s.cfg.Progress, s.cfg.Color = format, progress, color
		})
		sc.Step(`^the readable progress includes "([^"]*)"$`, func(text string) error {
			if !strings.Contains(s.stderr.String(), text) {
				return fmt.Errorf("missing progress label %q", text)
			}
			return nil
		})
		sc.Step(`^the progress output contains no terminal escapes$`, func() error {
			if strings.ContainsAny(s.stderr.String(), "\x1b\r") {
				return fmt.Errorf("terminal controls leaked into plain output")
			}
			return nil
		})
		sc.Step(`^the progress output is empty$`, func() error {
			if s.stderr.Len() != 0 {
				return fmt.Errorf("progress was not suppressed")
			}
			return nil
		})
		sc.Step(`^safe activity from both agents appears in valid JSONL$`, func() error {
			seen := map[string]bool{}
			for _, line := range bytes.Split(bytes.TrimSpace(s.stderr.Bytes()), []byte{'\n'}) {
				var record struct{ Code, Agent, Activity string }
				if json.Unmarshal(line, &record) != nil {
					return fmt.Errorf("invalid JSONL progress")
				}
				if record.Code == "agent_activity" && record.Activity != "" {
					seen[record.Agent] = true
				}
			}
			if !seen["codex"] || !seen["opencode"] {
				return fmt.Errorf("missing live agent wiring")
			}
			return nil
		})
		sc.Step(`^the "([^"]*)" response contains a "([^"]*)"$`, func(stage, issue string) {
			s.state.OutputStage, s.state.OutputIssue = stage, issue
			if stage == "repair" {
				s.state.Stage = "repair" // Make initial validation require a repair.
			}
		})
		sc.Step(`^the initial implementation requires repair$`, func() { s.state.Stage = "repair" })
		sc.Step(`^every implementation fails validation$`, func() { s.state.AlwaysBroken = true })
		sc.Step(`^the reviewer incorrectly approves failing validation$`, func() { s.state.ReviewAlwaysApproves = true })
		sc.Step(`^(\d+) repair attempts are allowed$`, func(n int) { s.cfg.MaxRepairAttempts = n })
		sc.Step(`^the independently verified changes contain only "([^"]*)"$`, func(path string) error {
			if s.result.Repository == nil || s.result.Implementation == nil || !slices.Equal(s.result.Repository.ChangedFiles, []string{path}) || !slices.Equal(s.result.Implementation.ChangedFiles, []string{path}) {
				return fmt.Errorf("implementation claims replaced independent change attribution")
			}
			return nil
		})
		sc.Step(`^the result records (\d+) repair attempts$`, func(n int) error {
			if s.result.RepairAttempts != n {
				return fmt.Errorf("repair attempt accounting mismatch")
			}
			return nil
		})
		sc.Step(`^the operation sequence is "([^"]*)"$`, func(sequence string) error {
			data, err := os.ReadFile(filepath.Join(s.directory, "calls.log"))
			if err != nil || strings.Join(strings.Fields(string(data)), ",") != sequence {
				return fmt.Errorf("unexpected operation order")
			}
			return nil
		})
		sc.Step(`^(\d+) transient retries are allowed$`, func(n int) { s.cfg.Execution.MaxRetries = n })
		sc.Step(`^the "([^"]*)" provider reports "([^"]*)" for (\d+) invocations$`, func(stage, code string, n int) { s.state.Stage = stage; s.state.Code = code; s.state.Failures = n })
		sc.Step(`^the error transport is "([^"]*)"$`, func(mode string) { s.state.Transport = mode })
		sc.Step(`^the agent edits a file before failing$`, func() { s.state.Partial = true })
		sc.Step(`^the provider requests a (\d+) second wait$`, func(n int) { s.state.RetryAfter = n })
		sc.Step(`^the agent invocation limit is (\d+)$`, func(n int) { s.cfg.Execution.MaxAgentInvocations = n })
		sc.Step(`^a monetary cap of (\d+) microdollars is required$`, func(n int) { s.cfg.Execution.MaxCostMicrousd = int64(n) })
		sc.Step(`^the workflow runs$`, func() error { return s.run() })
		sc.Step(`^the result is "([^"]*)" with exit code (\d+)$`, func(status string, exit int) error {
			if s.result.Status != store.TaskStatus(status) || s.exit != exit {
				return fmt.Errorf("got status=%s exit=%d; want %s/%d", s.result.Status, s.exit, status, exit)
			}
			return nil
		})
		sc.Step(`^the provider failure is "([^"]*)" after (\d+) attempts$`, func(kind string, n int) error {
			if s.result.Failure == nil || s.result.Failure.Provider == nil || string(s.result.Failure.Provider.Kind) != kind || s.result.Failure.Provider.Attempts != n {
				return fmt.Errorf("missing expected normalized provider failure %s/%d", kind, n)
			}
			return nil
		})
		sc.Step(`^the "([^"]*)" agent was invoked (\d+) times$`, func(stage string, n int) error {
			if count := s.callCount(stage); count != n {
				return fmt.Errorf("%s invocations=%d; want %d", stage, count, n)
			}
			return nil
		})
		sc.Step(`^no agent ran after "([^"]*)"$`, func(stage string) error {
			data, _ := os.ReadFile(filepath.Join(s.directory, "calls.log"))
			if !strings.HasSuffix(string(data), stage+"\n") {
				return fmt.Errorf("workflow continued after failure")
			}
			return nil
		})
		sc.Step(`^the partial file remains in independent repository evidence$`, func() error {
			data, err := os.ReadFile(filepath.Join(s.repo, "result.txt"))
			if err != nil || string(data) != "partial\n" || s.result.Repository == nil || !slices.Contains(s.result.Repository.ChangedFiles, "result.txt") {
				return fmt.Errorf("partial work or repository evidence lost")
			}
			return nil
		})
		sc.Step(`^(\d+) retry events were logged$`, func(n int) error {
			if count := strings.Count(s.stderr.String(), `"type":"agent_retry_scheduled"`); count != n {
				return fmt.Errorf("retry logs=%d; want %d", count, n)
			}
			return nil
		})
		sc.Step(`^the result records (\d+) agent invocations$`, func(n int) error {
			if s.result.AgentInvocations != n {
				return fmt.Errorf("invocation accounting mismatch")
			}
			return nil
		})
		sc.Step(`^the failure code is "([^"]*)"$`, func(code string) error {
			if s.result.Failure == nil || string(s.result.Failure.Code) != code {
				return fmt.Errorf("failure code mismatch")
			}
			return nil
		})
	}, Options: &godog.Options{Format: "progress", Paths: []string{"features"}, TestingT: t, Strict: true, Concurrency: 1, Randomize: 42}}
	if suite.Run() != 0 {
		t.Fatal("Gherkin acceptance suite failed")
	}
}
