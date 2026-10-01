package store_test

import (
	"fmt"
	"strings"
	"testing"

	"multiharness-core/internal/store"
)

func TestImplementationHandoffOmitsDiffAndFileList(t *testing.T) {
	repo := &store.RepositoryEvidence{
		Baseline:         store.RepositoryState{Root: "/w", Fingerprint: "base"},
		Current:          store.RepositoryState{Root: "/w", Fingerprint: "cur"},
		Complete:         true,
		Diff:             "huge diff",
		PreExistingFiles: make([]string, 20000),
	}
	for i := range repo.PreExistingFiles {
		repo.PreExistingFiles[i] = "file.txt"
	}
	plan := store.Plan{Action: store.PlanActionImplement, Summary: "s", Steps: []string{"a"}, AcceptanceCriteria: []string{"b"}}
	input := store.TaskInput{Task: "do it", WorkingDir: "/w"}
	h := store.ProjectImplementation(input, plan, repo, nil)
	if h.PreExistingFileCount != 20000 {
		t.Fatalf("count = %d", h.PreExistingFileCount)
	}
	if h.WorkspaceRoot != "/w" || h.WorkspaceFingerprint != "cur" {
		t.Fatalf("workspace identity lost: %+v", h)
	}
}

func TestChunkReviewDiffCoversEveryHunk(t *testing.T) {
	var b strings.Builder
	files := []string{"a.go", "b.go", "c.go"}
	for _, f := range files {
		b.WriteString("--- before/" + f + "\n+++ after/" + f + "\n")
		b.WriteString("@@ -1,2 +1,2 @@\n")
		b.WriteString(" context\n-old\n+new\n")
	}
	chunks := store.ChunkReviewDiff(b.String(), files, 150)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if len(c.DiffChunk) > 150 && !strings.Contains(c.DiffChunk, "@@") {
			t.Fatalf("oversized chunk without hunk boundary")
		}
		if c.ChunkCount != len(chunks) || !c.Complete {
			t.Fatalf("bad completeness markers: %+v", c)
		}
	}
	// Every changed file assigned to exactly one batch manifest.
	seen := map[string]int{}
	for _, c := range chunks {
		for _, f := range c.ChangedFiles {
			seen[f]++
		}
	}
	for _, f := range files {
		if seen[f] != 1 {
			t.Fatalf("file %s assigned %d times", f, seen[f])
		}
	}
}

func TestProjectValidationKeepsFailuresFirst(t *testing.T) {
	report := store.ValidationReport{Passed: false, Checks: []store.ValidationEvidence{
		{Command: "pass", Passed: true, ExitCode: 0, Output: strings.Repeat("p", 100)},
		{Command: "fail", Passed: false, ExitCode: 1, Output: strings.Repeat("f", 100)},
	}}
	h := store.ProjectValidation(report, 120)
	if h.Passed {
		t.Fatal("should not pass")
	}
	foundFail := false
	for _, c := range h.Checks {
		if !c.Passed {
			foundFail = true
		}
	}
	if !foundFail {
		t.Fatal("blocking failure dropped")
	}
}

func TestRelevantDiffOmitsCumulativeBaseline(t *testing.T) {
	diff := "--- before/a.go\n+++ after/a.go\n@@ -1 +1 @@\n-a\n+b\n" +
		"--- before/unrelated.go\n+++ after/unrelated.go\n@@ -1 +1 @@\n-x\n+y\n"
	out := store.RelevantDiff(diff, []store.ReviewFinding{{File: "a.go", Blocking: true}}, 1<<20)
	if strings.Contains(out, "unrelated.go") {
		t.Fatalf("repair carried cumulative diff: %q", out)
	}
	if !strings.Contains(out, "a.go") {
		t.Fatalf("relevant hunk missing: %q", out)
	}
}

// Workspace diffs quote headers with %q; attribution must not fall back to the
// full manifest or the cumulative diff tail.
func TestQuotedWorkspaceHeadersAttributeHunks(t *testing.T) {
	section := func(name, body string) string {
		return fmt.Sprintf("--- %q\n+++ %q\n", "before/"+name, "after/"+name) + body
	}
	big := "@@ -1,1 +1,1 @@\n" + strings.Repeat("-old line\n+new line\n", 20)
	diff := section("dir/a b.go", big) + section("unrelated.go", "@@ -1 +1 @@\n-x\n+y\n")
	chunks := store.ChunkReviewDiff(diff, []string{"dir/a b.go", "unrelated.go"}, 200)
	if len(chunks) < 3 {
		t.Fatalf("expected the large file to split, got %d chunks", len(chunks))
	}
	var joined strings.Builder
	for _, c := range chunks {
		if len(c.ChangedFiles) != 1 {
			t.Fatalf("chunk %d attributed to %v", c.ChunkIndex, c.ChangedFiles)
		}
		if !strings.HasPrefix(c.DiffChunk, "--- \"before/") {
			t.Fatalf("chunk %d lost its file header: %q", c.ChunkIndex, c.DiffChunk)
		}
		joined.WriteString(c.DiffChunk)
	}
	if got := strings.Count(joined.String(), "+new line\n"); got != 20 {
		t.Fatalf("hunk lines duplicated or dropped: %d", got)
	}
	out := store.RelevantDiff(diff, []store.ReviewFinding{{File: "unrelated.go", Blocking: true}}, 1<<20)
	if strings.Contains(out, "a b.go") || !strings.Contains(out, "unrelated.go") {
		t.Fatalf("relevant diff not scoped to the finding: %q", out)
	}
}
