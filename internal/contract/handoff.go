package contract

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Bounded handoff limits for model-controlled contracts. Blocking findings are
// never dropped to fit; callers create another review batch or fail closed.
const (
	MaxImplementationSummaryBytes = 8 << 10
	MaxChangedFiles               = 256
	MaxChangedFilePathBytes       = 1024
	MaxReviewSummaryBytes         = 8 << 10
	MaxReviewFindings             = 128
	MaxReviewSuggestions          = 64
	MaxFindingTextBytes           = 4 << 10
	MaxFindingEvidenceBytes       = 8 << 10
	MaxSuggestionBytes            = 2 << 10
	MaxValidationHandoffBytes     = 64 << 10
	MaxReviewBatchFiles           = 256
)

// ImplementationHandoff is the bounded projection sent to implementers.
// It never includes repository.diff or the complete pre_existing_files array.
type ImplementationHandoff struct {
	Task                   string             `json:"task"`
	WorkingDir             string             `json:"working_dir"`
	RecentTurns            []ConversationTurn `json:"recent_turns,omitempty"`
	Plan                   Plan               `json:"plan"`
	WorkspaceRoot          string             `json:"workspace_root"`
	WorkspaceFingerprint   string             `json:"workspace_fingerprint"`
	PreExistingFileCount   int                `json:"pre_existing_file_count"`
	ExistingWorkAuthorized bool               `json:"existing_work_authorized"`
	RecoveryDirectory      string             `json:"recovery_directory,omitempty"`
	ChangedFiles           []string           `json:"changed_files,omitempty"`
	ChangedFilesTruncated  bool               `json:"changed_files_truncated,omitempty"`
	ImplementationSummary  string             `json:"implementation_summary,omitempty"`
}

// ReviewChunk carries one bounded unified-diff chunk with completeness markers.
type ReviewChunk struct {
	ChunkIndex           int               `json:"chunk_index"`
	ChunkCount           int               `json:"chunk_count"`
	ChangedFiles         []string          `json:"changed_files"`
	BeforeHashes         map[string]string `json:"before_hashes,omitempty"`
	AfterHashes          map[string]string `json:"after_hashes,omitempty"`
	BeforeSizes          map[string]int64  `json:"before_sizes,omitempty"`
	AfterSizes           map[string]int64  `json:"after_sizes,omitempty"`
	DiffChunk            string            `json:"diff_chunk"`
	WorkspaceRoot        string            `json:"workspace_root"`
	WorkspaceFingerprint string            `json:"workspace_fingerprint"`
	Complete             bool              `json:"complete"`
}

// ValidationHandoff is bounded per-check output with failures preserved first.
type ValidationHandoff struct {
	Passed          bool                 `json:"passed"`
	Checks          []ValidationEvidence `json:"checks"`
	Truncated       bool                 `json:"truncated"`
	BytesRetained   int                  `json:"bytes_retained"`
	TotalCheckCount int                  `json:"total_check_count"`
}

// RepairHandoff is delta-oriented: blocking findings plus relevant hunks only.
type RepairHandoff struct {
	Task                   string               `json:"task"`
	WorkingDir             string               `json:"working_dir"`
	RecentTurns            []ConversationTurn   `json:"recent_turns,omitempty"`
	Plan                   Plan                 `json:"plan"`
	ImplementationSummary  string               `json:"implementation_summary"`
	ChangedFiles           []string             `json:"changed_files"`
	ChangedFilesTruncated  bool                 `json:"changed_files_truncated,omitempty"`
	ReviewSummary          string               `json:"review_summary"`
	BlockingFindings       []ReviewFinding      `json:"blocking_findings"`
	RelevantDiff           string               `json:"relevant_diff,omitempty"`
	FailedValidation       []ValidationEvidence `json:"failed_validation"`
	PassingSummary         []string             `json:"passing_summary,omitempty"`
	WorkspaceRoot          string               `json:"workspace_root"`
	WorkspaceFingerprint   string               `json:"workspace_fingerprint"`
	PreExistingFileCount   int                  `json:"pre_existing_file_count"`
	ExistingWorkAuthorized bool                 `json:"existing_work_authorized"`
	RecoveryDirectory      string               `json:"recovery_directory,omitempty"`
}

