package structured

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Budget bounds the complete agent request after instructions, serialized
// payload and output schema have been added. Compact JSON is used throughout.
type Budget struct {
	MaxPromptBytes   int
	ReviewChunkBytes int
}

func DefaultBudget() Budget { return Budget{MaxPromptBytes: 262144, ReviewChunkBytes: 131072} }

func (b Budget) withDefaults() Budget {
	d := DefaultBudget()
	if b.MaxPromptBytes <= 0 {
		b.MaxPromptBytes = d.MaxPromptBytes
	}
	if b.ReviewChunkBytes <= 0 || b.ReviewChunkBytes > b.MaxPromptBytes {
		b.ReviewChunkBytes = min(d.ReviewChunkBytes, b.MaxPromptBytes/2)
	}
	return b
}

// HandoffTooLargeError fails locally before provider execution starts.
type HandoffTooLargeError struct {
	Stage           string
	ActualBytes     int
	LimitBytes      int
	LargestSections map[string]int
}

func (e *HandoffTooLargeError) Error() string {
	names := make([]string, 0, len(e.LargestSections))
	for name, size := range e.LargestSections {
		names = append(names, fmt.Sprintf("%s=%d", name, size))
	}
	sort.Strings(names)
	return fmt.Sprintf("%s handoff is %d bytes, exceeding the %d byte limit (%s); split the task or raise execution.max_prompt_bytes",
		e.Stage, e.ActualBytes, e.LimitBytes, strings.Join(names, ", "))
}

func compact(v any) ([]byte, error) { return json.Marshal(v) }

func sectionSizes(sections map[string][]byte) map[string]int {
	out := make(map[string]int, len(sections))
	for k, v := range sections {
		out[k] = len(v)
	}
	return out
}

func checkBudget(stage string, instructions string, payload []byte, schema []byte, sections map[string][]byte, limit int) error {
	if limit <= 0 {
		limit = DefaultBudget().MaxPromptBytes
	}
	total := len(instructions) + len(payload) + len(schema)
	if total <= limit {
		return nil
	}
	return &HandoffTooLargeError{Stage: stage, ActualBytes: total, LimitBytes: limit, LargestSections: sectionSizes(sections)}
}
