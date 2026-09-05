// Package cli owns command-line input and presentation, not workflow policy.
package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

const (
	ExitSuccess     = 0
	ExitFailed      = 1
	ExitUsage       = 2
	ExitRepairLimit = 3
	ExitCancelled   = 130
)

type Runner interface {
	Run(context.Context, store.TaskInput) store.TaskOutput
}
type Factory func(config.Config, workflow.EventSink) (Runner, error)

type Handler struct {
	factory        Factory
	stdout, stderr io.Writer
	baseDir        string
	lookupEnv      func(string) (string, bool)
}

func NewHandler(factory Factory, stdout, stderr io.Writer, baseDir string, lookupEnv func(string) (string, bool)) (*Handler, error) {
	if factory == nil || stdout == nil || stderr == nil || !filepath.IsAbs(baseDir) {
		return nil, fmt.Errorf("CLI requires a factory, output writers, and an absolute invocation directory")
	}
	return &Handler{factory: factory, stdout: stdout, stderr: stderr, baseDir: baseDir, lookupEnv: lookupEnv}, nil
}

func (h *Handler) Run(ctx context.Context, args []string) int {
	// Presentation state belongs to this invocation, never to a reusable handler
	// or the workflow domain. IDs are random, not hashes of sensitive task text.
	presentation := newPresentation(h.stdout, h.stderr)
	defer presentation.progress.stop()
	return h.run(ctx, args, presentation)
}

func (h *Handler) run(ctx context.Context, args []string, presentation *presentation) int {
	if ctx == nil {
		return presentation.fail("workflow context is required", ExitFailed)
	}
	invocation, err := parseInvocation(args)
	presentation.progress.quiet = invocation.quiet
	if errors.Is(err, flag.ErrHelp) {
		return h.help(invocation.flags)
	}
	if err != nil {
		return presentation.fail(err.Error(), ExitUsage)
	}
	cfg, err := invocation.configuration(h.baseDir, h.lookupEnv)
	if err != nil {
		return presentation.fail(err.Error(), ExitUsage)
	}
	presentation.progress.format = cfg.LogFormat
	input, err := invocation.taskInput(cfg, h.baseDir)
	if err != nil {
		return presentation.fail(err.Error(), ExitUsage)
	}
	return h.runWorkflow(ctx, cfg, input, presentation)
}

func (h *Handler) runWorkflow(ctx context.Context, cfg config.Config, input store.TaskInput, presentation *presentation) int {
	if err := ctx.Err(); err != nil {
		return presentation.finish(store.TaskOutput{Status: store.TaskStatusCancelled, Summary: "workflow cancelled before startup"}, ExitCancelled)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout))
	defer cancel()
	progress := presentation.progress
	progress.cancel, progress.noChecks = cancel, len(cfg.Validation.Checks) == 0
	progress.configure(cfg, h.lookupEnv)
	progress.start(ctx)
	runner, err := h.factory(cfg, progress)
	if err != nil {
		return presentation.fail("initialize workflow: "+err.Error(), ExitUsage)
	}
	if runner == nil {
		return presentation.fail("workflow factory returned no runner", ExitFailed)
	}
	output := runner.Run(ctx, input)
	if err := output.Validate(); err != nil {
		return presentation.fail("workflow returned invalid output: "+err.Error(), ExitFailed)
	}
	return presentation.finish(output, exitCode(output.Status))
}

func (h *Handler) help(flags *flag.FlagSet) int {
	var help bytes.Buffer
	fmt.Fprintln(
		&help,
		"Usage: multiharness [flags] \"task\"\n       multiharness [flags] --task-file task.txt\n\nPrecedence: defaults < explicit JSON file < environment < CLI flags.\nAll relative application paths use the invocation directory; validation scripts use the target directory.\nExit codes: 0 approved/answered, 1 failed, 2 usage/config, 3 repair limit, 130 cancelled.\nOptions:",
	)
	flags.SetOutput(&help)
	flags.PrintDefaults()
	if _, err := io.Copy(h.stdout, &help); err != nil {
		return ExitFailed
	}
	return ExitSuccess
}

func exitCode(status store.TaskStatus) int {
	switch status {
	case store.TaskStatusApproved, store.TaskStatusAnswered:
		return ExitSuccess
	case store.TaskStatusRepairLimitReached:
		return ExitRepairLimit
	case store.TaskStatusCancelled:
		return ExitCancelled
	default:
		return ExitFailed
	}
}
