package gittrust

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isolatedGit(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return home
}

func TestSelectedRepositoryTrustIsExactPersistentAndIdempotent(t *testing.T) {
	home := isolatedGit(t)
	root := t.TempDir()
	repo := filepath.Join(root, "project with spaces")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	if err := Prepare(t.Context(), root, filepath.Join(repo, "src")); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(t.Context(), root, repo); err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command("git", "config", "--global", "--get-all", "safe.directory").Output()
	canonical, _ := filepath.EvalSymlinks(repo)
	if err != nil || strings.TrimSpace(string(got)) != filepath.ToSlash(canonical) {
		t.Fatalf("trust widened or duplicated: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".gitconfig")); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "status", "--porcelain").CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("Git access: %v %s", err, out)
	}
}

func TestGitTrustDoesNotScanOrTrustOtherProjects(t *testing.T) {
	home := isolatedGit(t)
	root, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "child", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(t.Context(), root, root); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(t.Context(), "", outside); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(t.Context(), root, outside); err == nil {
		t.Fatal("outside folder trusted")
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
			t.Fatal(err)
		}
		if err := Prepare(t.Context(), root, filepath.Join(root, "escape")); err == nil {
			t.Fatal("symlink escape trusted")
		}
		star := filepath.Join(root, "*")
		if err := os.MkdirAll(filepath.Join(star, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := Prepare(t.Context(), root, star); err == nil {
			t.Fatal("wildcard trusted")
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatal("unselected project changed global configuration", err)
	}
}

func TestGitTrustFailureIsReported(t *testing.T) {
	home := isolatedGit(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(t.Context(), root, root); err == nil {
		t.Fatal("invalid global config was ignored")
	}
}
