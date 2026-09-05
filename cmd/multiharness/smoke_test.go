package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/agent/sessionexec"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

// Real agents are NEVER invoked by an ordinary go test run. These tests consume
// the caller's existing CLI authentication/usage and only target fresh TempDirs.
// No automatic authentication, permission escalation, or model fallback occurs.
func smokeConfig(t *testing.T, needsOpenCode bool) config.Config {
	t.Helper()
	if os.Getenv("MULTIHARNESS_SMOKE") != "1" {
		t.Skip("opt-in: MULTIHARNESS_SMOKE=1; see docs/testing.md")
	}
	if os.Getenv("MULTIHARNESS_FIXTURE_PROCESS") != "" {
		t.Fatal("smoke tests cannot run in fixture-process mode")
	}
	if os.Getenv("CI") != "" {
		t.Fatal("live smoke tests are local opt-in only; CI must not receive provider credentials")
	}
	if err := smokeEnvironment(os.Environ()); err != nil {
		t.Fatal(err)
	}
	base, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(os.Getenv("MULTIHARNESS_SMOKE_CONFIG"), base, nil, smokeOverrides(os.Getenv))
	if err != nil {
		t.Fatal("invalid smoke configuration (values withheld); see docs/testing.md")
	}
	if needsOpenCode && cfg.Implementer.Model == "" {
		t.Fatal("set MULTIHARNESS_SMOKE_MODEL to an explicitly selected provider/model, or use implementer.model in MULTIHARNESS_SMOKE_CONFIG")
	}
	executables := []string{cfg.Planner.Executable, cfg.Git.Executable, "go"}
	if needsOpenCode {
		executables = append(executables, cfg.Reviewer.Executable, cfg.Implementer.Executable)
	}
	for _, name := range executables {
		if name == schemaexec.DefaultExecutable {
			continue // The runtime resolver also checks app-bundled installations.
		}
		if _, err := exec.LookPath(name); err != nil {
			t.Fatal("a configured smoke executable is unavailable")
		}
	}
	return cfg
}

// Bound model launches even when the caller's application config permits more.
// Permission settings and independently selected role models remain unchanged.
func smokeOverrides(getenv func(string) string) map[string]string {
	overrides := map[string]string{
		"timeout": "20m", "log-format": "json", "fallback-mode": "disabled",
		"max-agent-invocations": "8", "provider-max-retries": "0",
	}
	for variable, flag := range map[string]string{
		"MULTIHARNESS_SMOKE_MODEL":          "implementer-model",
		"MULTIHARNESS_SMOKE_FALLBACK_MODEL": "fallback-opencode-planner-model",
	} {
		if model := getenv(variable); model != "" {
			overrides[flag] = model
			if variable == "MULTIHARNESS_SMOKE_FALLBACK_MODEL" {
				overrides["fallback-opencode-reviewer-model"] = model
			}
		}
	}
	timeout := getenv("MULTIHARNESS_SMOKE_STAGE_TIMEOUT")
	if timeout == "" {
		timeout = "5m"
	}
	for _, role := range []string{
		"planner",
		"reviewer",
		"implementer",
		"fallback-codex-implementer",
		"fallback-opencode-planner",
		"fallback-opencode-reviewer",
	} {
		overrides[role+"-timeout"] = timeout
	}
	return overrides
}

func smokeEnvironment(environment []string) error {
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		// The Go command sets this for child processes. It prevents prompts and
		// cannot redirect repository access or enable command execution.
		if name == "GIT_TERMINAL_PROMPT" && value == "0" {
			continue
		}
		if strings.HasPrefix(name, "GIT_") {
			return fmt.Errorf("smoke tests require an environment without inherited GIT_* overrides; they can redirect agent Git commands outside the disposable checkout")
		}
	}
	return nil
}

const smokeSource = "package example\n\nfunc Add(a, b int) int { return 0 }\n"
const smokeFault = "package example\n\nfunc Add(a, b int) int { return a - b }\n"
const smokeTask = "Fix Add(a, b int) in sum.go to return the integer sum a+b, including negative and zero inputs. Change only sum.go. Do not alter tests, go.mod, user notes, Git metadata, or add files. Run go test -count=1 ./... to verify. This is a coding task."
const smokeTests = `package example
import "testing"
func TestAdd(t *testing.T) {
 for _, c := range [][3]int{{2,3,5},{-2,3,1},{-3,-4,-7},{0,0,0},{9,0,9},{0,8,8}} {
  if got := Add(c[0],c[1]); got != c[2] { t.Errorf("Add(%d,%d)=%d; want %d",c[0],c[1],got,c[2]) }
 }
}
`

