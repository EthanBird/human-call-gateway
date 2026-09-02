package config

import (
	"fmt"
	"os"

	"github.com/EthanBird/human-call-gateway/internal/adapter"
	"gopkg.in/yaml.v3"
)

// Config represents the gateway configuration
type Config struct {
	DefaultChannelID string    `yaml:"default_channel_id"`
	Channels         []Channel `yaml:"channels"`
}

// Channel represents a notification channel
type Channel struct {
	ID       string                 `yaml:"id"`
	Type     string                 `yaml:"type"`
	Enabled  *bool                  `yaml:"enabled"`
	Slack    *SlackChannelConfig    `yaml:"slack,omitempty"`
	Telegram *TelegramChannelConfig `yaml:"telegram,omitempty"`
	Feishu   *FeishuChannelConfig   `yaml:"feishu,omitempty"`
	QQ       *QQChannelConfig       `yaml:"qq,omitempty"`
	Webhook  *WebhookChannelConfig  `yaml:"webhook,omitempty"`
}

// IsEnabled returns whether the channel is enabled (default true)
func (c *Channel) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// GetChannelConfig returns the adapter.ChannelConfig for this channel
func (c *Channel) GetChannelConfig() (adapter.ChannelConfig, error) {
	switch c.Type {
	case "slack":
		if c.Slack == nil {
			return nil, fmt.Errorf("slack configuration missing for channel %s", c.ID)
		}
		return &adapter.SlackConfig{
			WebhookURLEnv: c.Slack.WebhookURLEnv,
		}, nil
	case "telegram":
		if c.Telegram == nil {
			return nil, fmt.Errorf("telegram configuration missing for channel %s", c.ID)
		}
		apiBaseURL := c.Telegram.APIBaseURL
		if apiBaseURL == "" {
			apiBaseURL = "https://api.telegram.org"
		}
		return &adapter.TelegramConfig{
			BotTokenEnv: c.Telegram.BotTokenEnv,
			ChatID:      c.Telegram.ChatID,
			APIBaseURL:  apiBaseURL,
		}, nil
	case "feishu":
		if c.Feishu == nil {
			return nil, fmt.Errorf("feishu configuration missing for channel %s", c.ID)
		}
		return &adapter.FeishuConfig{
			WebhookURLEnv: c.Feishu.WebhookURLEnv,
		}, nil
	case "qq":
		if c.QQ == nil {
			return nil, fmt.Errorf("qq configuration missing for channel %s", c.ID)
		}
		return &adapter.QQConfig{
			HTTPAPIUrl:     c.QQ.HTTPAPIUrl,
			AccessTokenEnv: c.QQ.AccessTokenEnv,
			UserID:         c.QQ.UserID,
			GroupID:        c.QQ.GroupID,
		}, nil
	case "webhook":
		if c.Webhook == nil {
			return nil, fmt.Errorf("webhook configuration missing for channel %s", c.ID)
		}
		contentType := c.Webhook.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		method := c.Webhook.Method
		if method == "" {
			method = "POST"
		}
		return &adapter.WebhookConfig{
			URL:         c.Webhook.URL,
			URLEnv:      c.Webhook.URLEnv,
			Method:      method,
			Headers:     c.Webhook.Headers,
			ContentType: contentType,
		}, nil
	default:
		return nil, fmt.Errorf("unknown channel type: %s", c.Type)
	}
}

// SlackChannelConfig represents Slack-specific configuration
type SlackChannelConfig struct {
	WebhookURLEnv string `yaml:"webhook_url_env"`
}

// TelegramChannelConfig represents Telegram-specific configuration
type TelegramChannelConfig struct {
	BotTokenEnv string `yaml:"bot_token_env"`
	ChatID      string `yaml:"chat_id"`
	APIBaseURL  string `yaml:"api_base_url"`
}

// FeishuChannelConfig represents Feishu-specific configuration
type FeishuChannelConfig struct {
	WebhookURLEnv string `yaml:"webhook_url_env"`
}

