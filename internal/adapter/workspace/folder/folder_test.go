//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package folder

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFolderLocksExcludeOverlapsButAllowSiblings(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "first/file", "one")
	put(t, dir, "second/file", "two")
	workspace := newWorkspace(t, Config{})
	parent := acquire(t, workspace, dir)
	if lease, err := workspace.Acquire(t.Context(), filepath.Join(dir, "first")); !errors.Is(err, ErrBusy) {
		if lease != nil {
			_ = lease.Close()
		}
		t.Fatalf("parent lock did not exclude child: %v", err)
	}
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	child := acquire(t, workspace, filepath.Join(dir, "first"))
	_ = acquire(t, workspace, filepath.Join(dir, "second"))
	if lease, err := workspace.Acquire(t.Context(), dir); !errors.Is(err, ErrBusy) {
		if lease != nil {
			_ = lease.Close()
		}
		t.Fatalf("child lock did not exclude parent: %v", err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFolderScopeIgnoresMetadataAndRetainsBaselineAcrossIgnoreChanges(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	outer := t.TempDir()
	put(t, outer, "outside.txt", "do not inspect")
	put(t, outer, "project/main.txt", "before")
	put(t, outer, "project/.git", "broken metadata")
	put(t, outer, "project/.gitignore", "cache/\n*.log\n!keep.log\n")
	put(t, outer, "project/cache/generated", "ignored")
	put(t, outer, "project/keep.log", "include")
	put(t, outer, "project/drop.log", "exclude")
	put(t, outer, "project/sub/.magentignore", "secret.txt\n")
	put(t, outer, "project/sub/secret.txt", "excluded")
	put(t, outer, "project/sub/app.txt", "subproject")
	dir := filepath.Join(outer, "project")
	s := acquire(t, newWorkspace(t, Config{}), dir)
	if got := s.Baseline().PreExistingFiles; strings.Contains(strings.Join(got, ","), "outside") || strings.Contains(strings.Join(got, ","), "secret.txt") || !slices.Contains(got, "keep.log") || slices.Contains(got, "drop.log") {
		t.Fatal("wrong folder scope", got)
	}
	put(t, dir, ".gitignore", "main.txt\ncache/\n*.log\n!keep.log\n")
	put(t, dir, "main.txt", "after")
	put(t, dir, ".git", "still not repository metadata")
	e, err := s.Inspect(t.Context())
	if err != nil || !slices.Contains(e.ChangedFiles, "main.txt") || slices.Contains(e.ChangedFiles, ".git") || len(e.PreservationViolations) != 0 {
		t.Fatal("folder change lost", e, err)
	}
}

func TestFolderPreservesOriginalsAndDoesNotFollowSymlinkTargets(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	put(t, outside, "private", "outside content")
	if err := os.Symlink(filepath.Join(outside, "private"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "app.txt", "original")
	s := acquire(t, newWorkspace(t, Config{ExistingWork: "preserve"}), dir)
	put(t, dir, "app.txt", "changed")
	e, err := s.Inspect(t.Context())
	if err != nil || !slices.Contains(e.PreservationViolations, "app.txt") {
		t.Fatal("preserve mode ignored", e, err)
	}
	original, err := os.ReadFile(filepath.Join(e.RecoveryDirectory, "files/app.txt"))
	if err != nil || string(original) != "original" {
		t.Fatal("original lost", err)
	}
	target, err := os.Readlink(filepath.Join(e.RecoveryDirectory, "files/link"))
	if err != nil || target != filepath.Join(outside, "private") {
		t.Fatal("symlink dereferenced", err)
	}
}

func TestFolderSnapshotLimitsStopBeforeEdits(t *testing.T) {
	for _, c := range []Config{{MaxFiles: 1}, {MaxFileBytes: 2}, {MaxSnapshotBytes: 3}} {
		dir := t.TempDir()
		put(t, dir, "one", "123")
		put(t, dir, "two", "456")
		if s, err := newWorkspace(t, c).Acquire(t.Context(), dir); err == nil {
			_ = s.Close()
			t.Fatal("snapshot limit ignored")
		}
	}
}