// HandoffDiagnostics publishes metadata only, never prompt contents.
type HandoffDiagnostics struct {
	Stage                   string         `json:"stage"`
	PromptBytes             int            `json:"prompt_bytes"`
	SectionSizes            map[string]int `json:"section_sizes,omitempty"`
	PreExistingFileCount    int            `json:"pre_existing_file_count"`
	RawDiffBytes            int            `json:"raw_diff_bytes"`
	ReviewChunkCount        int            `json:"review_chunk_count"`
	ValidationBytesRetained int            `json:"validation_bytes_retained"`
	Attempt                 int            `json:"attempt"`
	Outcome                 string         `json:"outcome,omitempty"`
}

// ProjectImplementation builds the bounded implementation projection.
func ProjectImplementation(input TaskInput, plan Plan, repo *RepositoryEvidence, impl *ImplementationResult) ImplementationHandoff {
	h := ImplementationHandoff{
		Task: input.Task, WorkingDir: input.WorkingDir,
		RecentTurns: append([]ConversationTurn(nil), input.RecentTurns...),
		Plan:        plan,
	}
	if repo != nil {
		h.WorkspaceRoot = repo.Baseline.Root
		fp := repo.Current.Fingerprint
		if fp == "" {
			fp = repo.Baseline.Fingerprint
		}
		h.WorkspaceFingerprint = fp
		h.PreExistingFileCount = len(repo.PreExistingFiles)
		h.ExistingWorkAuthorized = repo.ExistingWorkAuthorized
		h.RecoveryDirectory = repo.RecoveryDirectory
	}
	if impl != nil {
		h.ChangedFiles = append([]string(nil), impl.ChangedFiles...)
		h.ImplementationSummary = impl.Summary
	}
	// The changed-file manifest (names only, never diff content) keeps partial
	// work visible across permission retries; canonical diffs stay in workflow
	// state and the agent inspects live files itself.
	if len(h.ChangedFiles) == 0 && repo != nil && len(repo.ChangedFiles) > 0 {
		h.ChangedFiles = append([]string(nil), repo.ChangedFiles...)
	}
	if len(h.ChangedFiles) > MaxChangedFiles {
		h.ChangedFiles = h.ChangedFiles[:MaxChangedFiles]
		h.ChangedFilesTruncated = true
	}
	return h
}

// ProjectRepair builds the delta-oriented repair projection: blocking findings,
// hunks for the files they name, and failed validation output (passing checks
// are reduced to one line each). The cumulative baseline diff is never sent.
func ProjectRepair(request RepairRequest, relevantDiffBytes int) RepairHandoff {
	impl := ProjectImplementation(request.Input, request.Plan, request.Repository, &request.Implementation)
	h := RepairHandoff{
		Task: impl.Task, WorkingDir: impl.WorkingDir, RecentTurns: impl.RecentTurns, Plan: impl.Plan,
		ImplementationSummary: impl.ImplementationSummary, ChangedFiles: impl.ChangedFiles,
		ChangedFilesTruncated: impl.ChangedFilesTruncated,
		ReviewSummary:         request.Review.Summary,
		BlockingFindings:      []ReviewFinding{},
		FailedValidation:      []ValidationEvidence{},
		WorkspaceRoot:         impl.WorkspaceRoot,
		WorkspaceFingerprint:  impl.WorkspaceFingerprint,
		PreExistingFileCount:  impl.PreExistingFileCount, ExistingWorkAuthorized: impl.ExistingWorkAuthorized,
		RecoveryDirectory: impl.RecoveryDirectory,
	}
	for _, f := range request.Review.Findings {
		if f.Blocking {
			h.BlockingFindings = append(h.BlockingFindings, f)
		}
	}
	for _, c := range ProjectValidation(request.Validation, MaxValidationHandoffBytes).Checks {
		if c.Passed {
			h.PassingSummary = append(h.PassingSummary, c.Command+": passed")
		} else {
			h.FailedValidation = append(h.FailedValidation, c)
		}
	}
	if request.Repository != nil {
		h.RelevantDiff = RelevantDiff(request.Repository.Diff, h.BlockingFindings, relevantDiffBytes)
	}
	return h
}

