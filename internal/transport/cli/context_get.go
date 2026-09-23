package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"multiharness-core/internal/history"
)

// ContextGet exposes exact saved records by opaque ID to read-only agents.
// The default output is small; full content requires an explicit section.
func ContextGet(args []string, workspace, settingsPath string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "get" || len(args) > 4 {
		_, _ = fmt.Fprintln(stderr, "Usage: magent context get PLAN_ID|TURN_ID|IMPL_ID|REVIEW_ID [--section summary|steps|handoff|user|assistant|output|full]")
		return ExitUsage
	}
	section := "summary"
	if len(args) > 2 {
		if len(args) != 4 || args[2] != "--section" {
			return ExitUsage
		}
		section = args[3]
	}
	archive, err := history.OpenReadOnly(historyPath(settingsPath))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Cannot read local history:", err)
		return ExitFailed
	}
	defer archive.Close()
	id := args[1]
	var value any
	if strings.HasPrefix(id, "plan_") {
		plan, _, err := archive.LoadPlan(workspace, id)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "Plan unavailable:", err)
			return ExitFailed
		}
		switch section {
		case "summary":
			value = map[string]any{"id": plan.ID, "case_id": plan.CaseID, "version": plan.Version, "title": plan.Title, "tags": plan.Tags, "summary": plan.Summary}
		case "steps":
			value = plan.Steps
		case "handoff":
			value = plan.HandoffContext
		case "full":
			value = plan
		default:
			return ExitUsage
		}
	} else if strings.HasPrefix(id, "turn_") {
		turn, err := archive.LoadTurn(workspace, id)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "Exchange unavailable:", err)
			return ExitFailed
		}
		switch section {
		case "summary":
			value = map[string]any{"id": turn.ID, "kind": turn.Kind, "plan_id": turn.PlanID, "implementation_id": turn.ImplementationID, "review_id": turn.ReviewID, "user": boundedConversationText(turn.User), "assistant": boundedConversationText(turn.Assistant)}
		case "user":
			value = turn.User
		case "assistant":
			value = turn.Assistant
		case "output":
			value = turn.Output
		case "full":
			value = turn
		default:
			return ExitUsage
		}
	} else if strings.HasPrefix(id, "impl_") {
		result, err := archive.LoadImplementation(workspace, id)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "Implementation unavailable:", err)
			return ExitFailed
		}
		switch section {
		case "summary":
			value = map[string]any{"id": result.ID, "version": result.Version, "summary": result.Summary, "changed_files": result.ChangedFiles}
		case "full":
			value = result
		default:
			return ExitUsage
		}
	} else if strings.HasPrefix(id, "review_") {
		result, err := archive.LoadReview(workspace, id)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "Review unavailable:", err)
			return ExitFailed
		}
		switch section {
		case "summary":
			value = map[string]any{"approved": result.Approved, "summary": result.Summary}
		case "full":
			value = result
		default:
			return ExitUsage
		}
	} else {
		return ExitUsage
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ExitFailed
	}
	if _, err := stdout.Write(append(data, '\n')); err != nil {
		return ExitFailed
	}
	return ExitSuccess
}
