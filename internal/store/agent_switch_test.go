package store_test

import (
	"encoding/json"
	"testing"

	"multiharness-core/internal/store"
)

func TestAgentSwitchContract(t *testing.T) {
	valid := store.AgentSwitch{Stage: store.WorkflowStagePlanning, From: "Codex", To: "OpenCode", Model: "provider/model"}
	encoded, _ := json.Marshal(valid)
	var decoded store.AgentSwitch
	if json.Unmarshal(encoded, &decoded) != nil || decoded != valid || decoded.Validate() != nil {
		t.Fatal("switch roundtrip failed")
	}
	for _, mutate := range []func(*store.AgentSwitch){
		func(s *store.AgentSwitch) { s.CanWrite = true },
		func(s *store.AgentSwitch) { s.Stage = store.WorkflowStageIntake },
		func(s *store.AgentSwitch) { s.To = s.From },
		func(s *store.AgentSwitch) { s.From = "unsafe\ntext" },
		func(s *store.AgentSwitch) { s.Model = "" },
	} {
		s := valid
		mutate(&s)
		if s.Validate() == nil {
			t.Fatal("invalid consent contract accepted")
		}
	}
}
