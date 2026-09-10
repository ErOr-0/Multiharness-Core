package sessionexec

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// ReadOnlyAgent supports independent planning/review using fresh OpenCode sessions.
// A dedicated runtime agent denies all tools except source-reading tools. This
// is CLI permission policy, not an OS sandbox; managed settings/plugins remain
// an operator trust boundary. Git evidence detects review mutations; planning
// runs before baseline capture and relies on this provider permission policy.
type ReadOnlyAgent struct {
	structured.Agent
	runner ProcessRunner
	config Config
}

func NewReadOnlyAgent(runner ProcessRunner, config Config) (*ReadOnlyAgent, error) {
	if runner == nil {
		return nil, errNilRunner
	}
	config = config.withDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.PermissionPolicy != PermissionRejectOnPrompt {
		return nil, &ConfigurationError{Field: "permission_policy", Message: "read-only roles require reject_on_prompt"}
	}
	for _, arg := range config.ExtraArgs {
		if strings.HasPrefix(arg, "--pure=") {
			return nil, &ConfigurationError{Field: "extra_args", Message: "read-only roles cannot override --pure"}
		}
	}
	a := &ReadOnlyAgent{runner: runner, config: config}
	a.Agent = structured.Agent{Execute: func(ctx context.Context, r structured.Invocation) (structured.Response, error) {
		data, err := a.execute(ctx, r.Role, r.WorkingDir, r.Prompt, r.Schema)
		return structured.Response{Data: data}, err
	}, OutputError: func(role, session string, err error) error {
		return &OutputError{Operation: role, SessionID: session, Cause: err}
	}}
	return a, nil
}

func (a *ReadOnlyAgent) execute(ctx context.Context, role, dir, prompt string, schema []byte) ([]byte, error) {
	if ctx == nil {
		return nil, errNilContext
	}
	stream := newEventStream("")
	prompt += "\nUse only permitted read/glob/grep/list tools. Shell commands and edits are denied. For review, use the supplied workflow-owned Git evidence instead of invoking Git yourself; cross-check relevant source files. Do not claim to have executed unavailable tools. Return only JSON matching this schema, without Markdown fences or surrounding commentary:\n" + string(schema)
	command := buildCommand(a.config, dir, "", prompt, stream)
	// A fresh name avoids merging an existing project-defined agent's tool rules.
	name := "multiharness-readonly-" + rand.Text()
	inline, err := readOnlyConfig(os.Getenv("OPENCODE_CONFIG_CONTENT"), name)
	if err != nil {
		return nil, err
	}
	command.Args = append(command.Args, "--agent", name, "--pure")
	command.EnvOverrides = map[string]string{"OPENCODE_CONFIG_CONTENT": string(inline)}
	_, err = provider.Run(ctx, a.runner, command)
	if err != nil {
		return nil, &ExecutionError{Operation: role, Cause: err}
	}
	events, err := stream.finish()
	if err != nil {
		return nil, &OutputError{Operation: role, Cause: err}
	}
	if events.agentFailed {
		return nil, &store.ProviderFailure{Kind: store.ProviderUnknown, Attempts: 1}
	}
	return unwrapJSONFence([]byte(events.finalText)), nil
}

// Preserve caller-supplied provider/auth/model configuration without printing it
// or modifying the environment. Only the fresh agent is added to the child copy.
func readOnlyConfig(inherited, name string) ([]byte, error) {
	base := map[string]json.RawMessage{}
	if inherited != "" && (len(inherited) > 1<<20 || json.Unmarshal([]byte(inherited), &base) != nil || base == nil) {
		return nil, fmt.Errorf("inherited inline OpenCode configuration is invalid or too large")
	}
	agents := map[string]json.RawMessage{}
	if raw, exists := base["agent"]; exists {
		if json.Unmarshal(raw, &agents) != nil || agents == nil {
			return nil, fmt.Errorf("inherited inline OpenCode agent configuration must be an object")
		}
	}
	rule := struct {
		Description string            `json:"description"`
		Mode        string            `json:"mode"`
		Permission  map[string]string `json:"permission"`
	}{
		Description: "Read-only workflow planning and review",
		Mode:        "primary",
		Permission:  map[string]string{"*": "deny", "read": "allow", "glob": "allow", "grep": "allow", "list": "allow"},
	}
	agents[name], _ = json.Marshal(rule)
	base["agent"], _ = json.Marshal(agents)
	return json.Marshal(base)
}

var _ workflow.Planner = (*ReadOnlyAgent)(nil)
var _ workflow.Reviewer = (*ReadOnlyAgent)(nil)
