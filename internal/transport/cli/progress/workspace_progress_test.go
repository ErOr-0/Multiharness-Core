package progress

import (
	"bytes"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
)

func TestWorkspaceProgressDistinguishesScanFromAgent(t *testing.T) {
	var output bytes.Buffer
	p := New(&output)
	p.Configure(config.Defaults(), nil)
	p.WorkspaceInspection("listing files (pass 1/2)", 25000)
	if p.stageLabel(contract.WorkflowStageImplementation) != "Inspecting workspace" || !strings.Contains(p.view.workspaceScan, "25000 files") {
		t.Fatal(p.view.workspaceScan)
	}
	p.WorkspaceInspection("secret/path", 1)
	if strings.Contains(output.String(), "secret") {
		t.Fatal("unbounded progress leaked")
	}
	p.WorkspaceInspection("", 0)
	if p.view.workspaceScan != "" || p.stageLabel(contract.WorkflowStageImplementation) == "Inspecting workspace" {
		t.Fatal("scan label persisted into agent work")
	}
}
