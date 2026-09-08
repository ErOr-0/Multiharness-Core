package config

import "strings"

// Option describes a setting shared by the environment and CLI. JSON values
// are used for numbers and collections; strings and durations are plain text.
type Option struct {
	Name string
	Path string
	JSON bool
	Help string
}

func (o Option) Environment() string {
	return "MULTIHARNESS_" + strings.ToUpper(strings.ReplaceAll(o.Name, "-", "_"))
}

func Options() []Option {
	options := []Option{
		{"planner-harness", "planner.harness", false, "planning and simple answers: codex (default) or opencode"},
		{
			"install-mode",
			"install_mode",
			false,
			"prompt to install missing default agent CLIs, or disabled; never installs unattended",
		},
		{
			"install-timeout",
			"install_timeout",
			false,
			"installer timeout (default 5m, maximum 30m); whole-run timeout also applies",
		},
		{
			"fallback-mode",
			"fallback.mode",
			false,
			"prompt for billing-only agent switching, or disabled; never switches unattended",
		},
		{"workdir", "working_dir", false, "target folder (one or more projects; Git optional)"},
		{"max-repair-attempts", "max_repair_attempts", true, "maximum repair calls (zero disables repairs)"},
		{"session-id", "session_id", false, "optional prior agent session ID to resume (empty starts a fresh session)"},
		{"timeout", "timeout", false, "whole workflow timeout, e.g. 4h"},
		{"max-task-bytes", "max_task_bytes", true, "maximum task text size in bytes"},
		{"log-format", "log_format", false, "stderr lifecycle logs: text or json (JSONL)"},
		{"color", "color", false, "text colours: auto, always or never; NO_COLOR and TERM=dumb disable colours"},
		{"progress", "progress", false, "auto terminal animation, plain readable lines, or off; JSON logs never animate"},
		{
			"max-agent-invocations",
			"execution.max_agent_invocations",
			true,
			"hard whole-agent launch limit per run (not a token or dollar cap)",
		},
		{
			"provider-max-retries",
			"execution.max_retries",
			true,
			"additional transient retries for read-only planning/review (default 0; maximum 10)",
		},
		{"provider-initial-delay", "execution.initial_delay", false, "initial exponential retry delay with jitter"},
		{
			"provider-max-delay",
			"execution.max_delay",
			false,
			"maximum retry wait; larger Retry-After stops instead of retrying early",
		},
		{
			"max-cost-microusd",
			"execution.max_cost_microusd",
			true,
			"0 only: positive monetary caps fail closed because CLI billing cannot be enforced",
		},
		{"implementer-harness", "implementer.harness", false, "implementation and repair: opencode (default) or codex"},
		{"implementer-executable", "implementer.executable", false, "selected agent executable name or path"},
		{"implementer-model", "implementer.model", false, "Codex model ID or OpenCode provider/model"},
		{"implementer-reasoning", "implementer.reasoning", false, "Codex implementation reasoning effort"},
		{"implementer-sandbox", "implementer.sandbox", false, "Codex implementation requires workspace-write"},
		{"implementer-variant", "implementer.variant", false, "OpenCode variant (empty uses its default)"},
		{"implementer-timeout", "implementer.timeout", false, "timeout for each implementation or repair"},
		{"implementer-permission-policy", "implementer.permission_policy", false, "reject_on_prompt or explicit auto_approve"},
		{"implementer-extra-args", "implementer.extra_args", true, "JSON array of non-managed selected-provider flags"},
		{"git-executable", "git.executable", false, "Git executable name or path"},
		{"git-timeout", "git.timeout", false, "repository-inspection timeout"},
		{"git-max-files", "git.max_files", true, "maximum snapshot file count (0 = unlimited)"},
		{"git-max-file-bytes", "git.max_file_bytes", true, "maximum snapshot bytes per file (0 = unlimited)"},
		{"git-max-snapshot-bytes", "git.max_snapshot_bytes", true, "maximum total snapshot bytes (0 = unlimited)"},
		{"git-max-output-bytes", "git.max_output_bytes", true, "maximum Git metadata or diff bytes (0 = unlimited)"},
		{"validation-checks", "validation.checks", true, "JSON array of executable/args/timeout/env_overrides checks"},
		{"validation-default-timeout", "validation.default_timeout", false, "default deterministic-check timeout"},
		{"validation-output-limit", "validation.output_limit", true, "retained output bytes per validation check"},
	}
	options = append(options,
		Option{"planner-variant", "planner.variant", false, "OpenCode planner variant"},
		Option{"planner-permission-policy", "planner.permission_policy", false, "planning requires reject_on_prompt"},
	)
	for _, role := range []string{"planner", "reviewer"} {
		for _, field := range []struct {
			name, help string
			json       bool
		}{
			{"executable", "agent executable name or path", false},
			{"model", "model identifier (OpenCode: provider/model)", false},
			{"reasoning", "Codex reasoning effort", false},
			{"timeout", "timeout for this agent invocation", false},
			{"sandbox", "must remain read-only", false},
			{"extra_args", "JSON array of non-managed provider flags", true},
		} {
			options = append(
				options,
				Option{role + "-" + strings.ReplaceAll(field.name, "_", "-"), role + "." + field.name, field.json, field.help},
			)
		}
	}
	for _, agent := range []struct {
		name, path string
		codex      bool
	}{
		{"codex-implementer", "codex_implementer", true},
		{"opencode-reviewer", "opencode_reviewer", false},
	} {
		fields := []string{"executable", "model", "timeout", "extra_args"}
		if agent.codex {
			fields = append(fields, "reasoning", "sandbox")
		} else {
			fields = append(fields, "variant", "permission_policy")
		}
		for _, field := range fields {
			options = append(
				options,
				Option{
					"fallback-" + agent.name + "-" + strings.ReplaceAll(field, "_", "-"),
					"fallback." + agent.path + "." + field,
					field == "extra_args",
					"alternate role " + field + " (used only after explicit confirmation)",
				},
			)
		}
	}
	for _, field := range []string{"harness", "executable", "model", "reasoning", "variant", "timeout", "sandbox", "permission_policy", "extra_args"} {
		options = append(options, Option{
			"fallback-planner-" + strings.ReplaceAll(field, "_", "-"),
			"fallback.planner." + field,
			field == "extra_args",
			"alternate planner " + field + " (used only after explicit confirmation)",
		})
	}
	return options
}
