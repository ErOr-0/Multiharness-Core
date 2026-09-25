package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
	"multiharness-core/internal/adapter/agent/activity"
)

// These are recognizable command diagnostic prefixes, not a search for the word
// "error" in arbitrary source code. Matches remain attributed to combined output;
// they are never used to decide workflow success or the cause of a failure.
var commandDiagnostic = regexp.MustCompile(`^(?:(?:rg|grep|sed|awk|cat|ls|find|git|fatal|error|Error|bash|sh|/bin/(?:ba)?sh|PermissionError|FileNotFoundError):\s|[^\s]+\([0-9]+,[0-9]+\):\s+(?:fatal )?error\s)`)

func failureOverview(event activity.Event) string {
	var body strings.Builder
	body.WriteString("COMMAND\n")
	if event.Command != "" {
		first, _, multi := strings.Cut(event.Command, "\n")
		body.WriteString(runewidth.Truncate(first, 160, "…"))
		if multi || runewidth.StringWidth(first) > 160 {
			body.WriteString("\n[command preview; press c for the complete command]")
		}
	} else {
		body.WriteString("The agent did not provide a command for this event.")
	}
	if event.Error != "" {
		body.WriteString("\n\nREPORTED ERROR\n")
		body.WriteString(event.Error)
	}
	var diagnostics []string
	for _, line := range strings.Split(event.Output, "\n") {
		line = strings.TrimSpace(line)
		if commandDiagnostic.MatchString(line) {
			diagnostics = append(diagnostics, line)
		}
	}
	if len(diagnostics) > 0 {
		body.WriteString("\n\nOUTPUT DIAGNOSTICS\nMatched lines from combined output:\n")
		if len(diagnostics) > 16 {
			diagnostics = append(append(diagnostics[:8:8], "[more diagnostic lines in output]"), diagnostics[len(diagnostics)-8:]...)
		}
		body.WriteString(strings.Join(diagnostics, "\n"))
	} else if event.Error == "" {
		body.WriteString("\n\nERROR DETAILS\nNo separate error message was provided by the agent.\nThe exit status alone does not identify the cause.")
	}
	if event.Output != "" {
		fmt.Fprintf(&body, "\n\nOUTPUT\n%d retained lines. Expand Output [o] to inspect.", strings.Count(event.Output, "\n")+1)
	}
	return body.String()
}
