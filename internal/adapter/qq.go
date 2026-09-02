package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// QQConfig represents QQ (OneBot) channel configuration
type QQConfig struct {
	HTTPAPIUrl      string
	AccessTokenEnv  string
	UserID          string
	GroupID         string
}

func (c *QQConfig) GetType() string {
	return "qq"
}

// QQAdapter sends messages to QQ via OneBot HTTP API
type QQAdapter struct {
	httpHelper *HTTPHelper
}

// NewQQAdapter creates a new QQ adapter
func NewQQAdapter(httpHelper *HTTPHelper) *QQAdapter {
	return &QQAdapter{httpHelper: httpHelper}
}

// Send sends a message to QQ
func (a *QQAdapter) Send(ctx context.Context, channelConfig ChannelConfig, payload HumanCallPayload) (SendResult, error) {
	config, ok := channelConfig.(*QQConfig)
	if !ok {
		return SendResult{}, NewConfigMissingError("invalid QQ configuration")
	}

	headers := make(map[string]string)
	if config.AccessTokenEnv != "" {
		accessToken := os.Getenv(config.AccessTokenEnv)
		if accessToken == "" {
			return SendResult{}, NewConfigMissingError(fmt.Sprintf("Environment variable %s required for QQ channel is not set", config.AccessTokenEnv))
		}
		headers["Authorization"] = fmt.Sprintf("Bearer %s", accessToken)
	}

	message := a.formatMessage(payload)
	body := map[string]interface{}{
		"message": message,
		"urgent":  payload.Urgent,
	}

	if config.UserID != "" {
		body["message_type"] = "private"
		body["user_id"] = config.UserID
	} else if config.GroupID != "" {
		body["message_type"] = "group"
		body["group_id"] = config.GroupID
	} else {
		return SendResult{}, NewConfigMissingError("QQ channel requires either user_id or group_id")
	}

	respBody, statusCode, err := a.httpHelper.PostJSON(ctx, config.HTTPAPIUrl, headers, body)
	if err != nil {
		if IsTimeoutError(err) {
			return SendResult{}, err
		}
		return SendResult{}, NewTransportError(fmt.Sprintf("QQ API request failed: %v", err), err)
	}

	if statusCode < 200 || statusCode >= 300 {
		return SendResult{}, NewTransportError(fmt.Sprintf("QQ API returned %d: %s", statusCode, string(respBody)), nil)
	}

	var qqResp struct {
		Status  string `json:"status"`
		RetCode int    `json:"retcode"`
		Data    struct {
			MessageID int `json:"message_id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &qqResp); err != nil {
		return SendResult{}, NewTransportError("Failed to parse QQ response", err)
	}

	if qqResp.Status != "ok" || qqResp.RetCode != 0 {
		return SendResult{}, NewTransportError(fmt.Sprintf("QQ API returned status=%s retcode=%d: %s", qqResp.Status, qqResp.RetCode, string(respBody)), nil)
	}

	return SendResult{
		Sent: true,
		ID:   fmt.Sprintf("%d", qqResp.Data.MessageID),
	}, nil
}

func (a *QQAdapter) formatMessage(payload HumanCallPayload) string {
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
