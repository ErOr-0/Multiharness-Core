//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package folder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotDirectoryRejectsSymlinkAncestors(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "real/nested/source.txt", "source")
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, name := range []string{"alias", "alias/nested"} {
		if handle, err := openSnapshotDirectory(root, name); err == nil {
			if handle != nil {
				_ = handle.Close()
			}
			t.Fatalf("followed ancestor symlink: %s", name)
		}
	}
	handle, err := openSnapshotDirectory(root, "real/nested")
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	file, err := readFile(handle, "source.txt", 0)
	if err != nil || string(file.data) != "source" {
		t.Fatal(file, err)
	}
	if handle, err := openSnapshotDirectory(root, "missing/nested"); err != nil || handle != nil {
		t.Fatal("missing baseline directory must remain missing", handle, err)
	}
}

func TestParallelSnapshotKeepsNestedIgnoreRulesSeparate(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine"} {
		put(t, dir, name+"/.gitignore", "*.tmp\n!keep.tmp\n")
		put(t, dir, name+"/keep.tmp", "include")
		put(t, dir, name+"/drop.tmp", "exclude")
	}
	put(t, dir, "two/.magentignore", "keep.tmp\n")
	w := newWorkspace(t, Config{})
	s, err := w.stableCapture(t.Context(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := s.files["one/keep.tmp"]; !exists {
		t.Fatal("negated ignore lost")
	}
	if _, exists := s.files["two/keep.tmp"]; exists {
		t.Fatal("nested override lost")
	}
	if _, exists := s.files["three/drop.tmp"]; exists {
		t.Fatal("ignored file included")
	}
}
