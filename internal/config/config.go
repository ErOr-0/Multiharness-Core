// Package config loads explicit, versioned application settings. Precedence is
// defaults, JSON file, environment, then explicitly supplied CLI overrides.
// No repository configuration is discovered or executed automatically.
package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/schemaexec"
	decisionadapter "multiharness-core/internal/adapter/decision/openrouter"
	folderworkspace "multiharness-core/internal/adapter/workspace/folder"
	"multiharness-core/internal/workflow"
)

// Duration uses human-readable Go durations ("30s", "5m", "2h") in JSON.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }
func (d *Duration) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("duration must be a string")
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid duration; use units such as 30s or 5m")
	}
	*d = Duration(parsed)
	return nil
}

type Codex struct {
	Executable string                 `json:"executable"`
	Model      string                 `json:"model"`
	Reasoning  string                 `json:"reasoning"`
	Timeout    Duration               `json:"timeout"`
	Sandbox    schemaexec.SandboxMode `json:"sandbox"`
	ExtraArgs  []string               `json:"extra_args"`
}

type Workspace struct {
	ExistingWork     string   `json:"existing_work"`
	RecoveryDir      string   `json:"recovery_dir"`
	Executable       string   `json:"executable,omitempty"` // Legacy setting, ignored; no process is used.
	Timeout          Duration `json:"timeout"`
	MaxFiles         int      `json:"max_files"`
	MaxFileBytes     int64    `json:"max_file_bytes"`
	MaxSnapshotBytes int64    `json:"max_snapshot_bytes"`
	MaxOutputBytes   int      `json:"max_output_bytes"`
}

type Check struct {
	Executable   string            `json:"executable"`
	Args         []string          `json:"args,omitempty"`
	Timeout      Duration          `json:"timeout"`
	EnvOverrides map[string]string `json:"env_overrides,omitempty"`
}

type Validation struct {
	Checks         []Check  `json:"checks"`
	DefaultTimeout Duration `json:"default_timeout"`
	OutputLimit    int      `json:"output_limit"`
}

// Decision configures the optional System One router. Provider is jev (hosted
// through the operator's OpenRouter key, the default) or laya (a self-hosted
// Jev-compatible server). Blank model and endpoint take the provider defaults.
type Decision struct {
	Enabled             bool     `json:"enabled"`
	Provider            string   `json:"provider"`
	Model               string   `json:"model"`
	Endpoint            string   `json:"endpoint"`
	Timeout             Duration `json:"timeout"`
	ConfidenceThreshold float64  `json:"confidence_threshold"`
}

type Config struct {
	Mode              string      `json:"mode"`
	Version           int         `json:"version"`
	WorkingDir        string      `json:"working_dir"`
	MaxRepairAttempts int         `json:"max_repair_attempts"`
	SessionID         string      `json:"session_id"`
	Timeout           Duration    `json:"timeout"`
	MaxTaskBytes      int         `json:"max_task_bytes"`
	LogFormat         string      `json:"log_format"`
	Color             string      `json:"color"`
	Progress          string      `json:"progress"`
	InstallMode       string      `json:"install_mode"`
	InstallTimeout    Duration    `json:"install_timeout"`
	Planner           Planner     `json:"planner"`
	Reviewer          Planner     `json:"reviewer"`
	Implementer       Implementer `json:"implementer"`
	Workspace         Workspace   `json:"workspace"`
	Validation        Validation  `json:"validation"`
	Execution         Execution   `json:"execution"`
	Decision          Decision    `json:"decision"`
}

type Execution struct {
	MaxAgentInvocations int      `json:"max_agent_invocations"`
	MaxRetries          int      `json:"max_retries"`
	InitialDelay        Duration `json:"initial_delay"`
	MaxDelay            Duration `json:"max_delay"`
	// Zero means no monetary cap. Positive requests fail closed because the
	// supported CLI interfaces cannot enforce authoritative per-request spend.
	MaxCostMicrousd  int64 `json:"max_cost_microusd"`
	MaxPromptBytes   int   `json:"max_prompt_bytes"`
	ReviewChunkBytes int   `json:"review_chunk_bytes"`
}

