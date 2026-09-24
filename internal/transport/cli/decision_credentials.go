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
	override       bool
	setupTransport http.RoundTripper
}

func (c *DecisionCredentials) currentKey() string {
	if c.override && c.key != "" {
		return c.key
	}
	if c.Getenv != nil {
		if key := strings.TrimSpace(c.Getenv("OPENROUTER_API_KEY")); key != "" {
			return key
		}
	}
	return c.key
}

func (c *DecisionCredentials) Resolve(ctx context.Context, events workflow.EventSink) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if key := c.currentKey(); key != "" {
		return key, nil
	}
	return c.promptKey(ctx, events, false)
}

// Replace always asks for a hidden key and uses it for this session, even when
// an environment key is configured. A cancelled entry keeps the current key.
func (c *DecisionCredentials) Replace(ctx context.Context) error {
	_, err := c.promptKey(ctx, nil, true)
	return err
}

func (c *DecisionCredentials) promptKey(ctx context.Context, events workflow.EventSink, override bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
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
	c.override = override
	return key, nil
}
