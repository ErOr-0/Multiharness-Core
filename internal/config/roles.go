package config

// RoleAgent names one agent a run depends on.
type RoleAgent struct {
	Role  string
	Agent Planner
}

// RoleAgents lists the agents the selected mode needs, in workflow order.
func (c Config) RoleAgents() []RoleAgent {
	if c.Mode == "direct" {
		return []RoleAgent{{"agent", Planner(c.Implementer)}}
	}
	return []RoleAgent{{"planner", c.Planner}, {"implementer", Planner(c.Implementer)}, {"reviewer", c.Reviewer}}
}
