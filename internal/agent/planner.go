package agent

import (
	"multiharness-core/internal/config"
)

// PlannerHooks connect harness-bridge to an agent that plans, for all
// projects at once, by registering `harness-bridge mcp` as an MCP server.
type PlannerHooks struct {
	// Register connects exe (run as `exe mcp`) to the planner whose CLI is at
	// cli. cfg is what the user chose, for planners that need instructions too.
	Register func(cli, exe string, cfg config.Config) error
	// Unregister undoes Register; cli is "" when the planner is no longer
	// installed, and missing registrations are not an error.
	Unregister func(cli string) error
	// ConnectNote, if set, replaces the plan line that says what Register does.
	ConnectNote string
	// Allow, if set, lets the planner call harness-bridge without asking each
	// time; AllowNote describes it in the wizard's plan.
	Allow     func() error
	AllowNote string
}

func (h *PlannerHooks) ConnectLabel(a *Adapter) string {
	return FirstNonEmpty(h.ConnectNote, "Connect harness-bridge to "+a.Title+" for all your projects")
}
