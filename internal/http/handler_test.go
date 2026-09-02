package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/EthanBird/human-call-gateway/internal/adapter"
	"github.com/EthanBird/human-call-gateway/internal/config"
)

type fakeAdapter struct {
	sendFunc func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error)
}

func (f *fakeAdapter) Send(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
	return f.sendFunc(ctx, channelConfig, payload)
}

func setupTestHandler(t *testing.T, cfg *config.Config, fakeAdapters map[string]*fakeAdapter) *Handler {
	registry := adapter.NewRegistry()
	for channelType, fakeAdapter := range fakeAdapters {
		registry.Register(channelType, fakeAdapter)
	}
	return NewHandler(cfg, registry)
}

func TestHandler_InvalidPayload(t *testing.T) {
	cfg := &config.Config{
		DefaultChannelID: "test-channel",
		Channels: []config.Channel{
			{
				ID:      "test-channel",
				Type:    "webhook",
				Enabled: boolPtr(true),
				Webhook: &config.WebhookChannelConfig{
					URL: "http://example.com",
				},
			},
		},
	}

	tests := []struct {
		name           string
		body           string
		expectedErrMsg string
	}{
		{
			name:           "missing need field",
			body:           `{"blocker":"test","action":"test","fallback":"test"}`,
			expectedErrMsg: "Field 'need' is required and must be non-empty",
		},
		{
			name:           "empty need field",
			body:           `{"need":"  ","blocker":"test","action":"test","fallback":"test"}`,
			expectedErrMsg: "Field 'need' is required and must be non-empty",
		},
		{
			name:           "need field too long",
			body:           `{"need":"` + strings.Repeat("a", 501) + `","blocker":"test","action":"test","fallback":"test"}`,
			expectedErrMsg: "Field 'need' exceeds maximum length of 500 characters",
		},
		{
			name:           "missing blocker field",
			body:           `{"need":"test","action":"test","fallback":"test"}`,
			expectedErrMsg: "Field 'blocker' is required and must be non-empty",
		},
		{
			name:           "missing action field",
			body:           `{"need":"test","blocker":"test","fallback":"test"}`,
			expectedErrMsg: "Field 'action' is required and must be non-empty",
		},
		{
			name:           "missing fallback field",
			body:           `{"need":"test","blocker":"test","action":"test"}`,
			expectedErrMsg: "Field 'fallback' is required and must be non-empty",
		},
		{
			name:           "unknown field",
			body:           `{"need":"test","blocker":"test","action":"test","fallback":"test","unknown":"field"}`,
			expectedErrMsg: "Invalid JSON",
		},
		{
			name:           "invalid JSON",
			body:           `{invalid json}`,
			expectedErrMsg: "Invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := setupTestHandler(t, cfg, map[string]*fakeAdapter{
				"webhook": {
					sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
						t.Fatal("adapter should not be called")
						return adapter.SendResult{}, nil
					},
				},
			})

			req := httptest.NewRequest("POST", "/v1/human-call", bytes.NewReader([]byte(tt.body)))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", w.Code)
			}

			var resp ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Sent != false {
				t.Errorf("expected sent=false, got %v", resp.Sent)
			}

			if resp.Error.Code != "INVALID_PAYLOAD" {
				t.Errorf("expected code=INVALID_PAYLOAD, got %s", resp.Error.Code)
			}

			if !strings.Contains(resp.Error.Message, tt.expectedErrMsg) {
				t.Errorf("expected message to contain %q, got %q", tt.expectedErrMsg, resp.Error.Message)
			}
		})
	}
}

