package main

import (
	"bytes"
	"testing"

	"multiharness-core/internal/config"
)

func TestCommandHelpAndInvalidInputWithoutAgentInvocations(t *testing.T) {
	for _, test := range []struct {
		args []string
		exit int
	}{{[]string{"--help"}, 0}, {nil, 2}} {
		var stdout, stderr bytes.Buffer
		if exit := run(test.args, &stdout, &stderr); exit != test.exit {
			t.Fatalf("exit=%d; stdout=%s; stderr=%s", exit, stdout.String(), stderr.String())
		}
		if stdout.Len() == 0 {
			t.Fatal("no command output")
		}
	}
}

func TestCompositionBuildsRealAdaptersWithoutInvokingAgents(t *testing.T) {
	runner, err := buildWorkflow(config.Defaults(), nil)
	if err != nil || runner == nil {
		t.Fatalf("composition: %v", err)
	}
}
