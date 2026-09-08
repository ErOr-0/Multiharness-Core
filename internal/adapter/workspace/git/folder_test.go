//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package git

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPlainFolderTracksEditsWithoutCreatingRepository(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "app/main.txt", "original\n")
	put(t, dir, ".gitignore", "node_modules/\n")
	put(t, dir, "node_modules/large", strings.Repeat("x", 1000))
	session := acquire(t, newWorkspace(t, Config{MaxFileBytes: 100}), dir)
	if len(session.Baseline().PreExistingFiles) != 0 {
		t.Fatal("plain files must remain editable without a Git clean/dirty distinction")
	}
	put(t, dir, "app/main.txt", "implemented\n")
	put(t, dir, "other/new.txt", "new\n")
	evidence, err := session.Inspect(t.Context())
	if err != nil || !evidence.Complete || len(evidence.PreservationViolations) != 0 {
		t.Fatalf("inspect: %#v %v", evidence, err)
	}
	if !reflect.DeepEqual(evidence.ChangedFiles, []string{"app/main.txt", "other/new.txt"}) || !strings.Contains(evidence.Diff, "-original") {
		t.Fatalf("folder evidence: %#v", evidence)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inspection created repository metadata", err)
	}
	// Changing ignore rules must not hide a file already captured in the baseline.
	put(t, dir, ".gitignore", "app/\nnode_modules/\n")
	evidence, err = session.Inspect(t.Context())
	if err != nil || !strings.Contains(evidence.Diff, "+implemented") {
		t.Fatalf("baseline hidden by ignore rule: %#v %v", evidence, err)
	}
}

func TestParentFolderCombinesRepositoriesAndProtectsTheirDirtyFiles(t *testing.T) {
	for _, outerGit := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain parent", true: "nested repositories"}[outerGit], func(t *testing.T) {
			dir := t.TempDir()
			if outerGit {
				runGit(t, dir, "init", "-q")
			}
			for _, project := range []string{"api", "web"} {
				put(t, dir, project+"/main.txt", "original\n")
				put(t, dir, project+"/.gitignore", "build/\n")
				root := filepath.Join(dir, project)
				runGit(t, root, "init", "-q")
				runGit(t, root, "add", ".")
				runGit(t, root, "commit", "-qm", "baseline")
			}
			put(t, dir, "api/notes.txt", "user draft\n")
			put(t, dir, "web/build/artifact", "ignored\n")
			session := acquire(t, newWorkspace(t, Config{}), dir)
			if !reflect.DeepEqual(session.Baseline().PreExistingFiles, []string{"api/notes.txt"}) {
				t.Fatalf("protected paths: %v", session.Baseline().PreExistingFiles)
			}
			put(t, dir, "api/main.txt", "API change\n")
			put(t, dir, "web/main.txt", "web change\n")
			evidence, err := session.Inspect(t.Context())
			if err != nil || !evidence.Complete || len(evidence.PreservationViolations) != 0 ||
				!reflect.DeepEqual(evidence.ChangedFiles, []string{"api/main.txt", "web/main.txt"}) {
				t.Fatalf("combined evidence: %#v %v", evidence, err)
			}
			put(t, dir, "api/notes.txt", "overwritten\n")
			runGit(t, filepath.Join(dir, "web"), "add", "main.txt")
			evidence, err = session.Inspect(t.Context())
			cleanupRecovery(t, evidence)
			if err != nil || !reflect.DeepEqual(evidence.PreservationViolations, []string{"api/notes.txt", "[Git index: web]"}) {
				t.Fatalf("nested preservation: %#v %v", evidence, err)
			}
			backup, err := os.ReadFile(filepath.Join(evidence.RecoveryDirectory, "files", "api", "notes.txt"))
			if err != nil || string(backup) != "user draft\n" {
				t.Fatal("missing original nested file", err)
			}
		})
	}
}

func TestRepositorySubfolderLimitsFileEvidenceToSelectedFolder(t *testing.T) {
	dir := repository(t)
	put(t, dir, "sub/main.txt", "subproject\n")
	runGit(t, dir, "add", "sub")
	runGit(t, dir, "commit", "-qm", "subproject")
	session := acquire(t, newWorkspace(t, Config{}), filepath.Join(dir, "sub"))
	put(t, dir, "sub/main.txt", "changed\n")
	put(t, dir, "notes.txt", "outside selected folder\n")
	evidence, err := session.Inspect(t.Context())
	if err != nil || !reflect.DeepEqual(evidence.ChangedFiles, []string{"main.txt"}) || strings.Contains(evidence.Diff, "outside selected folder") {
		t.Fatalf("subfolder scope: %#v %v", evidence, err)
	}
}

func TestNestedIndexIsProtectedWhenParentAlsoTracksItsFiles(t *testing.T) {
	dir := repository(t)
	put(t, dir, "nested/main.txt", "original\n")
	runGit(t, dir, "add", "nested")
	runGit(t, dir, "commit", "-qm", "parent tracks project")
	nested := filepath.Join(dir, "nested")
	runGit(t, nested, "init", "-q")
	runGit(t, nested, "add", ".")
	runGit(t, nested, "commit", "-qm", "nested baseline")
	session := acquire(t, newWorkspace(t, Config{}), dir)
	put(t, nested, "main.txt", "changed\n")
	runGit(t, nested, "add", "main.txt")
	evidence, err := session.Inspect(t.Context())
	cleanupRecovery(t, evidence)
	if err != nil || !reflect.DeepEqual(evidence.PreservationViolations, []string{"[Git index: nested]"}) {
		t.Fatalf("nested index not protected: %#v %v", evidence, err)
	}
}

func TestRepositoryCreationCannotHidePlainFolderChanges(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "app/main.txt", "original\n")
	session := acquire(t, newWorkspace(t, Config{}), dir)
	runGit(t, filepath.Join(dir, "app"), "init", "-q")
	put(t, dir, "app/.gitignore", "main.txt\n")
	put(t, dir, "app/main.txt", "changed\n")
	evidence, err := session.Inspect(t.Context())
	cleanupRecovery(t, evidence)
	if err != nil || len(evidence.PreservationViolations) == 0 || !strings.Contains(evidence.Diff, "+changed") {
		t.Fatalf("new metadata hid changes: %#v %v", evidence, err)
	}
}

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
