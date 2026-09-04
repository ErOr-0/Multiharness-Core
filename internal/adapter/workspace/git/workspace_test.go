//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly || windows

package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	result, err := process.NewOSRunner().Run(t.Context(), process.Command{Name: "git", Dir: dir,
		Args: append([]string{"-c", "user.name=Tests", "-c", "user.email=tests@example.invalid", "-c", "commit.gpgsign=false"}, args...)})
	if err != nil {
		t.Fatalf("git %v: %v; %s", args, err, result.Stderr)
	}
	return result.Stdout
}

func put(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func repository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	put(t, dir, "main.txt", "original\n")
	put(t, dir, "notes.txt", "notes\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "baseline")
	return dir
}

func newWorkspace(t *testing.T, config Config) *Workspace {
	t.Helper()
	workspace, err := NewWorkspace(process.NewOSRunner(), config)
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func acquire(t *testing.T, workspace *Workspace, dir string) workflow.WorkspaceSession {
	t.Helper()
	session, err := workspace.Acquire(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	return session
}

func cleanupRecovery(t *testing.T, evidence store.RepositoryEvidence) {
	t.Helper()
	if evidence.RecoveryDirectory != "" {
		t.Cleanup(func() { _ = os.RemoveAll(evidence.RecoveryDirectory) })
	}
}

func TestSnapshotsSeparateUserChangesAndDeriveActualDiff(t *testing.T) {
	dir := repository(t)
	put(t, dir, "notes.txt", "staged user notes\n")
	runGit(t, dir, "add", "notes.txt")
	put(t, dir, "notes.txt", "unstaged user notes\n")
	put(t, dir, "untracked user.txt", "draft\n")
	beforeStatus := runGit(t, dir, "status", "--porcelain=v1")
	session := acquire(t, newWorkspace(t, Config{}), dir)
	baseline := session.Baseline()
	if !reflect.DeepEqual(baseline.PreExistingFiles, []string{"notes.txt", "untracked user.txt"}) {
		t.Fatalf("baseline = %#v", baseline)
	}
	if after := runGit(t, dir, "status", "--porcelain=v1"); after != beforeStatus {
		t.Fatal("snapshot changed index or checkout")
	}
	put(t, dir, "main.txt", "implemented\n")
	newFile := "new\nfile.txt"
	if runtime.GOOS == "windows" {
		newFile = "new_file.txt"
	}
	put(t, dir, newFile, "new contents\n")
	evidence, err := session.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(evidence.ChangedFiles, []string{"main.txt", newFile}) {
		t.Fatalf("changes = %#v", evidence.ChangedFiles)
	}
	if len(evidence.PreservationViolations) != 0 || strings.Contains(evidence.Diff, "user notes") {
		t.Fatalf("user work incorrectly attributed: %#v", evidence)
	}
	for _, want := range []string{"-original", "+implemented", "+new contents"} {
		if !strings.Contains(evidence.Diff, want) {
			t.Fatalf("diff missing %q: %s", want, evidence.Diff)
		}
	}
	if evidence.Baseline.Fingerprint == evidence.Current.Fingerprint {
		t.Fatal("fingerprint unchanged")
	}
}

func TestProtectedChangesRetainOriginalsWithoutDestructiveRollback(t *testing.T) {
	dir := repository(t)
	put(t, dir, "notes.txt", "valuable user work\n")
	session := acquire(t, newWorkspace(t, Config{}), dir)
	put(t, dir, "notes.txt", "agent overwrote it\n")
	evidence, err := session.Inspect(t.Context())
	cleanupRecovery(t, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(evidence.PreservationViolations, []string{"notes.txt"}) {
		t.Fatalf("violations = %v", evidence.PreservationViolations)
	}
	backup, err := os.ReadFile(filepath.Join(evidence.RecoveryDirectory, "files", "notes.txt"))
	if err != nil || string(backup) != "valuable user work\n" {
		t.Fatalf("recovery: %q %v", backup, err)
	}
	actual, _ := os.ReadFile(filepath.Join(dir, "notes.txt"))
	if string(actual) != "agent overwrote it\n" {
		t.Fatal("adapter performed an unsafe rollback")
	}
}

func TestChangesIncludeDeletionsBinaryModesAndSymlinks(t *testing.T) {
	dir := repository(t)
	if err := os.Symlink("/outside/target", filepath.Join(dir, "link")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("skipping symlink test on Windows without privilege")
		}
		t.Fatal(err)
	}
	runGit(t, dir, "add", "link")
	runGit(t, dir, "commit", "-qm", "link")
	session := acquire(t, newWorkspace(t, Config{}), dir)
	if err := os.Remove(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "main.txt"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/different/target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "binary.dat", "\x00\x01\x02")
	evidence, err := session.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(evidence.ChangedFiles, []string{"binary.dat", "link", "main.txt", "notes.txt"}) {
		t.Fatalf("changes: %v", evidence.ChangedFiles)
	}
	for _, want := range []string{"GIT binary patch", "old mode 100644", "new mode 100755", "-/outside/target", "+/different/target", "deleted file mode"} {
		if !strings.Contains(evidence.Diff, want) {
			t.Fatalf("missing %q: %s", want, evidence.Diff)
		}
	}
}

func TestUnbornRepositoryAndIgnoredFiles(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	put(t, dir, ".gitignore", "build/\n")
	session := acquire(t, newWorkspace(t, Config{}), dir)
	if session.Baseline().Baseline.Head != "" {
		t.Fatal("unborn repository unexpectedly has HEAD")
	}
	put(t, dir, "build/artifact", "ignored")
	put(t, dir, "source.txt", "source")
	evidence, err := session.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(evidence.ChangedFiles, []string{"source.txt"}) {
		t.Fatalf("changes: %v", evidence.ChangedFiles)
	}
}

func TestIndexAndHeadChangesCannotBeApproved(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "index", true: "HEAD"}[commit], func(t *testing.T) {
			dir := repository(t)
			session := acquire(t, newWorkspace(t, Config{}), dir)
			put(t, dir, "main.txt", "updated\n")
			runGit(t, dir, "add", "main.txt")
			if commit {
				runGit(t, dir, "commit", "-qm", "unauthorized")
			}
			evidence, err := session.Inspect(t.Context())
			cleanupRecovery(t, evidence)
			if err != nil {
				t.Fatal(err)
			}
			if len(evidence.PreservationViolations) == 0 {
				t.Fatal("Git mutation was not detected")
			}
		})
	}
}

