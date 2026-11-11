package agent

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ParseModels reads the model ids out of a CLI's model listing: one id per
// line, optionally followed by two or more spaces and a description. Headings,
// usage examples and everything from a "Pass the full id" or "Decision models"
// line on are skipped.
func ParseModels(output string) []string {
	var twoCol, single []string
	seenTwo, seenSingle := map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Pass the full id") || strings.HasPrefix(line, "Decision models") {
			break
		}
		if line == "" {
			continue
		}
		id, two := splitModelLine(line)
		keep := !strings.Contains(id, " ") && SafeValue(id)
		switch {
		case two && keep && !seenTwo[id]:
			seenTwo[id] = true
			twoCol = append(twoCol, id)
		case !two && keep && !seenSingle[id]:
			seenSingle[id] = true
			single = append(single, id)
		}
	}
	// A two-column listing names each model next to its description; a plain
	// one (OpenCode's `models`) prints one id per line. Headings only look
	// like ids in the plain form, so the two-column ids win when both exist.
	if len(twoCol) > 0 {
		return twoCol
	}
	return single
}

// splitModelLine cuts a listing line at its first run of two or more spaces
// or a tab, which separates the id from its description. It reports whether
// the line had such a separator.
func splitModelLine(line string) (string, bool) {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\t':
			return line[:i], true
		case ' ':
			j := i
			for j < len(line) && line[j] == ' ' {
				j++
			}
			if j-i >= 2 {
				return line[:i], true
			}
			i = j - 1
		}
	}
	return line, false
}

// Models asks the CLI at path for its models. It returns nil when the adapter
// has no list command or the command fails, so the caller can fall back.
func (a *Adapter) Models(path string) []string {
	if len(a.ModelList) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, a.ModelList...)
	cmd.Dir = os.TempDir() // keep the workspace's package-manager config out of it
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return ParseModels(string(out))
}