// ProjectValidation bounds validation output, preserving failures first.
func ProjectValidation(report ValidationReport, maxBytes int) ValidationHandoff {
	if maxBytes <= 0 {
		maxBytes = MaxValidationHandoffBytes
	}
	out := ValidationHandoff{Passed: report.Passed, TotalCheckCount: len(report.Checks)}
	// Failures first, then passes, preserving original relative order within each group.
	failed := make([]ValidationEvidence, 0, len(report.Checks))
	passed := make([]ValidationEvidence, 0, len(report.Checks))
	for _, c := range report.Checks {
		if c.Passed {
			passed = append(passed, c)
		} else {
			failed = append(failed, c)
		}
	}
	budget := maxBytes
	kept := make([]ValidationEvidence, 0, len(report.Checks))
	for _, c := range append(failed, passed...) {
		need := len(c.Output)
		if need > budget {
			truncated := c
			truncated.Output = tailText(c.Output, budget)
			truncated.OutputTruncated = true
			kept = append(kept, truncated)
			out.Truncated = true
			out.BytesRetained += len(truncated.Output)
			budget = 0
			continue
		}
		kept = append(kept, c)
		out.BytesRetained += need
		budget -= need
	}
	out.Checks = kept
	return out
}

// RelevantDiff returns diff hunks for files named by blocking findings.
// It never returns the complete cumulative baseline diff when findings name
// files it can attribute; otherwise it falls back to the bounded leading diff.
func RelevantDiff(fullDiff string, findings []ReviewFinding, maxBytes int) string {
	files := map[string]bool{}
	for _, f := range findings {
		if strings.TrimSpace(f.File) != "" {
			files[f.File] = true
		}
	}
	sections := splitDiffByFile(fullDiff)
	var relevant []string
	for _, section := range sections {
		name := diffChunkFile(section)
		for f := range files {
			if name == f || strings.HasSuffix(name, "/"+f) || strings.HasSuffix(f, "/"+name) {
				relevant = append(relevant, section)
				break
			}
		}
	}
	if len(relevant) == 0 {
		relevant = sections
	}
	return boundDiff(relevant, maxBytes)
}

const diffTruncatedMarker = "(diff truncated to fit the handoff; read the workspace files for the rest)\n"

// boundDiff keeps whole file sections while they fit. The first section that
// does not fit keeps its file header and leading hunks, cut at a hunk boundary
// (or a line boundary inside one oversized hunk), and is followed by a marker.
// Bounded evidence therefore never starts mid-hunk or splits a character.
func boundDiff(sections []string, maxBytes int) string {
	var b strings.Builder
	for _, section := range sections {
		if maxBytes <= 0 || b.Len()+len(section) <= maxBytes {
			b.WriteString(section)
			continue
		}
		if room := maxBytes - b.Len() - len(diffTruncatedMarker); room > 0 {
			cut := strings.LastIndex(section[:room], "\n@@ ")
			if cut <= 0 {
				cut = strings.LastIndex(section[:room], "\n")
			}
			b.WriteString(section[:cut+1])
		}
		b.WriteString(diffTruncatedMarker)
		break
	}
	return b.String()
}

