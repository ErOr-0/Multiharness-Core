package contract_test

import (
	"encoding/json"
	"testing"

	"multiharness-core/internal/contract"
)

func TestAgentSwitchContract(t *testing.T) {
	valid := contract.AgentSwitch{Stage: contract.WorkflowStagePlanning, From: "Codex", To: "OpenCode", Model: "provider/model"}
	encoded, _ := json.Marshal(valid)
	var decoded contract.AgentSwitch
	if json.Unmarshal(encoded, &decoded) != nil || decoded != valid || decoded.Validate() != nil {
		t.Fatal("switch roundtrip failed")
	}
	for _, mutate := range []func(*contract.AgentSwitch){
		func(s *contract.AgentSwitch) { s.CanWrite = true },
		func(s *contract.AgentSwitch) { s.Stage = contract.WorkflowStageIntake },
		func(s *contract.AgentSwitch) { s.To = s.From },
		func(s *contract.AgentSwitch) { s.From = "unsafe\ntext" },
		func(s *contract.AgentSwitch) { s.Model = "" },
	} {
		s := valid
		mutate(&s)
		if s.Validate() == nil {
			t.Fatal("invalid consent contract accepted")
		}
	}
}
