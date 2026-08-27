package ai

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func Run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	outputBytes, err := cmd.CombinedOutput()
	outputStr := string(outputBytes)

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command %s timed out after 5 minutes", name)
	}

	if err != nil {
		return outputStr, fmt.Errorf("command %s failed: %w (output: %s)", name, err, strings.TrimSpace(outputStr))
	}

	return outputStr, nil

}