// ChunkReviewDiff partitions a unified diff by file and hunk into bounded chunks.
// Small multi-file diffs pack into one batch so common changes need one review
// call; oversized files split at hunk boundaries and each continuation keeps
// its file header. Every diff hunk is assigned to exactly one batch.
func ChunkReviewDiff(diff string, changedFiles []string, chunkBytes int) []ReviewChunk {
	if chunkBytes <= 0 {
		chunkBytes = 131072
	}
	type piece struct{ text, file string }
	var pieces []piece
	covered := map[string]bool{}
	for _, section := range splitDiffByFile(diff) {
		file := diffChunkFile(section)
		header := ""
		if file != "" {
			covered[file] = true
			if first, rest, ok := strings.Cut(section, "\n"); ok {
				if second, _, ok := strings.Cut(rest, "\n"); ok {
					header = first + "\n" + second + "\n"
				}
			}
			if len(header) > chunkBytes/2 {
				header = ""
			}
		}
		for len(section) > chunkBytes {
			cut := chunkBytes
			if i := strings.LastIndex(section[:cut], "\n@@ "); i >= len(header) {
				cut = i + 1
			} else if i := strings.LastIndex(section[:cut], "\n"); i >= len(header) {
				cut = i + 1
			}
			pieces = append(pieces, piece{section[:cut], file})
			section = header + section[cut:]
		}
		pieces = append(pieces, piece{section, file})
	}
	headerless := len(covered) == 0
	if !headerless {
		for _, f := range changedFiles {
			if !covered[f] {
				pieces = append(pieces, piece{fmt.Sprintf("--- %q\n+++ %q\n(no textual hunks captured)\n", "before/"+f, "after/"+f), f})
				covered[f] = true
			}
		}
	}
	if len(pieces) == 0 {
		pieces = []piece{{}}
	}
	// Greedily pack pieces so small multi-file diffs fit in one batch.
	type batch struct {
		text  strings.Builder
		files []string
	}
	var batches []*batch
	for _, p := range pieces {
		if len(batches) == 0 || batches[len(batches)-1].text.Len()+len(p.text) > chunkBytes {
			batches = append(batches, &batch{files: []string{}})
		}
		current := batches[len(batches)-1]
		current.text.WriteString(p.text)
		if p.file != "" && !slices.Contains(current.files, p.file) {
			current.files = append(current.files, p.file)
		}
	}
	// A headerless diff cannot attribute hunks; the first batch carries the manifest.
	if headerless {
		batches[0].files = append([]string{}, changedFiles...)
	}
	out := make([]ReviewChunk, 0, len(batches))
	for i, b := range batches {
		out = append(out, ReviewChunk{
			ChunkIndex: i, ChunkCount: len(batches),
			ChangedFiles: b.files,
			DiffChunk:    b.text.String(), Complete: true,
		})
	}
	return out
}

func splitDiffByFile(diff string) []string {
	if diff == "" {
		return nil
	}
	var chunks []string
	var cur strings.Builder
	for _, line := range strings.SplitAfter(diff, "\n") {
		if diffHeaderFile(line) != "" && cur.Len() > 0 {
			chunks = append(chunks, cur.String())
			cur.Reset()
		}
		cur.WriteString(line)
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

// diffChunkFile names the file of a section produced by splitDiffByFile.
func diffChunkFile(chunk string) string {
	line, _, _ := strings.Cut(chunk, "\n")
	return diffHeaderFile(line)
}

// diffHeaderFile parses workspace diff headers, which quote names with %q
// ("--- \"before/a.go\""); the unquoted form is accepted for hand-built diffs.
func diffHeaderFile(line string) string {
	name, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "--- ")
	if !ok {
		return ""
	}
	if unquoted, err := strconv.Unquote(name); err == nil {
		name = unquoted
	}
	name, ok = strings.CutPrefix(name, "before/")
	if !ok {
		return ""
	}
	return name
}

// tailText keeps the last maxBytes of s without splitting a character; the end
// of command output is where failures are reported.
func tailText(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}
