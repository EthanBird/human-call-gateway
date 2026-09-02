package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// TelegramConfig represents Telegram channel configuration
type TelegramConfig struct {
	BotTokenEnv string
	ChatID      string
	APIBaseURL  string
}

func (c *TelegramConfig) GetType() string {
	return "telegram"
}

// TelegramAdapter sends messages to Telegram via Bot API
type TelegramAdapter struct {
	httpHelper *HTTPHelper
}

// NewTelegramAdapter creates a new Telegram adapter
func NewTelegramAdapter(httpHelper *HTTPHelper) *TelegramAdapter {
	return &TelegramAdapter{httpHelper: httpHelper}
}

// Send sends a message to Telegram
func (a *TelegramAdapter) Send(ctx context.Context, channelConfig ChannelConfig, payload HumanCallPayload) (SendResult, error) {
	config, ok := channelConfig.(*TelegramConfig)
	if !ok {
		return SendResult{}, NewConfigMissingError("invalid Telegram configuration")
	}

	botToken := os.Getenv(config.BotTokenEnv)
	if botToken == "" {
		return SendResult{}, NewConfigMissingError(fmt.Sprintf("Environment variable %s required for Telegram channel is not set", config.BotTokenEnv))
	}

	apiBaseURL := config.APIBaseURL
	if apiBaseURL == "" {
		apiBaseURL = "https://api.telegram.org"
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", apiBaseURL, botToken)
	text := a.formatMessage(payload)

	body := map[string]interface{}{
		"chat_id": config.ChatID,
		"text":    text,
		"urgent":  payload.Urgent,
	}

	respBody, statusCode, err := a.httpHelper.PostJSON(ctx, url, nil, body)
	if err != nil {
		if IsTimeoutError(err) {
			return SendResult{}, err
		}
		return SendResult{}, NewTransportError("Telegram API request failed", nil)
	}

	if statusCode < 200 || statusCode >= 300 {
		return SendResult{}, NewTransportError(fmt.Sprintf("Telegram API returned %d", statusCode), nil)
	}

	var telegramResp struct {
		Ok     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}

	if err := json.Unmarshal(respBody, &telegramResp); err != nil {
		return SendResult{}, NewTransportError("Failed to parse Telegram response", nil)
	}

	if !telegramResp.Ok {
		return SendResult{}, NewTransportError("Telegram API returned ok=false", nil)
	}

	return SendResult{
		Sent: true,
		ID:   fmt.Sprintf("%d", telegramResp.Result.MessageID),
	}, nil
}

func (a *TelegramAdapter) formatMessage(payload HumanCallPayload) string {
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