func smokeRepository(t *testing.T, cfg config.Config) string {
	t.Helper()
	repo := t.TempDir()
	for name, contents := range map[string]string{
		"go.mod":      "module smoke.example\n\ngo 1.23\n",
		"sum.go":      smokeSource,
		"sum_test.go": smokeTests,
		"notes.txt":   "original notes\n",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q", "--template="}, {"add", "."}, {"commit", "-qm", "smoke baseline"}} {
		cmd := process.Command{
			Name:    cfg.Git.Executable,
			Dir:     repo,
			Timeout: 10 * time.Second,
			Args: append(
				[]string{
					"-c",
					"user.name=Smoke",
					"-c",
					"user.email=smoke@example.invalid",
					"-c",
					"commit.gpgsign=false",
					"-c",
					"core.hooksPath=/dev/null",
				},
				args...,
			),
		}
		cmd.EnvOverrides = map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"}
		if _, err := process.NewOSRunner().Run(t.Context(), cmd); err != nil {
			t.Fatal("disposable Git setup failed")
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("pre-existing user notes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return repo
}

type smokeRepairProbe struct {
	workflow.Implementer
	inject  bool
	session string
	repairs int
}

func (p *smokeRepairProbe) Implement(ctx context.Context, request store.ImplementationRequest) (store.ImplementationResult, error) {
	result, err := p.Implementer.Implement(ctx, request)
	if err != nil {
		return result, err
	}
	p.session = result.AgentSessionID
	if p.inject {
		// This file belongs exclusively to this test. Inject before the workflow
		// captures evidence, so neither validation nor review evidence is faked.
		err = os.WriteFile(filepath.Join(request.Input.WorkingDir, "sum.go"), []byte(smokeFault), 0600)
	}
	return result, err
}

func (p *smokeRepairProbe) ApplyReview(ctx context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	blocking := slices.ContainsFunc(request.Review.Findings, func(finding store.ReviewFinding) bool { return finding.Blocking })
	if request.Validation.Passed || request.Review.Approved || !blocking || request.Implementation.AgentSessionID != p.session || p.session == "" {
		return store.ImplementationResult{}, fmt.Errorf("smoke repair did not receive failed validation, blocking review, and original session")
	}
	p.repairs++
	result, err := p.Implementer.ApplyReview(ctx, request)
	if err == nil && result.AgentSessionID != p.session {
		return result, fmt.Errorf("smoke repair changed agent session")
	}
	return result, err
}

func TestSmokeWorkflow(t *testing.T) {
	base := smokeConfig(t, true)
	for _, scenario := range []string{"immediate_approval", "repair_loop"} {
		t.Run(
			scenario,
			func(t *testing.T) {
				cfg := base
				cfg.WorkingDir = smokeRepository(t, cfg)
				cfg.MaxRepairAttempts = 0
				if scenario == "repair_loop" {
					cfg.MaxRepairAttempts = 1
				}
				var probe *smokeRepairProbe
				factory := func(cfg config.Config, events workflow.EventSink) (cli.Runner, error) {
					deps, err := buildDependencies(cfg, events)
					if err != nil {
						return nil, err
					}
					probe = &smokeRepairProbe{Implementer: deps.Implementer, inject: scenario == "repair_loop"}
					deps.Implementer = probe
					return workflow.NewService(deps)
				}
				result := runSmokeCLI(t, cfg, factory)
				if probe == nil || probe.session == "" || probe.repairs != cfg.MaxRepairAttempts || result.RepairAttempts != cfg.MaxRepairAttempts {
					t.Fatal("did not exercise expected repair/session path")
				}
				t.Logf(
					"approved; repairs=%d; same OpenCode session; real Git evidence; deterministic Go checks; run=%s",
					result.RepairAttempts,
					result.RunID,
				)
			},
		)
	}
}

// Shared assertions keep normal and billing-handoff live tests equally strict.
func runSmokeCLI(t *testing.T, cfg config.Config, factory cli.Factory) cli.Result {
	t.Helper()
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		t.Fatal("Go executable required for smoke validation")
	}
	cfg.Validation.Checks = []config.Check{{Executable: goExecutable, Args: []string{"test", "-count=1", "./..."}}}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal("encode smoke configuration")
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal("write smoke configuration")
	}
	var stdout, stderr bytes.Buffer
	handler, err := cli.NewHandler(factory, &stdout, &stderr, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	exit := handler.Run(t.Context(), []string{"--config", configPath, "--task", smokeTask})
	var result cli.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal("invalid smoke result JSON")
	}
	if exit != 0 || result.Status != store.TaskStatusApproved {
		stage, code := store.WorkflowStage(""), store.FailureCode("")
		if result.Failure != nil {
			stage, code = result.Failure.Stage, result.Failure.Code
		}
		t.Fatalf(
			"smoke failed: exit=%d status=%s stage=%s code=%s run=%s (provider diagnostics withheld)",
			exit,
			result.Status,
			stage,
			code,
			result.RunID,
		)
	}
	if result.Validate() != nil {
		t.Fatal("smoke result failed contract validation (values withheld)")
	}
	if result.Repository == nil || result.Validation == nil || !slices.Equal(result.Repository.ChangedFiles, []string{"sum.go"}) || !slices.Equal(result.Repository.PreExistingFiles, []string{"notes.txt"}) || len(result.Validation.Checks) != 1 || !result.Validation.Passed {
		t.Fatal("missing independent validation or repository attribution")
	}
	for name, want := range map[string]string{"sum_test.go": smokeTests, "notes.txt": "pre-existing user notes\n", "go.mod": "module smoke.example\n\ngo 1.23\n"} {
		contents, err := os.ReadFile(filepath.Join(cfg.WorkingDir, name))
		if err != nil || string(contents) != want {
			t.Fatal("smoke changed a protected fixture")
		}
	}
	return result
}

// cancelOnOutput cancels only after the real process has emitted bytes, proving
// cancellation is not merely a pre-cancelled context that skips process launch.
type cancelOnOutput struct {
	once     sync.Once
	cancel   context.CancelFunc
	observed bool
}

func (w *cancelOnOutput) Write(data []byte) (int, error) {
	if len(data) > 0 {
		w.once.Do(func() { w.observed = true; w.cancel() })
	}
	return len(data), nil
}

type smokeProcessRunner struct {
	trigger *cancelOnOutput
	timeout time.Duration
}

func (r smokeProcessRunner) Run(ctx context.Context, command process.Command) (process.Result, error) {
	command.Timeout = r.timeout
	if r.trigger != nil {
		if command.Stdout == nil {
			command.Stdout = io.Discard
		}
		if command.Stderr == nil {
			command.Stderr = io.Discard
		}
		command.Stdout = io.MultiWriter(r.trigger, command.Stdout)
		command.Stderr = io.MultiWriter(r.trigger, command.Stderr)
	}
	return process.NewOSRunner().Run(ctx, command)
}

func TestSmokeAgentCancellation(t *testing.T) {
	for _, agent := range []string{"codex", "opencode"} {
		for _, mode := range []string{"timeout", "cancel_after_output"} {
			t.Run(
				agent+"/"+mode,
				func(t *testing.T) {
					cfg := smokeConfig(t, agent == "opencode")
					repo := smokeRepository(t, cfg)
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					runner := smokeProcessRunner{timeout: 200 * time.Millisecond}
					want := context.DeadlineExceeded
					if mode == "cancel_after_output" {
						runner.timeout = 30 * time.Second
						runner.trigger = &cancelOnOutput{cancel: cancel}
						want = context.Canceled
					}
					input := store.TaskInput{Task: "Read sum.go and explain it. Do not modify any files or run external services.", WorkingDir: repo}
					started := time.Now()
					var err error
					if agent == "codex" {
						selected, resolveErr := schemaexec.NewRuntimeRunner(process.NewOSRunner(), nil).Resolve(ctx, cfg.Planner.Executable, repo)
						if resolveErr != nil {
							t.Fatal("Codex runtime compatibility check failed")
						}
						settings := cfg.Planner.Adapter()
						settings.Executable = selected.Executable
						planner, createErr := schemaexec.NewPlanner(runner, settings)
						if createErr != nil {
							t.Fatal(createErr)
						}
						_, err = planner.Plan(ctx, input)
					} else {
						implementer, createErr := sessionexec.NewImplementer(runner, cfg.Implementer.Adapter())
						if createErr != nil {
							t.Fatal(createErr)
						}
						_, err = implementer.Implement(
							ctx,
							store.ImplementationRequest{
								Input: input,
								Plan: store.Plan{
									Action:             store.PlanActionImplement,
									Summary:            "Read-only cancellation probe",
									Steps:              []string{"Inspect sum.go without changing any file"},
									AcceptanceCriteria: []string{"Report observations"},
								},
							},
						)
					}
					if !errors.Is(err, want) {
						t.Fatalf("real %s did not preserve %s semantics (diagnostics withheld)", agent, mode)
					}
					if runner.trigger != nil && !runner.trigger.observed {
						t.Fatal("cancelled before real process output")
					}
					if time.Since(started) > 35*time.Second {
						t.Fatal("process did not stop within bounded cancellation deadline")
					}
					t.Logf("real %s %s verified", agent, mode)
				},
			)
		}
	}
}

func TestSmokeEnvironmentRejectsGitRedirects(t *testing.T) {
	for _, env := range [][]string{{"GIT_DIR=/important/repository"}, {"PATH=/bin", "GIT_WORK_TREE=/important"}, {"GIT_CONFIG_COUNT=1"}} {
		if err := smokeEnvironment(env); err == nil || strings.Contains(err.Error(), "/important") {
			t.Fatal("unsafe Git environment or leaked value")
		}
	}
	if err := smokeEnvironment([]string{"PATH=/bin", "API_KEY=secret", "GIT_TERMINAL_PROMPT=0"}); err != nil {
		t.Fatal(err)
	}
}
