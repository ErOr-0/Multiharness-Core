//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package folder

import (
	"context"
	"errors"
	"fmt"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func put(t *testing.T, dir, name, content string) {
	t.Helper()
	target := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func repository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	put(t, dir, "main.txt", "original\n")
	put(t, dir, "notes.txt", "notes\n")
	return dir
}
func newWorkspace(t *testing.T, c Config) *Workspace {
	t.Helper()
	if c.RecoveryDir == "" {
		c.RecoveryDir = t.TempDir()
	}
	w, err := NewWorkspace(c)
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func acquire(t *testing.T, w *Workspace, dir string) workflow.WorkspaceSession {
	t.Helper()
	s, err := w.Acquire(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func cleanupRecovery(t *testing.T, e store.RepositoryEvidence) { t.Helper() }

func TestLockCoversAliasesInstancesAndIsReleased(t *testing.T) {
	dir := repository(t)
	first := acquire(t, newWorkspace(t, Config{}), dir)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("skipping symlink test on Windows without privilege")
		}
		t.Fatal(err)
	}
	second := newWorkspace(t, Config{})
	if _, err := second.Acquire(t.Context(), alias); !errors.Is(err, ErrBusy) {
		t.Fatalf("second lock: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	_ = acquire(t, second, alias)
}

func TestCancellationAndDiffOverflowReturnIncompleteEvidence(t *testing.T) {
	dir := repository(t)
	session := acquire(t, newWorkspace(t, Config{MaxOutputBytes: 512}), dir)
	put(t, dir, "main.txt", strings.Repeat("a large changed line\n", 200))
	evidence, err := session.Inspect(t.Context())
	cleanupRecovery(t, evidence)
	if err == nil || evidence.Complete || evidence.RecoveryDirectory == "" {
		t.Fatalf("overflow: %#v %v", evidence, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	evidence, err = session.Inspect(ctx)
	cleanupRecovery(t, evidence)
	if !errors.Is(err, context.Canceled) || evidence.Complete {
		t.Fatalf("cancel: %#v %v", evidence, err)
	}
}

func TestDiffPreservesAmbiguousPathsAndContents(t *testing.T) {
	dir := repository(t)
	session := acquire(t, newWorkspace(t, Config{}), dir)
	name := "folder with spaces/a/before/literal.txt"
	put(t, dir, name, "a/before/literal\n")
	evidence, err := session.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(evidence.Diff, name) || !strings.Contains(evidence.Diff, "+a/before/literal") {
		t.Fatalf("diff rewrote a path or file contents: %s", evidence.Diff)
	}
}

func TestUnlimitedWorkspaceCapturesLargeFilesAndCompleteDiff(t *testing.T) {
	dir := t.TempDir()
	// A single file exceeds both former defaults: 8 MiB/file and 64 MiB total.
	large, err := os.Create(filepath.Join(dir, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	err = large.Truncate(65 << 20)
	closeErr := large.Close()
	if err != nil || closeErr != nil {
		t.Fatal(errors.Join(err, closeErr))
	}
	session := acquire(t, newWorkspace(t, Config{}), dir)
	// Evidence must survive both the former 4 MiB Git cap and the runner's
	// bounded diagnostic tail. Check the beginning and end, not just its size.
	content := "first-evidence-line\n" + strings.Repeat("changed evidence line\n", 220000) + "last-evidence-line\n"
	put(t, dir, "change.txt", content)
	evidence, err := session.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Complete || !reflect.DeepEqual(evidence.ChangedFiles, []string{"change.txt"}) {
		t.Fatalf("incomplete change attribution: complete=%v files=%v", evidence.Complete, evidence.ChangedFiles)
	}
	if len(evidence.Diff) <= 4<<20 || !strings.Contains(evidence.Diff, "+first-evidence-line") || !strings.Contains(evidence.Diff, "+last-evidence-line") {
		t.Fatalf("diff evidence missing or truncated: %d bytes", len(evidence.Diff))
	}
}

func TestUnlimitedWorkspaceFileCount(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20001; i++ {
		put(t, dir, fmt.Sprintf("file-%05d", i), "")
	}
	session := acquire(t, newWorkspace(t, Config{}), dir)
	put(t, dir, "file-20000", "updated\n")
	evidence, err := session.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Complete || !reflect.DeepEqual(evidence.ChangedFiles, []string{"file-20000"}) {
		t.Fatalf("large workspace lost evidence: complete=%v files=%v", evidence.Complete, evidence.ChangedFiles)
	}
}
