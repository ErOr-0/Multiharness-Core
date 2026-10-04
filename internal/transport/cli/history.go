package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/history"
)

var planReference = regexp.MustCompile(`\bplan_[0-9a-f]{24}\b`)

func historyPath(settingsPath string) string {
	if state := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(state) {
		return filepath.Join(state, "magent", "history")
	}
	return filepath.Join(filepath.Dir(settingsPath), "state", "history")
}

func referencedPlanID(task string) string { return planReference.FindString(task) }

func resolvePlanID(archive *history.Archive, workspace, task, focused string) (string, []history.PlanMeta, error) {
	if id := referencedPlanID(task); id != "" {
		return id, nil, nil
	}
	lower := strings.ToLower(task)
	historical := false
	for _, phrase := range []string{"earlier", "previous", "old case", "we planned", "saved plan", "back to", "remember"} {
		if strings.Contains(lower, phrase) {
			historical = true
			break
		}
	}
	if !historical {
		return focusReference(lower, focused), nil, nil
	}
	stop := map[string]bool{"the": true, "a": true, "an": true, "our": true, "my": true, "that": true, "this": true, "plan": true, "planned": true, "earlier": true, "previous": true, "old": true, "case": true, "saved": true, "we": true, "to": true, "back": true, "remember": true, "please": true, "implement": true, "build": true, "do": true, "it": true, "about": true, "for": true, "from": true, "what": true, "was": true}
	var words []string
	for _, word := range strings.FieldsFunc(lower, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
		if len(word) >= 3 && !stop[word] {
			words = append(words, word)
		}
	}
	if len(words) == 0 {
		if id := focusReference(lower, focused); id != "" {
			return id, nil, nil
		}
		plans, err := archive.ListPlans(workspace, 6)
		if err != nil {
			return "", nil, err
		}
		if len(plans) == 1 {
			return plans[0].ID, plans, nil
		}
		return "", plans, nil
	}
	plans, err := archive.SearchPlans(workspace, strings.Join(words, " "), 6)
	if err != nil {
		return "", nil, err
	}
	if len(plans) == 1 {
		return plans[0].ID, plans, nil
	}
	return "", plans, nil
}

func focusReference(lower, focused string) string {
	if focused == "" {
		return ""
	}
	switch strings.TrimSpace(lower) {
	case "continue", "proceed", "go ahead", "implement", "implement the plan":
		return focused
	}
	for _, phrase := range []string{"this plan", "the plan", "that plan", "previous plan", "proceed with this", "implement this", "implement it", "do it", "continue this", "what did we plan"} {
		if strings.Contains(lower, phrase) {
			return focused
		}
	}
	return ""
}

func explicitPlanRequest(task string) bool {
	lower := strings.ToLower(strings.TrimSpace(task))
	for _, phrase := range []string{"/plan ", "make a plan", "create a plan", "give me a plan", "planning only", "plan only", "draft a plan", "refresh the plan", "revise the plan", "update the plan"} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return strings.HasPrefix(lower, "plan ") || strings.HasPrefix(lower, "what is the plan") || strings.HasPrefix(lower, "what's the plan")
}

func workspaceHead(workspace string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	digest := sha256.New()
	for _, args := range [][]string{{"rev-parse", "HEAD"}, {"diff", "--binary", "HEAD"}, {"ls-files", "--others", "--exclude-standard", "-z"}} {
		command := exec.CommandContext(ctx, "git", append([]string{"-C", workspace}, args...)...)
		command.Stdout = digest
		if err := command.Run(); err != nil {
			return ""
		}
		digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func recentTeamTurns(archive *history.Archive, conversationID string) ([]contract.ConversationTurn, error) {
	turns, err := archive.Recent(conversationID, contract.MaxRecentTurns)
	if err != nil {
		return nil, err
	}
	var result []contract.ConversationTurn
	for _, turn := range turns {
		result = contract.AppendTurn(result, turn.User, turn.Output)
		if len(result) > 0 {
			result[len(result)-1].ID = turn.ID
		}
	}
	return result, nil
}

func retrievedContextBytes(input contract.TaskInput) int {
	total := 0
	if len(input.RecentTurns) > 0 {
		data, _ := json.Marshal(input.RecentTurns)
		total += len(data)
	}
	if input.SelectedPlan != nil {
		data, _ := json.Marshal(input.SelectedPlan)
		total += len(data)
	}
	return total
}

func displayedReply(output contract.TaskOutput) string {
	if output.Direct != nil && output.Direct.Text != "" {
		return output.Direct.Text
	}
	if output.Plan != nil && output.Status == contract.TaskStatusAnswered {
		return output.Plan.Display()
	}
	reply := output.Summary
	if output.Implementation != nil {
		reply = output.Implementation.Summary + "\n" + reply
	}
	if output.Failure != nil {
		reply += "\n" + output.Failure.Message
	}
	return reply
}

func formatPlans(plans []history.PlanMeta) string {
	if len(plans) == 0 {
		return "No saved plans found in this workspace."
	}
	var text strings.Builder
	text.WriteString("Saved plans:\n")
	for _, plan := range plans {
		text.WriteString(plan.ID + " · " + plan.CaseID + " v" + fmt.Sprint(plan.Version) + " · " + plan.Title + " · " + strings.Join(plan.Tags, ", ") + "\n")
	}
	text.WriteString("Use /use PLAN_ID to select one.")
	return text.String()
}

func formatHistory(turns []history.Turn) string {
	if len(turns) == 0 {
		return "No saved exchanges found."
	}
	var text strings.Builder
	text.WriteString("Saved exchanges:\n")
	for _, turn := range turns {
		text.WriteString("• " + turn.ID + " · " + turn.Kind + " · " + contract.BoundTurnText(turn.User) + "\n  " + contract.BoundTurnText(turn.Assistant))
		for _, ref := range []string{turn.PlanID, turn.ImplementationID, turn.ReviewID} {
			if ref != "" {
				text.WriteString("\n  " + ref)
			}
		}
		if turn.Output.RetrievedContextBytes > 0 {
			text.WriteString(fmt.Sprintf("\n  Retrieved context: %d bytes", turn.Output.RetrievedContextBytes))
		}
		text.WriteString("\n")
	}
	return text.String()
}

// recallTurn adds one archived exchange when the task refers back to it. A
// failed search leaves the recent window unchanged.
func recallTurn(archive *history.Archive, conversationID, task string, recent []contract.ConversationTurn) []contract.ConversationTurn {
	query := contract.RecallQuery(task)
	if query == "" {
		return recent
	}
	turns, err := archive.SearchTurns(conversationID, query, 3)
	if err != nil {
		return recent
	}
	matches := make([]contract.ConversationTurn, 0, len(turns))
	for _, turn := range turns {
		matches = append(matches, contract.ConversationTurn{ID: turn.ID, User: turn.User, Assistant: turn.Assistant})
	}
	return contract.PrependRecalledTurn(recent, matches)
}
