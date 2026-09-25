package config

import "testing"

func TestMuseRoleConfiguration(t *testing.T) {
	c := Defaults()
	c.Mode = "team"
	c.Planner = DefaultPlanner("muse")
	c.Implementer = DefaultImplementer("muse")
	c.Reviewer = DefaultPlanner("muse")
	c.Planner.Reasoning = "low"
	c.Implementer.Reasoning = "medium"
	c.Reviewer.Reasoning = "high"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Planner.Sandbox != "read-only" || c.Reviewer.Sandbox != "read-only" || c.Implementer.Sandbox != "workspace-write" {
		t.Fatal("role permissions lost")
	}
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"} {
		c.Planner.Reasoning = effort
		if err := c.Validate(); err != nil {
			t.Fatal(effort, err)
		}
	}
	c.Implementer.PermissionPolicy = "auto_approve"
	if c.Validate() == nil {
		t.Fatal("accepted unsupported approval policy")
	}
}
