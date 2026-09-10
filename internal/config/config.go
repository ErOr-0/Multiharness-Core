// Package config loads explicit, versioned application settings. Precedence is
// defaults, JSON file, environment, then explicitly supplied CLI overrides.
// No repository configuration is discovered or executed automatically.
package config

import (
	"encoding/json"
	"fmt"
	"time"

	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/agent/sessionexec"
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

type OpenCode struct {
	Executable       string                       `json:"executable"`
	Model            string                       `json:"model"`
	Variant          string                       `json:"variant"`
	Timeout          Duration                     `json:"timeout"`
	PermissionPolicy sessionexec.PermissionPolicy `json:"permission_policy"`
	ExtraArgs        []string                     `json:"extra_args"`
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

type Config struct {
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
	Fallback          Fallback    `json:"fallback"`
}

type Fallback struct {
	Mode             string   `json:"mode"`
	CodexImplementer Codex    `json:"codex_implementer"`
	Planner          Planner  `json:"planner"`
	OpenCodeReviewer OpenCode `json:"opencode_reviewer"`
}

type Execution struct {
	MaxAgentInvocations int      `json:"max_agent_invocations"`
	MaxRetries          int      `json:"max_retries"`
	InitialDelay        Duration `json:"initial_delay"`
	MaxDelay            Duration `json:"max_delay"`
	// Zero means no monetary cap. Positive requests fail closed because the
	// supported CLI interfaces cannot enforce authoritative per-request spend.
	MaxCostMicrousd int64 `json:"max_cost_microusd"`
}

func (e Execution) Policy() workflow.ExecutionPolicy {
	return workflow.ExecutionPolicy{
		MaxAgentInvocations: e.MaxAgentInvocations,
		MaxRetries:          e.MaxRetries,
		InitialDelay:        time.Duration(e.InitialDelay),
		MaxDelay:            time.Duration(e.MaxDelay),
	}
}

func Defaults() Config {
	c := schemaexec.DefaultConfig()
	o := sessionexec.DefaultConfig()
	g := folderworkspace.DefaultConfig()
	p := workflow.DefaultExecutionPolicy()
	return Config{
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
		Implementer:       DefaultImplementer("opencode"),
		Workspace:         Workspace{ExistingWork: "snapshot", Timeout: Duration(g.Timeout), MaxFiles: g.MaxFiles, MaxFileBytes: g.MaxFileBytes, MaxSnapshotBytes: g.MaxSnapshotBytes, MaxOutputBytes: g.MaxOutputBytes},
		Validation:        Validation{Checks: []Check{}, DefaultTimeout: Duration(5 * time.Minute), OutputLimit: 64 << 10},
		Execution: Execution{
			MaxAgentInvocations: p.MaxAgentInvocations,
			MaxRetries:          p.MaxRetries,
			InitialDelay:        Duration(p.InitialDelay),
			MaxDelay:            Duration(p.MaxDelay),
		},
		Fallback: Fallback{
			Mode:             "prompt",
			CodexImplementer: Codex{c.Executable, c.Model, c.Reasoning, Duration(o.Timeout), schemaexec.SandboxWorkspaceWrite, []string{}},
			Planner:          DefaultPlanner("opencode"),
			OpenCodeReviewer: OpenCode{o.Executable, o.Model, o.Variant, Duration(c.Timeout), sessionexec.PermissionRejectOnPrompt, []string{}},
		},
	}
}
