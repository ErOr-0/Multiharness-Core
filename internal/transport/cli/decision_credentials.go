package cli

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"multiharness-core/internal/workflow"
)

// DecisionCredentials holds a prompted key only for this app session. It never
// writes configuration or exports the key to native agent subprocesses.
type DecisionCredentials struct {
	Getenv         func(string) string
	Prompt         func(context.Context) (string, error)
	key            string
	setupTransport http.RoundTripper
}

func (c *DecisionCredentials) Resolve(ctx context.Context, events workflow.EventSink) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if c.Getenv != nil {
		if key := strings.TrimSpace(c.Getenv("OPENROUTER_API_KEY")); key != "" {
			return key, nil
		}
	}
	if c.key != "" {
		return c.key, nil
	}
	if c.Prompt == nil {
		return "", errors.New("Jev is enabled: set OPENROUTER_API_KEY or use an interactive terminal to enter it")
	}
	if p, ok := events.(interface{ PauseProgress() (func(), error) }); ok {
		resume, err := p.PauseProgress()
		if resume != nil {
			defer resume()
		}
		if err != nil {
			return "", errors.New("cannot pause progress for API key input")
		}
	}
	key, err := c.Prompt(ctx)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", errors.New("cannot read OpenRouter API key; set OPENROUTER_API_KEY or retry in an interactive terminal")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("OpenRouter API key not provided; configure it or disable decision-enabled before retrying")
	}
	if strings.ContainsAny(key, "\r\n\x00") {
		return "", errors.New("invalid OpenRouter API key input")
	}
	c.key = key
	return key, nil
}
