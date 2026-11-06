package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	maxStatusLines = 200
	maxDiffStat    = 8 << 10
	gitTimeout     = 20 * time.Second
)

// GitEvidence is what Git reports about the workspace after the run, so the
// reviewer does not have to rely on the inner agent's own account. Status is
// null when there is no Git evidence and empty when the tree is clean.
type GitEvidence struct {
	DirtyBefore int      `json:"dirty_before"`
	Status      []string `json:"status"`
	DiffStat    string   `json:"diff_stat,omitempty"`
	Note        string   `json:"note,omitempty"`
}

func gitStatus(ctx context.Context, dir string) ([]string, error) {
	return gitLines(ctx, dir, "status", "--porcelain=v1")
}

func gitEvidence(dir string, before []string, beforeErr error) *GitEvidence {
	if beforeErr != nil {
		return &GitEvidence{Note: "no Git evidence: " + gitProblem(beforeErr)}
	}
	ctx := context.Background()
	after, err := gitStatus(ctx, dir)
	if err != nil {
		return &GitEvidence{Note: "no Git evidence: " + gitProblem(err)}
	}
	ev := &GitEvidence{DirtyBefore: len(before), Status: append([]string{}, after[:min(len(after), maxStatusLines)]...)}
	var notes []string
	if len(before) > 0 {
		notes = append(notes, fmt.Sprintf("%d paths were already uncommitted before this run; status and diff_stat include them", len(before)))
	}
	if len(after) > maxStatusLines {
		notes = append(notes, fmt.Sprintf("status lists the first %d of %d paths", maxStatusLines, len(after)))
	}
	stat, err := gitLines(ctx, dir, "diff", "HEAD", "--stat")
	if err != nil {
		stat, _ = gitLines(ctx, dir, "diff", "--stat") // a repository without commits
	}
	ev.DiffStat = clip(strings.Join(stat, "\n"), maxDiffStat)
	ev.Note = strings.Join(notes, "; ")
	return ev
}

func gitLines(ctx context.Context, dir string, args ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-c", "core.quotepath=false", "-C", dir}, args...)...).Output()
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func gitProblem(err error) string {
	if errors.Is(err, exec.ErrNotFound) {
		return "git is not installed"
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if line, _, _ := strings.Cut(strings.TrimSpace(string(exit.Stderr)), "\n"); line != "" {
			return strings.TrimPrefix(line, "fatal: ")
		}
	}
	return err.Error()
}
