package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"multiharness-core/internal/adapter/account"
	decisionadapter "multiharness-core/internal/adapter/decision/openrouter"
	"multiharness-core/internal/config"
)

func (c *DecisionCredentials) setupClient() *http.Client {
	// Never follow a redirect: a credential must not travel to another origin.
	return &http.Client{Timeout: 10 * time.Second, Transport: c.setupTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// CheckSetup verifies the configured decision provider. Jev keys are validated
// with OpenRouter's unbilled key endpoint; a self-hosted Laya server answers a
// small probe request. Neither check follows redirects or sends a credential to
// an origin other than the configured one.
func (c *DecisionCredentials) CheckSetup(ctx context.Context, cfg config.Config, prompt bool) account.Status {
	decision := cfg.Decision.Effective()
	key := c.currentKey(decision)
	if decision.Provider == decisionadapter.ProviderLaya {
		return c.checkLaya(ctx, decision, key)
	}
	if key == "" && prompt {
		value, err := c.Resolve(ctx, decision, nil)
		if err != nil {
			return account.Status{Detail: "OpenRouter key missing; use /login jev or disable Jev with /set decision-enabled false"}
		}
		key = value
	}
	if key == "" {
		return account.Status{Detail: "OpenRouter key missing; use /login jev (hidden input) or /set decision-enabled false to skip Jev"}
	}
	endpoint, err := url.Parse(decision.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host != "openrouter.ai" || endpoint.User != nil {
		return account.Status{Ready: true, Detail: "Key configured for custom Jev endpoint; remote authentication is not verified"}
	}
	client := c.setupClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/key", nil)
	if err != nil {
		return account.Status{Detail: "Cannot create OpenRouter authentication check"}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	response, err := client.Do(req)
	if err != nil {
		return account.Status{Detail: "OpenRouter authentication check could not connect; retry /configuration"}
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		c.forget()
		return account.Status{Detail: "OpenRouter rejected the key; replace OPENROUTER_API_KEY if set, otherwise use /login jev"}
	}
	if response.StatusCode != 200 {
		return account.Status{Detail: "OpenRouter authentication check unavailable; retry /configuration"}
	}
	var result struct {
		Data *struct {
			LimitRemaining *float64 `json:"limit_remaining"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result) != nil || result.Data == nil {
		return account.Status{Detail: "OpenRouter returned an unrecognized key status; retry /configuration"}
	}
	if result.Data.LimitRemaining != nil && *result.Data.LimitRemaining <= 0 {
		return account.Status{Detail: "OpenRouter key spending limit exhausted; update its limit or use another key"}
	}
	return account.Status{Ready: true, Detail: "OpenRouter key accepted; model access and account credits can still limit requests"}
}

// checkLaya sends one tiny System One request to the self-hosted server. It
// proves the container is reachable, accepts the configured model and key, and
// answers in the expected shape. The probe carries no project content.
func (c *DecisionCredentials) checkLaya(ctx context.Context, decision config.Decision, key string) account.Status {
	endpoint, err := url.Parse(decision.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return account.Status{Detail: "Laya endpoint must be an http(s) URL without embedded credentials; use /set decision-endpoint"}
	}
	body, err := json.Marshal(map[string]any{
		"model": decision.Model,
		"state": "Readiness probe from Multiharness.",
		"questions": map[string]any{
			"probe": map[string]any{
				"type":         "choice",
				"instructions": "Confirm the server answers typed questions.",
				"criteria":     map[string]any{"ready": "The server is answering.", "unready": "Never choose this."},
			},
		},
	})
	if err != nil {
		return account.Status{Detail: "Cannot build the Laya readiness probe"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, decision.Endpoint, bytes.NewReader(body))
	if err != nil {
		return account.Status{Detail: "Cannot create the Laya readiness probe"}
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := c.setupClient().Do(req)
	if err != nil {
		return account.Status{Detail: fmt.Sprintf("Laya server at %s is unreachable; start the container or fix decision-endpoint, then retry /configuration", endpoint.Host)}
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == 401 || response.StatusCode == 403:
		c.forget()
		if key == "" {
			return account.Status{Detail: "Laya server requires a key; set LAYA_API_KEY or use /login laya (hidden input)"}
		}
		return account.Status{Detail: "Laya server rejected the key; replace LAYA_API_KEY if set, otherwise use /login laya"}
	case response.StatusCode == 404 || response.StatusCode == 405:
		return account.Status{Detail: fmt.Sprintf("Laya server has no System One endpoint at %s; check the decision-endpoint path (usually /v1/systemone)", endpoint.Path)}
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return account.Status{Detail: fmt.Sprintf("Laya server returned HTTP %d; check decision-model (%s) and the container logs", response.StatusCode, decision.Model)}
	}
	var result struct {
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result) != nil || result.Answers["probe"] == nil {
		return account.Status{Detail: "Laya server answered without System One answers; check decision-endpoint and decision-model"}
	}
	detail := "Laya server answered at " + endpoint.Host
	if key == "" {
		detail += " without authentication"
	}
	if endpoint.Scheme == "http" && !isLoopback(endpoint.Hostname()) {
		detail += "; plain http to a remote host sends requests unencrypted"
	}
	return account.Status{Ready: true, Detail: detail}
}

func isLoopback(host string) bool {
	host = strings.ToLower(host)
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".localhost")
}
