package cli

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

// invocation holds only decoded CLI input. It never starts an agent or emits
// output; the handler owns presentation, deadlines and the single service call.
type invocation struct {
	flags                                  *flag.FlagSet
	task, taskFile, filename               string
	taskSet, taskFileSet, configSet, quiet bool
	overrides                              map[string]string
}

func parseInvocation(args []string) (*invocation, error) {
	in := &invocation{flags: flag.NewFlagSet("multiharness", flag.ContinueOnError), overrides: map[string]string{}}
	flags := in.flags
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.Func(
		"task",
		"task text (cannot be combined with a positional task or --task-file)",
		func(value string) error { in.task, in.taskSet = value, true; return nil },
	)
	flags.Func(
		"task-file",
		"read task text from a regular UTF-8 file",
		func(value string) error { in.taskFile, in.taskFileSet = value, true; return nil },
	)
	flags.Func(
		"config",
		"explicit version-1 JSON config file (MULTIHARNESS_CONFIG fallback)",
		func(value string) error { in.filename, in.configSet = value, true; return nil },
	)
	flags.BoolVar(&in.quiet, "quiet", false, "suppress progress on stderr")
	for _, option := range config.Options() {
		flags.Func(
			option.Name,
			option.Help+" ("+option.Environment()+")",
			func(value string) error { in.overrides[option.Name] = value; return nil },
		)
	}
	if err := flags.Parse(args); err != nil {
		return in, err
	}
	if flags.NArg() > 1 {
		return in, fmt.Errorf("supply one quoted task; put all flags before the positional task")
	}
	sources := flags.NArg()
	if in.taskSet {
		sources++
	}
	if in.taskFileSet {
		sources++
	}
	if sources != 1 {
		return in, fmt.Errorf("supply exactly one task using --task, --task-file, or one positional argument")
	}
	return in, nil
}

func (in *invocation) configuration(baseDir string, lookupEnv func(string) (string, bool)) (config.Config, error) {
	filename := in.filename
	if !in.configSet && lookupEnv != nil {
		filename, _ = lookupEnv("MULTIHARNESS_CONFIG")
	}
	if in.configSet && filename == "" {
		return config.Config{}, fmt.Errorf("--config must name a file")
	}
	return config.Load(filename, baseDir, lookupEnv, in.overrides)
}

func (in *invocation) taskInput(cfg config.Config, baseDir string) (store.TaskInput, error) {
	task := in.task
	if in.taskFileSet {
		if in.taskFile == "" || in.taskFile == "-" {
			return store.TaskInput{}, fmt.Errorf("--task-file requires a regular file; stdin is not supported")
		}
		filename := in.taskFile
		if !filepath.IsAbs(filename) {
			filename = filepath.Join(baseDir, filename)
		}
		data, err := config.ReadFile(filename, cfg.MaxTaskBytes)
		if err != nil {
			return store.TaskInput{}, fmt.Errorf("read task: %w", err)
		}
		task = string(data)
	} else if in.flags.NArg() == 1 {
		task = in.flags.Arg(0)
	}
	if len(task) > cfg.MaxTaskBytes || !utf8.ValidString(task) || strings.ContainsRune(task, 0) {
		return store.TaskInput{}, fmt.Errorf("task must be valid UTF-8 without NUL and within max-task-bytes")
	}
	input := store.TaskInput{Task: task, WorkingDir: cfg.WorkingDir, MaxRepairAttempts: cfg.MaxRepairAttempts, SessionID: cfg.SessionID}
	return input, input.Validate()
}
