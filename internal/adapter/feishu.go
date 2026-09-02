package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FeishuConfig represents Feishu/Lark channel configuration
type FeishuConfig struct {
	WebhookURLEnv string
}

func (c *FeishuConfig) GetType() string {
	return "feishu"
}

// FeishuAdapter sends messages to Feishu via webhooks
type FeishuAdapter struct {
	httpHelper *HTTPHelper
}

// NewFeishuAdapter creates a new Feishu adapter
func NewFeishuAdapter(httpHelper *HTTPHelper) *FeishuAdapter {
	return &FeishuAdapter{httpHelper: httpHelper}
}

// Send sends a message to Feishu
func (a *FeishuAdapter) Send(ctx context.Context, channelConfig ChannelConfig, payload HumanCallPayload) (SendResult, error) {
	config, ok := channelConfig.(*FeishuConfig)
	if !ok {
		return SendResult{}, NewConfigMissingError("invalid Feishu configuration")
	}

	webhookURL := os.Getenv(config.WebhookURLEnv)
	if webhookURL == "" {
		return SendResult{}, NewConfigMissingError(fmt.Sprintf("Environment variable %s required for Feishu channel is not set", config.WebhookURLEnv))
	}

	text := a.formatMessage(payload)
	body := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]interface{}{
			"text": text,
		},
		"urgent": payload.Urgent,
	}

	respBody, statusCode, err := a.httpHelper.PostJSON(ctx, webhookURL, nil, body)
	if err != nil {
		if IsTimeoutError(err) {
			return SendResult{}, err
		}
		return SendResult{}, NewTransportError(fmt.Sprintf("Feishu webhook request failed: %v", err), err)
	}

	if statusCode < 200 || statusCode >= 300 {
		return SendResult{}, NewTransportError(fmt.Sprintf("Feishu API returned %d: %s", statusCode, string(respBody)), nil)
	}

	var feishuResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(respBody, &feishuResp); err != nil {
		// If we can't parse response but got 2xx, consider it success
		return SendResult{
			Sent: true,
			ID:   fmt.Sprintf("gw-feishu-%d", generateTimestampID()),
		}, nil
	}

	if feishuResp.Code != 0 {
		return SendResult{}, NewTransportError(fmt.Sprintf("Feishu API returned code=%d: %s", feishuResp.Code, feishuResp.Msg), nil)
	}

	return SendResult{
		Sent: true,
		ID:   fmt.Sprintf("gw-feishu-%d", generateTimestampID()),
	}, nil
}

func (a *FeishuAdapter) formatMessage(payload HumanCallPayload) string {
	var sb strings.Builder
	sb.WriteString("🔔 Human Call Request\n\n")
	sb.WriteString(fmt.Sprintf("需要人: %s\n", payload.Need))
	sb.WriteString(fmt.Sprintf("卡在: %s\n", payload.Blocker))
	sb.WriteString(fmt.Sprintf("请你: %s\n", payload.Action))
	sb.WriteString(fmt.Sprintf("不做也能: %s\n", payload.Fallback))
	if payload.Urgent {
		sb.WriteString("\n⚠️ URGENT")
	}
	return sb.String()
}
