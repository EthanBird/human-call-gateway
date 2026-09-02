package adapter

import (
	"context"
	"fmt"
	"os"
	"regexp"
)

// WebhookConfig represents generic webhook channel configuration
type WebhookConfig struct {
	URL         string
	URLEnv      string
	Method      string
	Headers     map[string]string
	ContentType string
}

func (c *WebhookConfig) GetType() string {
	return "webhook"
}

// WebhookAdapter sends messages to generic webhooks
type WebhookAdapter struct {
	httpHelper *HTTPHelper
}

// NewWebhookAdapter creates a new webhook adapter
func NewWebhookAdapter(httpHelper *HTTPHelper) *WebhookAdapter {
	return &WebhookAdapter{httpHelper: httpHelper}
}

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Send sends a message to a webhook
func (a *WebhookAdapter) Send(ctx context.Context, channelConfig ChannelConfig, payload HumanCallPayload) (SendResult, error) {
	config, ok := channelConfig.(*WebhookConfig)
	if !ok {
		return SendResult{}, NewConfigMissingError("invalid Webhook configuration")
	}

	url := config.URL
	if url == "" && config.URLEnv != "" {
		url = os.Getenv(config.URLEnv)
		if url == "" {
			return SendResult{}, NewConfigMissingError(fmt.Sprintf("Environment variable %s required for Webhook channel is not set", config.URLEnv))
		}
	}
	if url == "" {
		return SendResult{}, NewConfigMissingError("Webhook URL is not configured")
	}

	body := map[string]interface{}{
		"need":     payload.Need,
		"blocker":  payload.Blocker,
		"action":   payload.Action,
		"fallback": payload.Fallback,
		"urgent":   payload.Urgent,
	}
	if payload.ChannelID != "" {
		body["channel_id"] = payload.ChannelID
	}

	headers := make(map[string]string)
	for k, v := range config.Headers {
		headers[k] = a.substituteEnvVars(v)
	}

	if config.ContentType != "" {
		headers["Content-Type"] = config.ContentType
	}

	_, statusCode, err := a.httpHelper.PostJSON(ctx, url, headers, body)
	if err != nil {
		if IsTimeoutError(err) {
			return SendResult{}, err
		}
		return SendResult{}, NewTransportError("Webhook request failed", nil)
	}

	if statusCode < 200 || statusCode >= 300 {
		return SendResult{}, NewTransportError(fmt.Sprintf("Webhook returned %d", statusCode), nil)
	}

	return SendResult{
		Sent: true,
		ID:   fmt.Sprintf("gw-webhook-%d", generateTimestampID()),
	}, nil
}

func (a *WebhookAdapter) substituteEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return match
	})
}