// QQChannelConfig represents QQ-specific configuration
type QQChannelConfig struct {
	HTTPAPIUrl     string `yaml:"http_api_url"`
	AccessTokenEnv string `yaml:"access_token_env"`
	UserID         string `yaml:"user_id"`
	GroupID        string `yaml:"group_id"`
}

// WebhookChannelConfig represents generic webhook configuration
type WebhookChannelConfig struct {
	URL         string            `yaml:"url"`
	URLEnv      string            `yaml:"url_env"`
	Method      string            `yaml:"method"`
	Headers     map[string]string `yaml:"headers"`
	ContentType string            `yaml:"content_type"`
}

// Load loads configuration from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if len(c.Channels) == 0 {
		return fmt.Errorf("at least one channel is required")
	}

	channelIDs := make(map[string]bool)
	for i, ch := range c.Channels {
		if ch.ID == "" {
			return fmt.Errorf("channel[%d]: id is required", i)
		}
		if channelIDs[ch.ID] {
			return fmt.Errorf("channel[%d]: duplicate channel id %s", i, ch.ID)
		}
		channelIDs[ch.ID] = true

		if ch.Type == "" {
			return fmt.Errorf("channel[%d] (%s): type is required", i, ch.ID)
		}

		switch ch.Type {
		case "slack":
			if ch.Slack == nil {
				return fmt.Errorf("channel[%d] (%s): slack configuration is required for type slack", i, ch.ID)
			}
			if ch.Slack.WebhookURLEnv == "" {
				return fmt.Errorf("channel[%d] (%s): slack.webhook_url_env is required", i, ch.ID)
			}
		case "telegram":
			if ch.Telegram == nil {
				return fmt.Errorf("channel[%d] (%s): telegram configuration is required for type telegram", i, ch.ID)
			}
			if ch.Telegram.BotTokenEnv == "" {
				return fmt.Errorf("channel[%d] (%s): telegram.bot_token_env is required", i, ch.ID)
			}
			if ch.Telegram.ChatID == "" {
				return fmt.Errorf("channel[%d] (%s): telegram.chat_id is required", i, ch.ID)
			}
		case "feishu":
			if ch.Feishu == nil {
				return fmt.Errorf("channel[%d] (%s): feishu configuration is required for type feishu", i, ch.ID)
			}
			if ch.Feishu.WebhookURLEnv == "" {
				return fmt.Errorf("channel[%d] (%s): feishu.webhook_url_env is required", i, ch.ID)
			}
		case "qq":
			if ch.QQ == nil {
				return fmt.Errorf("channel[%d] (%s): qq configuration is required for type qq", i, ch.ID)
			}
			if ch.QQ.HTTPAPIUrl == "" {
				return fmt.Errorf("channel[%d] (%s): qq.http_api_url is required", i, ch.ID)
			}
			if ch.QQ.UserID == "" && ch.QQ.GroupID == "" {
				return fmt.Errorf("channel[%d] (%s): qq requires at least one of user_id or group_id", i, ch.ID)
			}
		case "webhook":
			if ch.Webhook == nil {
				return fmt.Errorf("channel[%d] (%s): webhook configuration is required for type webhook", i, ch.ID)
			}
			if ch.Webhook.URL == "" && ch.Webhook.URLEnv == "" {
				return fmt.Errorf("channel[%d] (%s): webhook requires either url or url_env", i, ch.ID)
			}
			if ch.Webhook.URL != "" && ch.Webhook.URLEnv != "" {
				return fmt.Errorf("channel[%d] (%s): webhook cannot have both url and url_env", i, ch.ID)
			}
		default:
			return fmt.Errorf("channel[%d] (%s): unknown channel type %s", i, ch.ID, ch.Type)
		}
	}

	if c.DefaultChannelID != "" && !channelIDs[c.DefaultChannelID] {
		return fmt.Errorf("default_channel_id %s does not match any channel", c.DefaultChannelID)
	}

	return nil
}

// FindChannel finds a channel by ID
func (c *Config) FindChannel(id string) (*Channel, bool) {
	for i := range c.Channels {
		if c.Channels[i].ID == id {
			return &c.Channels[i], true
		}
	}
	return nil, false
}
