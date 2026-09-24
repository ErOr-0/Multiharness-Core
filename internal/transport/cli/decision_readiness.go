package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
)

// CheckSetup validates the key using the unbilled key endpoint. Never follows a
// redirect or sends a custom-endpoint credential to another origin.
func (c *DecisionCredentials) CheckSetup(ctx context.Context, cfg config.Config, prompt bool) account.Status {
	key := c.currentKey()
	if key == "" && prompt {
		value, err := c.Resolve(ctx, nil)
		if err != nil {
			return account.Status{Detail: "OpenRouter key missing; use /login jev or disable Jev with /set decision-enabled false"}
		}
		key = value
	}
	if key == "" {
		return account.Status{Detail: "OpenRouter key missing; use /login jev (hidden input) or /set decision-enabled false to skip Jev"}
	}
	endpoint, err := url.Parse(cfg.Decision.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host != "openrouter.ai" || endpoint.User != nil {
		return account.Status{Ready: true, Detail: "Key configured for custom Jev endpoint; remote authentication is not verified"}
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: c.setupTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
		c.key = ""
		c.override = false
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