func TestUnsupportedAndOversizedWorkspacesFailBeforeAgents(t *testing.T) {
	workspace := newWorkspace(t, Config{})
	if err := workspace.Validate(t.Context(), t.TempDir()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("non-Git: %v", err)
	}
	dir := repository(t)
	put(t, dir, "sub/file", "text")
	if err := workspace.Validate(t.Context(), filepath.Join(dir, "sub")); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("subdirectory: %v", err)
	}
	if _, err := newWorkspace(t, Config{MaxFileBytes: 2}).Acquire(t.Context(), dir); err == nil {
		t.Fatal("oversized file was accepted")
	}
	if _, err := newWorkspace(t, Config{MaxFiles: 1}).Acquire(t.Context(), dir); err == nil {
		t.Fatal("too many files accepted")
	}
	runGit(t, dir, "update-index", "--assume-unchanged", "main.txt")
	if _, err := workspace.Acquire(t.Context(), dir); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("assume-unchanged: %v", err)
	}
}

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

func TestGitRoutingEnvironmentCannotSelectAnotherIndex(t *testing.T) {
	dir := repository(t)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "wrong-index"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	t.Setenv("GIT_DIR", t.TempDir())
	session := acquire(t, newWorkspace(t, Config{}), dir)
	if len(session.Baseline().PreExistingFiles) != 0 {
		t.Fatal("inherited Git environment redirected capture")
	}
}

func TestNestedRepositoriesSubmodulesAndSparseEntriesAreRejected(t *testing.T) {
	for _, kind := range []string{"nested", "submodule", "skip-worktree"} {
		t.Run(kind, func(t *testing.T) {
			dir := repository(t)
			switch kind {
			case "nested":
				put(t, dir, "nested/file", "content")
				runGit(t, filepath.Join(dir, "nested"), "init", "-q")
			case "submodule":
				head := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
				runGit(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+head+",module")
			case "skip-worktree":
				runGit(t, dir, "update-index", "--skip-worktree", "main.txt")
			}
			if lease, err := newWorkspace(t, Config{}).Acquire(t.Context(), dir); !errors.Is(err, ErrUnsupported) {
				if lease != nil {
					_ = lease.Close()
				}
				t.Fatalf("%s: %v", kind, err)
			}
		})
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

func TestStagedDeletionCannotBeRecreatedBehindIgnoreRule(t *testing.T) {
	dir := repository(t)
	runGit(t, dir, "rm", "notes.txt")
	session := acquire(t, newWorkspace(t, Config{}), dir)
	put(t, dir, ".gitignore", "notes.txt\n")
	put(t, dir, "notes.txt", "recreated despite user's deletion\n")
	evidence, err := session.Inspect(t.Context())
	cleanupRecovery(t, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(evidence.PreservationViolations, []string{"notes.txt"}) {
		t.Fatalf("violations = %v", evidence.PreservationViolations)
	}
	manifest, err := os.ReadFile(filepath.Join(evidence.RecoveryDirectory, "manifest.json"))
	if err != nil || !strings.Contains(string(manifest), "\"missing_files\": [\n    \"notes.txt\"") {
		t.Fatalf("missing deletion recovery evidence: %s; %v", manifest, err)
	}
}
