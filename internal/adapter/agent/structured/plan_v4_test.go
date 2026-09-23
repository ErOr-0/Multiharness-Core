package structured

import (
	"strings"
	"testing"

	"multiharness-core/internal/store"
)

func TestVersionFourProposalCarriesSearchMetadataWithoutImplementation(t *testing.T) {
	data := `{"schema_version":"4","action":"propose","title":"Invoice export","tags":["invoices","export"],"answer":"","summary":"Add a scoped export","handoff_context":["Keep tenant scoping"],"steps":["Add endpoint"],"acceptance_criteria":["Tenant test passes"]}`
	plan, err := ParsePlan([]byte(data))
	if err != nil || plan.Action != store.PlanActionPropose || plan.Title != "Invoice export" || len(plan.Tags) != 2 || plan.Display() == "" {
		t.Fatalf("proposal: %+v %v", plan, err)
	}
	for _, invalid := range []string{
		strings.Replace(data, `"tags":["invoices","export"]`, `"tags":[]`, 1),
		strings.Replace(data, `"title":"Invoice export"`, `"title":""`, 1),
	} {
		if _, err := ParsePlan([]byte(invalid)); err == nil {
			t.Fatalf("accepted unsearchable proposal: %s", invalid)
		}
	}
	if !strings.Contains(string(PlanSchema()), `"4"`) || !strings.Contains(string(PlanSchema()), `"propose"`) {
		t.Fatal("planner schema did not advance")
	}
}
