package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const maxPromptBytes = 1 << 20

// Request is one hand-off to the inner harness. It is stored with the job so a
// detached worker can run it.
type Request struct {
	Harness   string        `json:"harness"`
	Dir       string        `json:"dir"`
	Prompt    string        `json:"prompt"`
	Session   string        `json:"session,omitempty"`
	Sandbox   string        `json:"sandbox"`
	Model     string        `json:"model,omitempty"`
	Reasoning string        `json:"reasoning,omitempty"`
	Allow     []string      `json:"allow,omitempty"`
	Bin       string        `json:"bin,omitempty"`
	Timeout   time.Duration `json:"timeout"`
	ExtraArgs []string      `json:"extra_args,omitempty"`
}

// Values that reach the harness command line, which on Windows may be a .cmd
// shim interpreted by cmd.exe, are limited to characters with no shell meaning.
var safeValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+#\[\]-]*$`)

type ruleList []string

func (l *ruleList) String() string     { return strings.Join(*l, ",") }
func (l *ruleList) Set(v string) error { *l = append(*l, v); return nil }

func parseRequest(name string, args []string, stdin io.Reader) (Request, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var req Request
	var allow ruleList
	var prompt, promptFile string
	fs.StringVar(&req.Harness, "to", "", "")
	fs.StringVar(&req.Dir, "dir", ".", "")
	fs.StringVar(&req.Session, "session", "", "")
	fs.StringVar(&req.Sandbox, "sandbox", "workspace-write", "")
	fs.StringVar(&req.Model, "model", "", "")
	fs.StringVar(&req.Reasoning, "reasoning", "", "")
	fs.Var(&allow, "allow", "")
	fs.DurationVar(&req.Timeout, "timeout", 60*time.Minute, "")
	fs.StringVar(&prompt, "prompt", "", "")
	fs.StringVar(&promptFile, "prompt-file", "", "")
	fs.StringVar(&req.Bin, "bin", "", "")
	if err := fs.Parse(args); err != nil {
		return Request{}, err
	}
	if fs.NArg() > 0 {
		if !slices.Contains(args, "--") {
			return Request{}, fmt.Errorf("unexpected argument %q (harness arguments go after --)", fs.Arg(0))
		}
		req.ExtraArgs = fs.Args()
	}
	req.Allow = allow
	applyConfig(&req)
	if err := req.validate(); err != nil {
		return Request{}, err
	}
	text, err := readPrompt(prompt, promptFile, stdin)
	if err != nil {
		return Request{}, err
	}
	req.Prompt = text
	return req, nil
}

// applyConfig fills what the user chose in setup: the implementer when --to is
// omitted, and its model and allow rules when those are omitted too.
func applyConfig(r *Request) {
	cfg, err := loadConfig()
	if err != nil {
		return
	}
	if r.Harness == "" {
		r.Harness = cfg.Implementer
	}
	if r.Harness != "" && r.Harness == cfg.Implementer {
		if r.Model == "" {
			r.Model = cfg.Model
		}
		if len(r.Allow) == 0 && r.Harness == claudeAdapter.Name {
			r.Allow = cfg.Allow
		}
	}
}

func (r *Request) validate() error {
	a := adapterNamed(r.Harness)
	switch {
	case r.Harness == "":
		return fmt.Errorf("--to is required (%s) until setup has chosen an implementer", adapterNames())
	case a == nil:
		return fmt.Errorf("--to %q: use one of %s", r.Harness, adapterNames())
	case r.Session != "" && !a.Resume:
		return fmt.Errorf("%s cannot continue a session; start a new job that repeats the context instead of --session", a.Title)
	case r.Reasoning != "" && a == opencodeAdapter:
		return errors.New("OpenCode takes the reasoning variant in --model (provider/model#variant), not --reasoning")
	}
	if r.Sandbox != "workspace-write" && r.Sandbox != "read-only" {
		return fmt.Errorf("--sandbox %q: use workspace-write or read-only", r.Sandbox)
	}
	for _, v := range []struct{ flag, value string }{{"session", r.Session}, {"model", r.Model}, {"reasoning", r.Reasoning}} {
		if v.value != "" && !safeValue.MatchString(v.value) {
			return fmt.Errorf("--%s %q contains unsupported characters", v.flag, v.value)
		}
	}
	if len(r.Allow) > 0 && a != claudeAdapter {
		return fmt.Errorf("--allow applies to Claude Code; pass %s settings after --", a.Title)
	}
	for _, rule := range r.Allow {
		if strings.TrimSpace(rule) == "" || strings.ContainsAny(rule, ",\"\r\n") {
			return fmt.Errorf("--allow %q: give one tool rule per flag, without commas, quotes or line breaks", rule)
		}
	}
	if r.Timeout <= 0 {
		return errors.New("--timeout must be positive")
	}
	dir, err := filepath.Abs(r.Dir)
	if err != nil {
		return fmt.Errorf("--dir: %w", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("--dir %s is not a directory", r.Dir)
	}
	r.Dir = dir
	return nil
}

func readPrompt(text, file string, stdin io.Reader) (string, error) {
	if text != "" && file != "" {
		return "", errors.New("use either --prompt or --prompt-file, not both")
	}
	var data []byte
	var err error
	switch {
	case text != "":
		data = []byte(text)
	case file != "":
		var f *os.File
		if f, err = os.Open(file); err == nil {
			data, err = io.ReadAll(io.LimitReader(f, maxPromptBytes+1))
			f.Close()
		}
	default:
		if f, ok := stdin.(*os.File); ok {
			if info, statErr := f.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice != 0 {
				return "", errors.New("no prompt: pipe it on stdin, or use --prompt or --prompt-file")
			}
		}
		data, err = io.ReadAll(io.LimitReader(stdin, maxPromptBytes+1))
	}
	if err != nil {
		return "", fmt.Errorf("read the prompt: %w", err)
	}
	if len(data) > maxPromptBytes {
		return "", errors.New("the prompt is larger than 1 MiB")
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", errors.New("the prompt is empty")
	}
	return string(data), nil
}
