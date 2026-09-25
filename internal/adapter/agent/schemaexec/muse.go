package schemaexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/adapter/musecli"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

type MuseConfig struct {
	Executable, Model, Reasoning string
	Timeout                      time.Duration
	CanWrite                     bool
	ExtraArgs                    []string
}

func (c MuseConfig) Validate() error {
	if strings.TrimSpace(c.Executable) == "" || strings.TrimSpace(c.Model) == "" || strings.ContainsAny(c.Model, " \t\r\n\x00") || strings.HasPrefix(c.Model, "-") {
		return errors.New("Muse executable and model must be nonempty identifiers")
	}
	if c.Timeout <= 0 || c.Timeout > 24*time.Hour {
		return errors.New("Muse timeout must be positive and at most 24h")
	}
	switch c.Reasoning {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return errors.New("Muse reasoning must be none, minimal, low, medium, high, xhigh, max or ultra")
	}
	if len(c.ExtraArgs) > 0 {
		return errors.New("Muse extra_args are not supported; use explicit role settings")
	}
	return nil
}

type Muse struct {
	structured.Agent
	runner ProcessRunner
	config MuseConfig
}

func NewMuse(runner ProcessRunner, cfg MuseConfig) (*Muse, error) {
	if runner == nil {
		return nil, errors.New("Muse runner must not be nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	a := &Muse{runner: runner, config: cfg}
	a.Agent = structured.Agent{CanWrite: cfg.CanWrite, Execute: func(ctx context.Context, r structured.Invocation) (structured.Response, error) {
		text, err := a.execute(ctx, r.WorkingDir, r.Prompt, r.Schema)
		return structured.Response{Data: []byte(text)}, err
	}}
	return a, nil
}

// Execute supports direct, fresh invocations. Team repair also receives full
// handoff context; no foreign or stale Muse session is silently resumed.
func (a *Muse) Execute(ctx context.Context, input store.TaskInput) (store.DirectResponse, error) {
	if input.SessionID != "" {
		return store.DirectResponse{}, errors.New("Muse session resume is not supported; use /new")
	}
	text, err := a.execute(ctx, input.WorkingDir, input.Task, nil)
	return store.DirectResponse{Text: text}, err
}

func (a *Muse) execute(ctx context.Context, dir, prompt string, schema []byte) (string, error) {
	if ctx == nil {
		return "", errors.New("Muse context must not be nil")
	}
	name, err := musecli.Executable(a.config.Executable)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "multiharness-muse-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	promptPath := filepath.Join(tmp, "prompt.txt")
	prompt += "\nShell execution is unavailable in this role. Use file tools; configured Multiharness validation runs separately. Do not claim you ran commands. Return the requested final response without markdown fences."
	if err = os.WriteFile(promptPath, []byte(prompt), 0600); err != nil {
		return "", err
	}
	profile := ":read-only"
	if a.config.CanWrite {
		profile = ":ask-me"
	}
	args := []string{"exec", "--json", "--provider", "meta", "--workspace", dir, "--model", a.config.Model, "--reasoning-effort", a.config.Reasoning, "--permission-profile", profile, "--disable-shell", "--no-foreign-personal-context", "--no-session-log", "--prompt-file", promptPath}
	if !a.config.CanWrite {
		args = append(args, "--disable-write")
	}
	if len(schema) > 0 {
		path := filepath.Join(tmp, "schema.json")
		if err = os.WriteFile(path, schema, 0600); err != nil {
			return "", err
		}
		args = append(args, "--output-schema", path)
	}
	stream := &musecli.Stream{}
	result, err := provider.Run(ctx, a.runner, process.Command{Name: name, Args: args, Dir: dir, Timeout: a.config.Timeout, Stdout: stream, OutputLimit: 1 << 20})
	if err != nil {
		return "", fmt.Errorf("execute Muse: %w", err)
	}
	if result.ExitCode != 0 {
		return "", errors.New("Muse CLI exited unsuccessfully")
	}
	text, err := stream.Finish()
	if err != nil {
		return "", &structured.OutputError{Role: "Muse", Cause: err}
	}
	return text, nil
}
