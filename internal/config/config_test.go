package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func configFile(t *testing.T, data string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(name, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func environment(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}

func TestLoadPrecedenceAndExplicitEmptyValues(t *testing.T) {
	file := configFile(t, `{"version":1,"max_repair_attempts":5,"planner":{"model":"file-model","reasoning":"high"},"implementer":{"model":"provider/file","extra_args":["--file-flag"]},"validation":{"checks":[{"executable":"go","args":["test","./..."]}]}}`)
	base := t.TempDir()
	cfg, err := Load(file, base, environment(map[string]string{
		"MULTIHARNESS_PLANNER_MODEL": "env-model", "MULTIHARNESS_MAX_REPAIR_ATTEMPTS": "2",
		"MULTIHARNESS_IMPLEMENTER_MODEL": "", "MULTIHARNESS_IMPLEMENTER_EXTRA_ARGS": "[]",
		"MULTIHARNESS_TIMEOUT": "invalid-but-overridden",
	}), map[string]string{"planner-model": "flag-model", "max-repair-attempts": "0", "timeout": "90m"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Planner.Model != "flag-model" || cfg.Planner.Reasoning != "high" || cfg.MaxRepairAttempts != 0 || time.Duration(cfg.Timeout) != 90*time.Minute {
		t.Fatalf("wrong precedence: %#v", cfg)
	}
	if cfg.Reviewer.Model != Defaults().Reviewer.Model || cfg.Implementer.Model != "" || len(cfg.Implementer.ExtraArgs) != 0 {
		t.Fatalf("defaults or explicit clearing lost: %#v", cfg)
	}
	if len(cfg.Validation.Checks) != 1 || cfg.Validation.DefaultTimeout != Defaults().Validation.DefaultTimeout {
		t.Fatal("nested defaults lost")
	}
	if cfg.WorkingDir != base {
		t.Fatalf("working dir = %q", cfg.WorkingDir)
	}
}

func TestLoadValidatesTheWinningConfiguration(t *testing.T) {
	cases := []map[string]string{
		{"color": "invalid"}, {"progress": "invalid"},
		{"fallback-mode": "auto"}, {"fallback-codex-implementer-sandbox": "danger-full-access"}, {"fallback-codex-implementer-model": "--bad"},
		{"fallback-opencode-planner-permission-policy": "auto_approve"}, {"fallback-opencode-reviewer-model": "no-provider"},
		{"fallback-codex-implementer-extra-args": `["-pprofile"]`}, {"fallback-codex-implementer-extra-args": `["-ooutput"]`},
		{"planner-model": ""}, {"planner-model": "--unsafe"}, {"planner-model": "has whitespace"},
		{"planner-executable": ""}, {"planner-executable": "bad\x00path"}, {"reviewer-executable": " -cmd"},
		{"planner-reasoning": "unknown"}, {"reviewer-sandbox": "workspace-write"},
		{"planner-timeout": "0s"}, {"implementer-timeout": "-1m"}, {"timeout": "0s"}, {"timeout": "nan"},
		{"max-repair-attempts": "-1"}, {"max-repair-attempts": "1.2"}, {"max-repair-attempts": "9999999999999999999999"},
		{"max-task-bytes": "0"}, {"git-max-files": "0"}, {"git-max-file-bytes": "9223372036854775807"},
		{"max-agent-invocations": "0"}, {"max-agent-invocations": "10001"},
		{"provider-max-retries": "-1"}, {"provider-max-retries": "11"},
		{"provider-initial-delay": "0s"}, {"provider-initial-delay": "31s"},
		{"provider-max-delay": "25h"}, {"provider-max-delay": "-1s"},
		{"max-cost-microusd": "1"}, {"max-cost-microusd": "-1"},
		{"git-timeout": "0s"}, {"git-executable": "-git"},
		{"implementer-model": "missing-provider"}, {"implementer-variant": "bad variant"}, {"implementer-permission-policy": "allow-everything"},
		{"implementer-extra-args": `["--auto"]`}, {"planner-extra-args": `["--sandbox=workspace-write"]`},
		{"planner-extra-args": `["--"]`}, {"planner-extra-args": `["-sdanger-full-access"]`}, {"planner-extra-args": `["-cpermissions=unsafe"]`},
		{"validation-output-limit": "0"}, {"validation-default-timeout": "-1s"},
		{"validation-checks": `[{"executable":"go","timeout":"-1s"}]`},
		{"validation-checks": `[{"executable":"go","env_overrides":{"BAD=KEY":"value"}}]`},
		{"validation-checks": `[{"executable":"","args":[]}]`},
		{"validation-checks": "null"}, {"planner-extra-args": "null"},
		{"validation-checks": `[{"executable":"go","argz":[]}]`}, {"unknown": "value"},
	}
	for _, override := range cases {
		t.Run(strings.Join(mapKeys(override), ","), func(t *testing.T) {
			if _, err := Load("", t.TempDir(), nil, override); err == nil {
				t.Fatalf("accepted invalid overrides: %#v", override)
			}
		})
	}
}

func TestPresentationConfigurationPrecedence(t *testing.T) {
	file := configFile(t, `{"version":1,"color":"never","progress":"plain"}`)
	cfg, err := Load(file, t.TempDir(), environment(map[string]string{"MULTIHARNESS_COLOR": "always", "MULTIHARNESS_PROGRESS": "off"}), map[string]string{"progress": "auto"})
	if err != nil || cfg.Color != "always" || cfg.Progress != "auto" {
		t.Fatalf("presentation precedence: color=%s progress=%s err=%v", cfg.Color, cfg.Progress, err)
	}
	old, err := Load(configFile(t, `{"version":1}`), t.TempDir(), nil, nil)
	if err != nil || old.Color != "auto" || old.Progress != "auto" {
		t.Fatal("old config lost safe defaults")
	}
}

func TestExecutionPolicyPrecedenceAndExplicitRetryDisable(t *testing.T) {
	file := configFile(t, `{"version":1,"execution":{"max_agent_invocations":12,"max_retries":2,"initial_delay":"2s","max_delay":"40s"}}`)
	cfg, err := Load(file, t.TempDir(), environment(map[string]string{
		"MULTIHARNESS_PROVIDER_MAX_RETRIES": "3", "MULTIHARNESS_MAX_AGENT_INVOCATIONS": "8",
	}), map[string]string{"provider-max-retries": "0", "provider-max-delay": "50s"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Execution.MaxRetries != 0 || cfg.Execution.MaxAgentInvocations != 8 || cfg.Execution.InitialDelay != Duration(2*time.Second) || cfg.Execution.MaxDelay != Duration(50*time.Second) {
		t.Fatalf("execution precedence lost: %+v", cfg.Execution)
	}
}

func mapKeys(values map[string]string) []string {
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func TestLoadRejectsMalformedOrAmbiguousFiles(t *testing.T) {
	for _, data := range []string{
		`{`, `[]`, `null`, `{}`, `{"version":2}`, `{"version":1,"typo":true}`,
		`{"version":1,"planner":{"modle":"typo"}}`, `{"version":1} {}`,
		`{"version":1,"planner":null}`, `{"version":1,"max_repair_attempts":null}`,
		`{"version":1,"planner":{"model":"first","model":"second"}}`,
		`{"version":1,"planner":{"model":"first","MODEL":"second"}}`,
		"{\"version\":1,\"planner\":{\"model\":\"\xff\"}}",
		`{"version":1,"planner":{"timeout":1000}}`,
	} {
		t.Run(data, func(t *testing.T) {
			if _, err := Load(configFile(t, data), t.TempDir(), nil, nil); err == nil {
				t.Fatal("accepted invalid file")
			}
		})
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing"), t.TempDir(), nil, nil); err == nil {
		t.Fatal("missing explicit config ignored")
	}
	if _, err := Load("", "relative-base", nil, nil); err == nil {
		t.Fatal("relative base accepted")
	}
}

func TestLoadResolvesPathsWithoutRebasingToConfigLocation(t *testing.T) {
	base := t.TempDir()
	file := configFile(t, `{"version":1,"working_dir":"project","planner":{"executable":"./tools/codex"},"validation":{"checks":[{"executable":"./scripts/check","args":["file.txt"]}]}}`)
	cfg, err := Load(file, base, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkingDir != filepath.Join(base, "project") || cfg.Planner.Executable != filepath.Join(base, "tools/codex") || cfg.Validation.Checks[0].Executable != filepath.Join(base, "project/scripts/check") {
		t.Fatalf("bad paths: %#v", cfg)
	}
	if cfg.Implementer.Executable != "opencode" {
		t.Fatal("bare executable should use PATH at invocation")
	}
	if cfg.Validation.Checks[0].Args[0] != "file.txt" {
		t.Fatal("argument rewritten")
	}
}

func TestDefaultConfigurationRoundTripsAndEveryOptionIsUsable(t *testing.T) {
	defaults := Defaults()
	data, err := json.Marshal(defaults)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Config
	if err := decodeStrict(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(defaults, decoded) {
		t.Fatal("defaults changed after JSON roundtrip")
	}
	if err := defaults.Validate(); err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := json.Unmarshal(data, &tree); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, option := range Options() {
		if seen[option.Name] {
			t.Fatalf("duplicate option: %s", option.Name)
		}
		seen[option.Name] = true
		var value any = tree
		for _, part := range strings.Split(option.Path, ".") {
			value = value.(map[string]any)[part]
		}
		var text string
		if option.JSON {
			encoded, _ := json.Marshal(value)
			text = string(encoded)
		} else {
			text = value.(string)
		}
		if _, err := Load("", t.TempDir(), environment(map[string]string{option.Environment(): text}), nil); err != nil {
			t.Fatalf("option %s is unusable: %v", option.Name, err)
		}
	}
}

func TestReadFileBoundsInput(t *testing.T) {
	file := configFile(t, "12345")
	if _, err := ReadFile(file, 4); err == nil {
		t.Fatal("oversized input accepted")
	}
	if _, err := ReadFile(file, 0); err == nil {
		t.Fatal("invalid limit accepted")
	}
	if _, err := ReadFile(t.TempDir(), 100); err == nil {
		t.Fatal("directory accepted")
	}
	if data, err := ReadFile(file, 5); err != nil || string(data) != "12345" {
		t.Fatalf("exact limit: %q %v", data, err)
	}
}

func TestExampleConfigurationLoadsAndRoundTrips(t *testing.T) {
	base, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(filepath.Join("..", "..", "examples", "multiharness.json"), base, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Validation.Checks) != 2 {
		t.Fatal("example must include deterministic checks")
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Config
	if err := decodeStrict(data, &decoded); err != nil {
		t.Fatalf("loaded config cannot roundtrip: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
}