func (e Execution) Policy() workflow.ExecutionPolicy {
	return workflow.ExecutionPolicy{
		MaxAgentInvocations: e.MaxAgentInvocations,
		MaxRetries:          e.MaxRetries,
		InitialDelay:        time.Duration(e.InitialDelay),
		MaxDelay:            time.Duration(e.MaxDelay),
		MaxPromptBytes:      e.MaxPromptBytes,
		ReviewChunkBytes:    e.ReviewChunkBytes,
	}
}

// Effective fills provider defaults into blank fields. A blank provider keeps
// configurations written before Laya support on Jev.
func (d Decision) Effective() Decision {
	if strings.TrimSpace(d.Provider) == "" {
		d.Provider = decisionadapter.ProviderJev
	}
	model, endpoint, _ := decisionadapter.ProviderDefaults(d.Provider)
	if strings.TrimSpace(d.Model) == "" {
		d.Model = model
	}
	if strings.TrimSpace(d.Endpoint) == "" {
		d.Endpoint = endpoint
	}
	return d
}

// ProviderName is the display name of the effective provider.
func (d Decision) ProviderName() string {
	return decisionadapter.ProviderName(d.Effective().Provider)
}

// KeyVariable names the environment variable that supplies the provider key.
func (d Decision) KeyVariable() string {
	if d.Effective().Provider == decisionadapter.ProviderLaya {
		return "LAYA_API_KEY"
	}
	return "OPENROUTER_API_KEY"
}

// RequiresKey reports whether the provider refuses to run without a key. A
// self-hosted Laya server may run without authentication.
func (d Decision) RequiresKey() bool { return d.Adapter("").RequiresKey() }

func (d Decision) Adapter(apiKey string) decisionadapter.Config {
	d = d.Effective()
	return decisionadapter.Config{
		Enabled:             d.Enabled,
		Provider:            d.Provider,
		Model:               d.Model,
		Endpoint:            d.Endpoint,
		Timeout:             time.Duration(d.Timeout),
		ConfidenceThreshold: d.ConfidenceThreshold,
		APIKey:              apiKey,
	}
}

func Defaults() Config {
	g := folderworkspace.DefaultConfig()
	p := workflow.DefaultExecutionPolicy()
	d := decisionadapter.DefaultConfig()
	return Config{
		Mode:              "direct",
		Version:           1,
		WorkingDir:        ".",
		MaxRepairAttempts: 3,
		Timeout:           Duration(4 * time.Hour),
		MaxTaskBytes:      1 << 20,
		LogFormat:         "text",
		Color:             "auto",
		Progress:          "auto",
		InstallMode:       "prompt",
		InstallTimeout:    Duration(5 * time.Minute),
		Planner:           DefaultPlanner("codex"),
		Reviewer:          DefaultPlanner("codex"),
		Implementer:       DefaultImplementer("claude"),
		Workspace:         Workspace{ExistingWork: "snapshot", Timeout: Duration(g.Timeout), MaxFiles: g.MaxFiles, MaxFileBytes: g.MaxFileBytes, MaxSnapshotBytes: g.MaxSnapshotBytes, MaxOutputBytes: g.MaxOutputBytes},
		Validation:        Validation{Checks: []Check{}, DefaultTimeout: Duration(5 * time.Minute), OutputLimit: 64 << 10},
		Execution: Execution{
			MaxAgentInvocations: p.MaxAgentInvocations,
			MaxRetries:          p.MaxRetries,
			InitialDelay:        Duration(p.InitialDelay),
			MaxDelay:            Duration(p.MaxDelay),
			MaxPromptBytes:      262144,
			ReviewChunkBytes:    131072,
		},
		// Model and endpoint stay blank so that switching decision.provider
		// also switches to that provider's defaults.
		Decision: Decision{
			Enabled:             d.Enabled,
			Provider:            d.Provider,
			Timeout:             Duration(d.Timeout),
			ConfidenceThreshold: d.ConfidenceThreshold,
		},
	}
}

// DirectTimeout identifies the effective operator-configured bound. Adapters
// do not introduce a second, hidden task deadline.
func (c Config) DirectTimeout() (time.Duration, string) {
	if c.Implementer.Timeout < c.Timeout {
		return time.Duration(c.Implementer.Timeout), "implementer-timeout"
	}
	return time.Duration(c.Timeout), "timeout"
}