func TestHandler_ConfigMissing(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.Config
		requestBody    string
		expectedErrMsg string
	}{
		{
			name: "channel_id not found",
			config: &config.Config{
				Channels: []config.Channel{
					{
						ID:      "test-channel",
						Type:    "webhook",
						Enabled: boolPtr(true),
						Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
					},
				},
			},
			requestBody:    `{"need":"test","blocker":"test","action":"test","fallback":"test","channel_id":"nonexistent"}`,
			expectedErrMsg: "Channel 'nonexistent' not found in configuration",
		},
		{
			name: "channel disabled",
			config: &config.Config{
				Channels: []config.Channel{
					{
						ID:      "disabled-channel",
						Type:    "webhook",
						Enabled: boolPtr(false),
						Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
					},
				},
			},
			requestBody:    `{"need":"test","blocker":"test","action":"test","fallback":"test","channel_id":"disabled-channel"}`,
			expectedErrMsg: "Channel 'disabled-channel' is disabled",
		},
		{
			name: "default_channel_id not configured",
			config: &config.Config{
				Channels: []config.Channel{
					{
						ID:      "test-channel",
						Type:    "webhook",
						Enabled: boolPtr(true),
						Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
					},
				},
			},
			requestBody:    `{"need":"test","blocker":"test","action":"test","fallback":"test"}`,
			expectedErrMsg: "channel_id is required when default_channel_id is not configured",
		},
		{
			name: "required env var not set (slack)",
			config: &config.Config{
				DefaultChannelID: "slack-channel",
				Channels: []config.Channel{
					{
						ID:      "slack-channel",
						Type:    "slack",
						Enabled: boolPtr(true),
						Slack:   &config.SlackChannelConfig{WebhookURLEnv: "SLACK_WEBHOOK_MISSING"},
					},
				},
			},
			requestBody:    `{"need":"test","blocker":"test","action":"test","fallback":"test"}`,
			expectedErrMsg: "Environment variable SLACK_WEBHOOK_MISSING required for Slack channel is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("SLACK_WEBHOOK_MISSING")

			registry := adapter.NewRegistry()
			httpHelper := adapter.NewHTTPHelper(registry.GetHTTPClient())
			registry.Register("webhook", &fakeAdapter{
				sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
					t.Fatal("adapter should not be called")
					return adapter.SendResult{}, nil
				},
			})
			registry.Register("slack", adapter.NewSlackAdapter(httpHelper))

			handler := NewHandler(tt.config, registry)

			req := httptest.NewRequest("POST", "/v1/human-call", bytes.NewReader([]byte(tt.requestBody)))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", w.Code)
			}

			var resp ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Sent != false {
				t.Errorf("expected sent=false, got %v", resp.Sent)
			}

			if resp.Error.Code != "CONFIG_MISSING" {
				t.Errorf("expected code=CONFIG_MISSING, got %s", resp.Error.Code)
			}

			if !strings.Contains(resp.Error.Message, tt.expectedErrMsg) {
				t.Errorf("expected message to contain %q, got %q", tt.expectedErrMsg, resp.Error.Message)
			}
		})
	}
}

func TestHandler_UrgentPassThrough(t *testing.T) {
	tests := []struct {
		name           string
		urgentValue    *bool
		expectedUrgent bool
	}{
		{
			name:           "urgent=true",
			urgentValue:    boolPtr(true),
			expectedUrgent: true,
		},
		{
			name:           "urgent=false",
			urgentValue:    boolPtr(false),
			expectedUrgent: false,
		},
		{
			name:           "urgent omitted (defaults to false)",
			urgentValue:    nil,
			expectedUrgent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedUrgent bool

			cfg := &config.Config{
				DefaultChannelID: "test-channel",
				Channels: []config.Channel{
					{
						ID:      "test-channel",
						Type:    "webhook",
						Enabled: boolPtr(true),
						Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
					},
				},
			}

			handler := setupTestHandler(t, cfg, map[string]*fakeAdapter{
				"webhook": {
					sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
						receivedUrgent = payload.Urgent
						return adapter.SendResult{Sent: true, ID: "test-id"}, nil
					},
				},
			})

			body := map[string]interface{}{
				"need":     "test",
				"blocker":  "test",
				"action":   "test",
				"fallback": "test",
			}
			if tt.urgentValue != nil {
				body["urgent"] = *tt.urgentValue
			}

			jsonBody, _ := json.Marshal(body)
			req := httptest.NewRequest("POST", "/v1/human-call", bytes.NewReader(jsonBody))
			w := httptest.NewRecorder()

			startTime := time.Now()
			handler.ServeHTTP(w, req)
			elapsed := time.Since(startTime)

			if elapsed > 1*time.Second {
				t.Errorf("request took too long: %v (urgent should not change timeout)", elapsed)
			}

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}

			if receivedUrgent != tt.expectedUrgent {
				t.Errorf("expected urgent=%v, got %v", tt.expectedUrgent, receivedUrgent)
			}
		})
	}
}

