package workflow

import (
	"fmt"

	"multiharness-core/internal/store"
)

// Handoff budgets come from the execution policy. Full evidence stays in
// workflow state; agent prompts receive only bounded projections.
func (service *Service) handoffBudget() (maxPrompt, chunkBytes int) {
	maxPrompt = service.execution.MaxPromptBytes
	chunkBytes = service.execution.ReviewChunkBytes
	if maxPrompt <= 0 {
		maxPrompt = 262144
	}
	if chunkBytes <= 0 {
		chunkBytes = 131072
	}
	return maxPrompt, chunkBytes
}

// BuildReviewBatches partitions the unified diff by file and hunk into bounded
// chunks. Every changed file and diff hunk belongs to exactly one batch.
// Canonical repository evidence is authoritative; the implementation claim is
// only a fallback because agents may misreport their changed files.
func BuildReviewBatches(request store.ReviewRequest, chunkBytes int) []store.ReviewChunk {
	diff := ""
	changed := request.Implementation.ChangedFiles
	if request.Repository != nil {
		diff = request.Repository.Diff
		if len(request.Repository.ChangedFiles) > 0 {
			changed = request.Repository.ChangedFiles
		}
	}
	chunks := store.ChunkReviewDiff(diff, changed, chunkBytes)
	root, fp := "", ""
	if request.Repository != nil {
		root = request.Repository.Baseline.Root
		fp = request.Repository.Current.Fingerprint
		if fp == "" {
			fp = request.Repository.Baseline.Fingerprint
		}
	}
	for i := range chunks {
		chunks[i].WorkspaceRoot = root
		chunks[i].WorkspaceFingerprint = fp
	}
	return chunks
}

// ChunkReviewRequest returns the per-batch request for one chunk: shared
// task/plan/validation with only that chunk's diff and files.
func ChunkReviewRequest(request store.ReviewRequest, chunk store.ReviewChunk) store.ReviewRequest {
	out := request
	if out.Repository != nil {
		clone := out.Repository.Clone()
		clone.Diff = chunk.DiffChunk
		clone.ChangedFiles = append([]string(nil), chunk.ChangedFiles...)
		out.Repository = clone
	}
	out.Implementation.ChangedFiles = append([]string(nil), chunk.ChangedFiles...)
	return out
}

// AggregateChunkReviews combines bounded chunk findings. Blocking findings are
// never dropped; repeated findings (a synthesis echoing a chunk) appear once,
// and non-blocking findings and suggestions are capped.
func AggregateChunkReviews(chunkReviews []store.Review, validation store.ValidationReport) store.Review {
	type key struct{ file, description string }
	seen := map[key]bool{}
	var blocking, advisory []store.ReviewFinding
	var suggestions []string
	approved := true
	var action *store.ValidationAction
	for _, r := range chunkReviews {
		approved = approved && r.Approved
		if action == nil {
			action = r.ValidationAction
		}
		for _, f := range r.Findings {
			k := key{f.File, f.Description}
			if seen[k] {
				continue
			}
			seen[k] = true
			if f.Blocking {
				blocking = append(blocking, f)
			} else {
				advisory = append(advisory, f)
			}
		}
		for _, s := range r.Suggestions {
			if len(suggestions) < store.MaxReviewSuggestions {
				suggestions = append(suggestions, s)
			}
		}
	}
	findings := blocking
	for _, f := range advisory {
		if len(findings) >= store.MaxReviewFindings {
			break
		}
		findings = append(findings, f)
	}
	if !validation.Passed {
		approved = false
		if len(blocking) == 0 {
			findings = append([]store.ReviewFinding{{
				Severity: store.FindingSeverityError, Blocking: true,
				Description:    "deterministic validation failed; repair is required",
				Evidence:       "validation report did not pass",
				RequiredAction: "fix the failing checks and rerun validation",
			}}, findings...)
		}
	} else if len(blocking) > 0 {
		approved = false
	}
	if !approved && !hasBlocking(findings) {
		findings = append(findings, store.ReviewFinding{
			Severity: store.FindingSeverityError, Blocking: true,
			Description:    "review chunks did not approve the change",
			Evidence:       "one or more chunk reviews rejected the change",
			RequiredAction: "address the chunk findings and request repairs",
		})
	}
	summary := fmt.Sprintf("aggregated %d chunk reviews; %d blocking findings", len(chunkReviews), countBlocking(findings))
	if len(chunkReviews) == 1 {
		summary = chunkReviews[0].Summary
	}
	return store.Review{ValidationAction: action, Approved: approved, Summary: summary, Findings: findings, Suggestions: suggestions}
}

func hasBlocking(findings []store.ReviewFinding) bool {
	return countBlocking(findings) > 0
}

func countBlocking(findings []store.ReviewFinding) int {
	n := 0
	for _, f := range findings {
		if f.Blocking {
			n++
		}
	}
	return n
}
