//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package folder

import (
	"strings"
	"testing"
)

func TestUnifiedDiffKeepsOneLineChangeSmall(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("same content line\n", 300000) // >4MiB
	put(t, dir, "big.txt", big)
	session := acquire(t, newWorkspace(t, Config{}), dir)
	changed := strings.Replace(big, "same content line\n", "changed content line\n", 1)
	put(t, dir, "big.txt", changed)
	evidence, err := session.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Complete {
		t.Fatal("evidence incomplete")
	}
	if !strings.Contains(evidence.Diff, "@@") {
		t.Fatalf("expected unified diff with context, got %d bytes", len(evidence.Diff))
	}
	if len(evidence.Diff) > 65536 {
		t.Fatalf("one-line change produced %d diff bytes; expected a small hunk", len(evidence.Diff))
	}
	if !strings.Contains(evidence.Diff, "-same content line") || !strings.Contains(evidence.Diff, "+changed content line") {
		t.Fatalf("hunk missing changed lines")
	}
}

func TestLargeWorkspacePreExistingCountDoesNotBoundPrompt(t *testing.T) {
	// Workspace evidence retains the full pre-existing list; the handoff
	// projection (tested in store/structured) carries only the count.
	dir := t.TempDir()
	for i := 0; i < 50; i++ {
		put(t, dir, "file.txt", "x")
		_ = i
	}
	session := acquire(t, newWorkspace(t, Config{}), dir)
	evidence, err := session.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cleanupRecovery(t, evidence)
	if !evidence.Complete {
		t.Fatal("expected complete canonical evidence")
	}
}
