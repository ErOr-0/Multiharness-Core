//go:build linux

package gittrust

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestForeignOwnedRepositoryAccess(t *testing.T) {
	if root := os.Getenv("MAGENT_GIT_TRUST_TEST_ROOT"); root != "" {
		for _, name := range []string{"selected", "other"} {
			out, err := exec.Command("git", "-C", filepath.Join(root, name), "status", "--porcelain").CombinedOutput()
			if err == nil || !strings.Contains(string(out), "dubious ownership") {
				t.Fatalf("missing ownership reproduction: %s %v", out, err)
			}
		}
		if err := Prepare(t.Context(), root, filepath.Join(root, "selected", "src")); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("git", "-C", filepath.Join(root, "selected"), "status", "--porcelain").CombinedOutput()
		if err != nil {
			t.Fatalf("selected repository remains blocked: %s %v", out, err)
		}
		out, err = exec.Command("git", "-C", filepath.Join(root, "other"), "status", "--porcelain").CombinedOutput()
		if err == nil || !strings.Contains(string(out), "dubious ownership") {
			t.Fatalf("unselected repository trust widened: %s %v", out, err)
		}
		return
	}
	if os.Getuid() != 0 {
		t.Skip("root needed to create foreign-owned repository fixture")
	}
	if _, err := exec.LookPath("setpriv"); err != nil {
		t.Skip("setpriv unavailable")
	}
	root, err := os.MkdirTemp("", "magent-git-ownership-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"selected", "other"} {
		repo := filepath.Join(root, name)
		if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
		if err := os.Mkdir(filepath.Join(repo, "src"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	home := filepath.Join(root, "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(home, 1000, 1000); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "probe")
	if err := os.WriteFile(child, data, 0755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("setpriv", "--reuid=1000", "--regid=1000", "--clear-groups", child, "-test.run=^TestForeignOwnedRepositoryAccess$", "-test.v")
	command.Env = append(os.Environ(), "MAGENT_GIT_TRUST_TEST_ROOT="+root, "HOME="+home, "GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig"), "GIT_CONFIG_NOSYSTEM=1")
	command.Dir = root
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("ownership test: %v\n%s", err, out)
	}
}
