package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// SlackConfig represents Slack channel configuration
type SlackConfig struct {
	WebhookURLEnv string
}

func (c *SlackConfig) GetType() string {
	return "slack"
}

// SlackAdapter sends messages to Slack via Incoming Webhooks
type SlackAdapter struct {
	httpHelper *HTTPHelper
}

// NewSlackAdapter creates a new Slack adapter
func NewSlackAdapter(httpHelper *HTTPHelper) *SlackAdapter {
	return &SlackAdapter{httpHelper: httpHelper}
}

// Send sends a message to Slack
func (a *SlackAdapter) Send(ctx context.Context, channelConfig ChannelConfig, payload HumanCallPayload) (SendResult, error) {
	config, ok := channelConfig.(*SlackConfig)
	if !ok {
		return SendResult{}, NewConfigMissingError("invalid Slack configuration")
	}

	webhookURL := os.Getenv(config.WebhookURLEnv)
	if webhookURL == "" {
		return SendResult{}, NewConfigMissingError(fmt.Sprintf("Environment variable %s required for Slack channel is not set", config.WebhookURLEnv))
	}

	text := a.formatMessage(payload)
	body := map[string]interface{}{
		"text":   text,
		"urgent": payload.Urgent,
	}

	respBody, statusCode, err := a.httpHelper.PostJSON(ctx, webhookURL, nil, body)
	if err != nil {
		if IsTimeoutError(err) {
			return SendResult{}, err
		}
		return SendResult{}, NewTransportError("Slack webhook request failed", nil)
	}

	if statusCode < 200 || statusCode >= 300 {
		return SendResult{}, NewTransportError(fmt.Sprintf("Slack API returned %d", statusCode), nil)
	}

	// Slack webhook returns "ok" on success, but doesn't provide a message ID
	// Generate a gateway ID
	id := fmt.Sprintf("gw-slack-%d", generateTimestampID())

	// Try to extract timestamp from response if available
	var slackResp struct {
		Ok bool   `json:"ok"`
		Ts string `json:"ts"`
	}
	if err := json.Unmarshal(respBody, &slackResp); err == nil && slackResp.Ts != "" {
		id = slackResp.Ts
	}

	return SendResult{
		Sent: true,
		ID:   id,
	}, nil
}

func (a *SlackAdapter) formatMessage(payload HumanCallPayload) string {
	var sb strings.Builder
	sb.WriteString("🔔 *Human Call Request*\n\n")
	sb.WriteString(fmt.Sprintf("*需要人:* %s\n", payload.Need))
	sb.WriteString(fmt.Sprintf("*卡在:* %s\n", payload.Blocker))
	sb.WriteString(fmt.Sprintf("*请你:* %s\n", payload.Action))
	sb.WriteString(fmt.Sprintf("*不做也能:* %s\n", payload.Fallback))
	if payload.Urgent {
		sb.WriteString("\n⚠️ *URGENT*")
	}
	return sb.String()
}
