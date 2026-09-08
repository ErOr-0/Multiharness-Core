package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/store"
)

func TestFolderWorkflowIntegration(t *testing.T) {
	for _, mode := range []string{"plain repair", "multiple repositories repair", "plain answer"} {
		t.Run(mode, func(t *testing.T) {
			cfg, log := fixtureConfiguration(t)
			root := t.TempDir()
			want := []string{"result.txt"}
			if mode == "multiple repositories repair" {
				second, _ := fixtureConfiguration(t)
				for name, source := range map[string]string{"api": cfg.WorkingDir, "web": second.WorkingDir} {
					if err := os.Rename(source, filepath.Join(root, name)); err != nil {
						t.Fatal(err)
					}
				}
				t.Setenv("MULTIHARNESS_FIXTURE_LOG", log)
				t.Setenv("MULTIHARNESS_FIXTURE_MULTIPROJECT", "1")
				want = []string{"api/result.txt", "web/result.txt"}
			} else {
				if err := os.WriteFile(filepath.Join(root, "result.txt"), []byte("before\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cfg.WorkingDir = root
			cfg.MaxRepairAttempts = 1
			runner, err := buildWorkflow(cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			task, status := "fixture change", store.TaskStatusApproved
			if mode == "plain answer" {
				task, status = "fixture answer", store.TaskStatusAnswered
			}
			result := runner.Run(t.Context(), store.TaskInput{Task: task, WorkingDir: root, MaxRepairAttempts: 1})
			if result.Status != status {
				t.Fatalf("result: %#v", result)
			}
			if status == store.TaskStatusApproved {
				if !slices.Equal(result.Repository.ChangedFiles, want) || !strings.Contains(result.Repository.Diff, "-before") || !strings.Contains(result.Repository.Diff, "+fixed") {
					t.Fatalf("lost combined baseline evidence: %#v", result.Repository)
				}
				calls, _ := os.ReadFile(log)
				if string(calls) != "plan\nimplement\ncheck\nreview\nrepair\ncheck\nreview\n" {
					t.Fatalf("repair loop: %s", calls)
				}
			}
			if mode == "multiple repositories repair" {
				for _, name := range []string{"api", "web"} {
					notes, err := os.ReadFile(filepath.Join(root, name, "notes.txt"))
					if err != nil || string(notes) != "user notes\n" {
						t.Fatal("lost user changes", err)
					}
				}
			}
			if _, err := os.Lstat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
				t.Fatal("workflow created a Git repository in the selected folder", err)
			}
		})
	}
}