func TestHandler_TransportSuccess(t *testing.T) {
	cfg := &config.Config{
		DefaultChannelID: "test-channel",
		Channels: []config.Channel{
			{
				ID:      "test-channel",
				Type:    "webhook",
				Enabled: boolPtr(true),
				Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
			},
		},
	}

	handler := setupTestHandler(t, cfg, map[string]*fakeAdapter{
		"webhook": {
			sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
				return adapter.SendResult{Sent: true, ID: "transport-msg-123"}, nil
			},
		},
	})

	body := `{"need":"test","blocker":"test","action":"test","fallback":"test"}`
	req := httptest.NewRequest("POST", "/v1/human-call", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Sent != true {
		t.Errorf("expected sent=true, got %v", resp.Sent)
	}

	if resp.ID != "transport-msg-123" {
		t.Errorf("expected id=transport-msg-123, got %s", resp.ID)
	}
}

func TestHandler_TransportError(t *testing.T) {
	cfg := &config.Config{
		DefaultChannelID: "test-channel",
		Channels: []config.Channel{
			{
				ID:      "test-channel",
				Type:    "webhook",
				Enabled: boolPtr(true),
				Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
			},
		},
	}

	handler := setupTestHandler(t, cfg, map[string]*fakeAdapter{
		"webhook": {
			sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
				return adapter.SendResult{}, adapter.NewTransportError("upstream returned 404", nil)
			},
		},
	})

	body := `{"need":"test","blocker":"test","action":"test","fallback":"test"}`
	req := httptest.NewRequest("POST", "/v1/human-call", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Sent != false {
		t.Errorf("expected sent=false, got %v", resp.Sent)
	}

	if resp.Error.Code != "TRANSPORT_ERROR" {
		t.Errorf("expected code=TRANSPORT_ERROR, got %s", resp.Error.Code)
	}

	if !strings.Contains(resp.Error.Message, "upstream returned 404") {
		t.Errorf("expected message to contain 'upstream returned 404', got %q", resp.Error.Message)
	}
}

func TestHandler_Timeout(t *testing.T) {
	cfg := &config.Config{
		DefaultChannelID: "test-channel",
		Channels: []config.Channel{
			{
				ID:      "test-channel",
				Type:    "webhook",
				Enabled: boolPtr(true),
				Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
			},
		},
	}

	handler := setupTestHandler(t, cfg, map[string]*fakeAdapter{
		"webhook": {
			sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
				return adapter.SendResult{}, adapter.NewTimeoutError("HTTP client exceeded 10s timeout")
			},
		},
	})

	body := `{"need":"test","blocker":"test","action":"test","fallback":"test"}`
	req := httptest.NewRequest("POST", "/v1/human-call", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("expected status 504, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Sent != false {
		t.Errorf("expected sent=false, got %v", resp.Sent)
	}

	if resp.Error.Code != "TIMEOUT" {
		t.Errorf("expected code=TIMEOUT, got %s", resp.Error.Code)
	}
}

