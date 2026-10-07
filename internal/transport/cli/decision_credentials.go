package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"multiharness-core/internal/config"
	"multiharness-core/internal/workflow"
)

// DecisionCredentials holds a prompted key only for this app session. It never
// writes configuration or exports the key to native agent subprocesses. The
// key belongs to one provider: a Jev (OpenRouter) key entered earlier is never
// sent to a Laya server, and vice versa.
type DecisionCredentials struct {
	Getenv         func(string) string
	Prompt         func(context.Context) (string, error)
	key            string
	keyProvider    string
	override       bool
	setupTransport http.RoundTripper
}

func (c *DecisionCredentials) sessionKey(decision config.Decision) string {
	// A key without a recorded provider predates provider tracking and belongs
	// to the default (Jev) flow.
	if c.keyProvider != "" && c.keyProvider != decision.Effective().Provider {
		return ""
	}
	return c.key
}

func (c *DecisionCredentials) currentKey(decision config.Decision) string {
	if key := c.sessionKey(decision); c.override && key != "" {
		return key
	}
	if c.Getenv != nil {
		if key := strings.TrimSpace(c.Getenv(decision.KeyVariable())); key != "" {
			return key
		}
	}
	return c.sessionKey(decision)
}

// forget drops a session key the provider rejected.
func (c *DecisionCredentials) forget() {
	c.key, c.keyProvider, c.override = "", "", false
}

// Resolve returns the key for the configured provider, asking for it only when
// the provider cannot run without one. A self-hosted Laya server with no key
// configured is used unauthenticated.
func (c *DecisionCredentials) Resolve(ctx context.Context, decision config.Decision, events workflow.EventSink) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if key := c.currentKey(decision); key != "" {
		return key, nil
	}
	if !decision.RequiresKey() {
		return "", nil
	}
	return c.promptKey(ctx, decision, events, false)
}

// Replace always asks for a hidden key and uses it for this session, even when
// an environment key is configured. A cancelled entry keeps the current key.
func (c *DecisionCredentials) Replace(ctx context.Context, decision config.Decision) error {
	_, err := c.promptKey(ctx, decision, nil, true)
	return err
}

func (c *DecisionCredentials) promptKey(ctx context.Context, decision config.Decision, events workflow.EventSink, override bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name, variable := decision.ProviderName(), decision.KeyVariable()
	if c.Prompt == nil {
		return "", fmt.Errorf("%s is enabled: set %s or use an interactive terminal to enter it", name, variable)
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
		return "", fmt.Errorf("cannot read %s API key; set %s or retry in an interactive terminal", name, variable)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("%s API key not provided; configure it or disable decision-enabled before retrying", name)
	}
	if strings.ContainsAny(key, "\r\n\x00") {
		return "", fmt.Errorf("invalid %s API key input", name)
	}
	c.key = key
	c.keyProvider = decision.Effective().Provider
	c.override = override
	return key, nil
}
