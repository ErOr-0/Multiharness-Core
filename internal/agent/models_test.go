package agent_test

import (
	"slices"
	"testing"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/config"
	"multiharness-core/internal/testkit"
)

func TestParseModels(t *testing.T) {
	commandCode := "Available models  ·  3 models\n" +
		"\n" +
		"Open Source\n" +
		"\n" +
		"moonshotai/kimi-k2.5                    multimodal frontend coding\n" +
		"\n" +
		"Anthropic\n" +
		"\n" +
		"claude-sonnet-5-5                       best combo of speed & intelligence (recommended)\n" +
		// SafeValue accepts ":", so a versioned id like this is kept.
		"stealth/glyph-cluster:free              versioned model id\n" +
		"\n" +
		"Pass the full id, or just the short name after the last \"/\":\n" +
		"cmdc --model moonshotai/kimi-k2.5\n" +
		"Docs: https://example.com/models\n" +
		"\n" +
		"Decision models (headless only)\n" +
		"typesafe/jev  typed questions in, probabilities out\n"
	opencode := "anthropic/claude-sonnet-5-5\r\nopenai/gpt-5.5\r\nopencode/big-pickle\r\n"
	for _, tc := range []struct {
		name, output string
		want         []string
	}{
		{"command code", commandCode, []string{"moonshotai/kimi-k2.5", "claude-sonnet-5-5", "stealth/glyph-cluster:free"}},
		{"opencode", opencode, []string{"anthropic/claude-sonnet-5-5", "openai/gpt-5.5", "opencode/big-pickle"}},
		{"empty", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := agent.ParseModels(tc.output); !slices.Equal(got, tc.want) {
				t.Fatalf("ParseModels(%q) = %q, want %q", tc.output, got, tc.want)
			}
		})
	}
}

// The saved reasoning effort is the default for every job whose CLI takes a
// separate reasoning flag; it must never reach one that names the reasoning
// variant in the model instead.
func TestApplyConfigDefaultsReasoning(t *testing.T) {
	testkit.UseConfig(t, config.Config{Implementer: "codex", Model: "gpt-5.2-codex", Reasoning: "high"})
	req := agent.Request{Harness: "codex"}
	agent.ApplyConfig(&req)
	if req.Model != "gpt-5.2-codex" || req.Reasoning != "high" {
		t.Fatalf("a codex request keeps the saved model and reasoning: %+v", req)
	}
	testkit.UseConfig(t, config.Config{Implementer: "opencode", Model: "x/y", Reasoning: "high"})
	req = agent.Request{Harness: "opencode"}
	agent.ApplyConfig(&req)
	if req.Model != "x/y" || req.Reasoning != "" {
		t.Fatalf("a stale reasoning value must not fail an opencode job: %+v", req)
	}
}
