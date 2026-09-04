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
	"strings"
	"time"
	"unicode/utf8"

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
	flags := flag.NewFlagSet("multiharness", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	var task, taskFile, filename string
	var taskSet, taskFileSet, configSet, quiet bool
	flags.Func("task", "task text (cannot be combined with a positional task or --task-file)", func(value string) error { task, taskSet = value, true; return nil })
	flags.Func("task-file", "read task text from a regular UTF-8 file", func(value string) error { taskFile, taskFileSet = value, true; return nil })
	flags.Func("config", "explicit version-1 JSON config file (MULTIHARNESS_CONFIG fallback)", func(value string) error { filename, configSet = value, true; return nil })
	flags.BoolVar(&quiet, "quiet", false, "suppress progress on stderr")
	overrides := map[string]string{}
	for _, option := range config.Options() {
		flags.Func(option.Name, option.Help+" ("+option.Environment()+")", func(value string) error { overrides[option.Name] = value; return nil })
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return h.help(flags)
		}
		presentation.progress.quiet = quiet
		return presentation.fail(err.Error(), ExitUsage)
	}
	presentation.progress.quiet = quiet
	if flags.NArg() > 1 {
		return presentation.fail("supply one quoted task; put all flags before the positional task", ExitUsage)
	}
	sources := flags.NArg()
	if taskSet {
		sources++
	}
	if taskFileSet {
		sources++
	}
	if sources != 1 {
		return presentation.fail("supply exactly one task using --task, --task-file, or one positional argument", ExitUsage)
	}
	if !configSet && h.lookupEnv != nil {
		filename, _ = h.lookupEnv("MULTIHARNESS_CONFIG")
	}
	if configSet && filename == "" {
		return presentation.fail("--config must name a file", ExitUsage)
	}
	cfg, err := config.Load(filename, h.baseDir, h.lookupEnv, overrides)
	if err != nil {
		return presentation.fail(err.Error(), ExitUsage)
	}
	presentation.progress.format = cfg.LogFormat
	if taskFileSet {
		if taskFile == "" || taskFile == "-" {
			return presentation.fail("--task-file requires a regular file; stdin is not supported", ExitUsage)
		}
		if !filepath.IsAbs(taskFile) {
			taskFile = filepath.Join(h.baseDir, taskFile)
		}
		data, err := config.ReadFile(taskFile, cfg.MaxTaskBytes)
		if err != nil {
			return presentation.fail("read task: "+err.Error(), ExitUsage)
		}
		task = string(data)
	} else if flags.NArg() == 1 {
		task = flags.Arg(0)
	}
	if len(task) > cfg.MaxTaskBytes || !utf8.ValidString(task) || strings.ContainsRune(task, 0) {
		return presentation.fail("task must be valid UTF-8 without NUL and within max-task-bytes", ExitUsage)
	}
	input := store.TaskInput{Task: task, WorkingDir: cfg.WorkingDir, MaxRepairAttempts: cfg.MaxRepairAttempts}
	if err := input.Validate(); err != nil {
		return presentation.fail(err.Error(), ExitUsage)
	}
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
	fmt.Fprintln(&help, "Usage: multiharness [flags] \"task\"\n       multiharness [flags] --task-file task.txt\n\nPrecedence: defaults < explicit JSON file < environment < CLI flags.\nAll relative application paths use the invocation directory; validation scripts use the target directory.\nExit codes: 0 approved/answered, 1 failed, 2 usage/config, 3 repair limit, 130 cancelled.\nOptions:")
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