func TestHandler_HTTPTransportWithTimeout(t *testing.T) {
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(11 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	cfg := &config.Config{
		DefaultChannelID: "test-channel",
		Channels: []config.Channel{
			{
				ID:      "test-channel",
				Type:    "webhook",
				Enabled: boolPtr(true),
				Webhook: &config.WebhookChannelConfig{URL: slowServer.URL},
			},
		},
	}

	registry := adapter.NewRegistry()
	httpHelper := adapter.NewHTTPHelper(registry.GetHTTPClient())
	registry.Register("webhook", adapter.NewWebhookAdapter(httpHelper))

	handler := NewHandler(cfg, registry)

	body := `{"need":"test","blocker":"test","action":"test","fallback":"test"}`
	req := httptest.NewRequest("POST", "/v1/human-call", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("expected status 504, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error.Code != "TIMEOUT" {
		t.Errorf("expected code=TIMEOUT, got %s", resp.Error.Code)
	}
}

func TestHandler_NoSensitiveInfoInErrors(t *testing.T) {
	// Test that failed transports don't leak webhook URLs, tokens, or other sensitive data
	sensitiveWebhookURL := "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXX"
	sensitiveTelegramToken := "123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
	
	tests := []struct {
		name              string
		channelType       string
		config            *config.Config
		setupEnv          func()
		cleanupEnv        func()
		forbiddenStrings  []string
	}{
		{
			name:        "Slack webhook URL not in error message",
			channelType: "slack",
			config: &config.Config{
				DefaultChannelID: "slack-test",
				Channels: []config.Channel{
					{
						ID:      "slack-test",
						Type:    "slack",
						Enabled: boolPtr(true),
						Slack:   &config.SlackChannelConfig{WebhookURLEnv: "TEST_SLACK_WEBHOOK"},
					},
				},
			},
			setupEnv: func() {
				os.Setenv("TEST_SLACK_WEBHOOK", sensitiveWebhookURL)
			},
			cleanupEnv: func() {
				os.Unsetenv("TEST_SLACK_WEBHOOK")
			},
			forbiddenStrings: []string{
				"hooks.slack.com",
				sensitiveWebhookURL,
				"XXXXXXXXXXXXXXXXXXXX",
			},
		},
		{
			name:        "Telegram token not in error message",
			channelType: "telegram",
			config: &config.Config{
				DefaultChannelID: "telegram-test",
				Channels: []config.Channel{
					{
						ID:      "telegram-test",
						Type:    "telegram",
						Enabled: boolPtr(true),
						Telegram: &config.TelegramChannelConfig{
							BotTokenEnv: "TEST_TELEGRAM_TOKEN",
							ChatID:      "-1001234567890",
							APIBaseURL:  "https://api.telegram.org",
						},
					},
				},
			},
			setupEnv: func() {
				os.Setenv("TEST_TELEGRAM_TOKEN", sensitiveTelegramToken)
			},
			cleanupEnv: func() {
				os.Unsetenv("TEST_TELEGRAM_TOKEN")
			},
			forbiddenStrings: []string{
				sensitiveTelegramToken,
				"ABCdefGHIjklMNOpqrsTUVwxyz",
				"/bot" + sensitiveTelegramToken,
			},
		},
		{
			name:        "Webhook URL with token not in error message",
			channelType: "webhook",
			config: &config.Config{
				DefaultChannelID: "webhook-test",
				Channels: []config.Channel{
					{
						ID:      "webhook-test",
						Type:    "webhook",
						Enabled: boolPtr(true),
						Webhook: &config.WebhookChannelConfig{
							URLEnv: "TEST_WEBHOOK_URL",
						},
					},
				},
			},
			setupEnv: func() {
				os.Setenv("TEST_WEBHOOK_URL", "https://example.com/webhook?token=secret123456")
			},
			cleanupEnv: func() {
				os.Unsetenv("TEST_WEBHOOK_URL")
			},
			forbiddenStrings: []string{
				"https://example.com/webhook?token=secret123456",
				"secret123456",
				"token=secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.cleanupEnv()

			// Create a fake failing transport
			slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"not_found"}`))
			}))
			defer slowServer.Close()

			registry := adapter.NewRegistry()
			httpHelper := adapter.NewHTTPHelper(registry.GetHTTPClient())
			
			switch tt.channelType {
			case "slack":
				registry.Register("slack", adapter.NewSlackAdapter(httpHelper))
			case "telegram":
				registry.Register("telegram", adapter.NewTelegramAdapter(httpHelper))
			case "webhook":
				registry.Register("webhook", adapter.NewWebhookAdapter(httpHelper))
			}

			handler := NewHandler(tt.config, registry)

			body := `{"need":"test","blocker":"test","action":"test","fallback":"test"}`
			req := httptest.NewRequest("POST", "/v1/human-call", bytes.NewReader([]byte(body)))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// Should return an error (not 200)
			if w.Code == http.StatusOK {
				t.Errorf("expected error status, got 200")
			}

			var resp ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// Check that error message doesn't contain forbidden strings
			errorMessage := resp.Error.Message
			for _, forbidden := range tt.forbiddenStrings {
				if strings.Contains(errorMessage, forbidden) {
					t.Errorf("error message contains forbidden string %q: %s", forbidden, errorMessage)
				}
			}

			// Error message should be generic
			if errorMessage == "" {
				t.Errorf("error message should not be empty")
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestHandler_HealthCheck(t *testing.T) {
	cfg := &config.Config{
		DefaultChannelID: "test-channel",
		Channels: []config.Channel{
			{
				ID:      "test-channel",
				Type:    "webhook",
				Enabled: boolPtr(true),
				Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
			},
		},
	}

	handler := setupTestHandler(t, cfg, map[string]*fakeAdapter{
		"webhook": {
			sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
				t.Fatal("adapter should not be called for health check")
				return adapter.SendResult{}, nil
			},
		},
	})

	tests := []struct {
		name   string
		path   string
		method string
	}{
		{name: "GET /health", path: "/health", method: "GET"},
		{name: "GET /", path: "/", method: "GET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", contentType)
			}

			body := w.Body.String()
			expected := `{"status":"ok"}`
			if body != expected {
				t.Errorf("expected body %s, got %s", expected, body)
			}
		})
	}
}

func TestHandler_HealthCheckMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{
		DefaultChannelID: "test-channel",
		Channels: []config.Channel{
			{
				ID:      "test-channel",
				Type:    "webhook",
				Enabled: boolPtr(true),
				Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
			},
		},
	}

	handler := setupTestHandler(t, cfg, map[string]*fakeAdapter{
		"webhook": {
			sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
				t.Fatal("adapter should not be called")
				return adapter.SendResult{}, nil
			},
		},
	})

	req := httptest.NewRequest("POST", "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandler_BearerTokenAuth(t *testing.T) {
	cfg := &config.Config{
		DefaultChannelID: "test-channel",
		Channels: []config.Channel{
			{
				ID:      "test-channel",
				Type:    "webhook",
				Enabled: boolPtr(true),
				Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
			},
		},
	}

	tests := []struct {
		name           string
		gatewayToken   string
		authHeader     string
		expectedStatus int
		shouldCallSend bool
	}{
		{
			name:           "no token configured, no auth header",
			gatewayToken:   "",
			authHeader:     "",
			expectedStatus: http.StatusOK,
			shouldCallSend: true,
		},
		{
			name:           "no token configured, with auth header",
			gatewayToken:   "",
			authHeader:     "Bearer some-token",
			expectedStatus: http.StatusOK,
			shouldCallSend: true,
		},
		{
			name:           "token configured, correct auth",
			gatewayToken:   "secret-token-123",
			authHeader:     "Bearer secret-token-123",
			expectedStatus: http.StatusOK,
			shouldCallSend: true,
		},
		{
			name:           "token configured, no auth header",
			gatewayToken:   "secret-token-123",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			shouldCallSend: false,
		},
		{
			name:           "token configured, wrong token",
			gatewayToken:   "secret-token-123",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
			shouldCallSend: false,
		},
		{
			name:           "token configured, malformed auth header",
			gatewayToken:   "secret-token-123",
			authHeader:     "secret-token-123",
			expectedStatus: http.StatusUnauthorized,
			shouldCallSend: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("GATEWAY_TOKEN", tt.gatewayToken)
			defer os.Unsetenv("GATEWAY_TOKEN")

			sendCalled := false
			handler := setupTestHandler(t, cfg, map[string]*fakeAdapter{
				"webhook": {
					sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
						sendCalled = true
						return adapter.SendResult{Sent: true, ID: "test-id"}, nil
					},
				},
			})

			body := `{"need":"test","blocker":"test","action":"test","fallback":"test"}`
			req := httptest.NewRequest("POST", "/v1/human-call", bytes.NewReader([]byte(body)))
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if sendCalled != tt.shouldCallSend {
				t.Errorf("expected sendCalled=%v, got %v", tt.shouldCallSend, sendCalled)
			}

			if tt.expectedStatus == http.StatusUnauthorized {
				var resp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if resp.Error.Code != "INVALID_PAYLOAD" {
					t.Errorf("expected error code INVALID_PAYLOAD, got %s", resp.Error.Code)
				}
				if !strings.Contains(resp.Error.Message, "Unauthorized") {
					t.Errorf("expected error message to contain 'Unauthorized', got %s", resp.Error.Message)
				}
			}
		})
	}
}

func TestHandler_HealthCheckNoAuth(t *testing.T) {
	cfg := &config.Config{
		DefaultChannelID: "test-channel",
		Channels: []config.Channel{
			{
				ID:      "test-channel",
				Type:    "webhook",
				Enabled: boolPtr(true),
				Webhook: &config.WebhookChannelConfig{URL: "http://example.com"},
			},
		},
	}

	os.Setenv("GATEWAY_TOKEN", "secret-token-123")
	defer os.Unsetenv("GATEWAY_TOKEN")

	handler := setupTestHandler(t, cfg, map[string]*fakeAdapter{
		"webhook": {
			sendFunc: func(ctx context.Context, channelConfig adapter.ChannelConfig, payload adapter.HumanCallPayload) (adapter.SendResult, error) {
				t.Fatal("adapter should not be called for health check")
				return adapter.SendResult{}, nil
			},
		},
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 even with GATEWAY_TOKEN set, got %d", w.Code)
	}
}
