package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/EthanBird/human-call-gateway/internal/adapter"
	"github.com/EthanBird/human-call-gateway/internal/config"
)

// HumanCallRequest represents the API request
type HumanCallRequest struct {
	Need      string `json:"need"`
	Blocker   string `json:"blocker"`
	Action    string `json:"action"`
	Fallback  string `json:"fallback"`
	ChannelID string `json:"channel_id"`
	Urgent    bool   `json:"urgent"`
}

// SuccessResponse represents a successful response
type SuccessResponse struct {
	Sent bool   `json:"sent"`
	ID   string `json:"id"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Sent  bool       `json:"sent"`
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Handler handles HTTP requests for the gateway
type Handler struct {
	config      *config.Config
	registry    *adapter.Registry
	gatewayToken string
}

// NewHandler creates a new HTTP handler
func NewHandler(cfg *config.Config, registry *adapter.Registry) *Handler {
	return &Handler{
		config:       cfg,
		registry:     registry,
		gatewayToken: strings.TrimSpace(getEnv("GATEWAY_TOKEN")),
	}
}

// ServeHTTP implements http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Health check endpoints (no auth required)
	if r.URL.Path == "/health" || r.URL.Path == "/" {
		if r.Method != http.MethodGet {
			h.sendError(w, http.StatusMethodNotAllowed, adapter.ErrCodeInvalidPayload, "Method not allowed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	// Only POST /v1/human-call is supported beyond this point
	if r.URL.Path != "/v1/human-call" {
		h.sendError(w, http.StatusNotFound, adapter.ErrCodeInvalidPayload, "Not found")
		return
	}

	if r.Method != http.MethodPost {
		h.sendError(w, http.StatusMethodNotAllowed, adapter.ErrCodeInvalidPayload, "Method not allowed")
		return
	}

	// Check Bearer token if GATEWAY_TOKEN is configured
	if h.gatewayToken != "" {
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + h.gatewayToken
		if authHeader != expectedAuth {
			h.sendError(w, http.StatusUnauthorized, adapter.ErrCodeInvalidPayload, "Unauthorized")
			return
		}
	}

	var req HumanCallRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, adapter.ErrCodeInvalidPayload, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.validateRequest(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, adapter.ErrCodeInvalidPayload, err.Error())
		return
	}

	channelID := req.ChannelID
	if channelID == "" {
		channelID = h.config.DefaultChannelID
		if channelID == "" {
			h.sendError(w, http.StatusBadRequest, adapter.ErrCodeConfigMissing, "channel_id is required when default_channel_id is not configured")
			return
		}
	}

	channel, found := h.config.FindChannel(channelID)
	if !found {
		h.sendError(w, http.StatusBadRequest, adapter.ErrCodeConfigMissing, fmt.Sprintf("Channel '%s' not found in configuration", channelID))
		return
	}

	if !channel.IsEnabled() {
		h.sendError(w, http.StatusBadRequest, adapter.ErrCodeConfigMissing, fmt.Sprintf("Channel '%s' is disabled", channelID))
		return
	}

	adapterInstance, found := h.registry.Get(channel.Type)
	if !found {
		h.sendError(w, http.StatusInternalServerError, adapter.ErrCodeConfigMissing, fmt.Sprintf("No adapter registered for channel type '%s'", channel.Type))
		return
	}

	channelConfig, err := channel.GetChannelConfig()
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, adapter.ErrCodeConfigMissing, err.Error())
		return
	}

	payload := adapter.HumanCallPayload{
		Need:      req.Need,
		Blocker:   req.Blocker,
		Action:    req.Action,
		Fallback:  req.Fallback,
		ChannelID: channelID,
		Urgent:    req.Urgent,
	}

	ctx := r.Context()
	result, err := adapterInstance.Send(ctx, channelConfig, payload)
	if err != nil {
		h.handleAdapterError(w, err)
		return
	}

	h.sendSuccess(w, result.ID)
}

func (h *Handler) validateRequest(req *HumanCallRequest) error {
	req.Need = strings.TrimSpace(req.Need)
	req.Blocker = strings.TrimSpace(req.Blocker)
	req.Action = strings.TrimSpace(req.Action)
	req.Fallback = strings.TrimSpace(req.Fallback)

	if req.Need == "" {
		return fmt.Errorf("Field 'need' is required and must be non-empty")
	}
	if len(req.Need) > 500 {
		return fmt.Errorf("Field 'need' exceeds maximum length of 500 characters")
	}

	if req.Blocker == "" {
		return fmt.Errorf("Field 'blocker' is required and must be non-empty")
	}
	if len(req.Blocker) > 500 {
		return fmt.Errorf("Field 'blocker' exceeds maximum length of 500 characters")
	}

	if req.Action == "" {
		return fmt.Errorf("Field 'action' is required and must be non-empty")
	}
	if len(req.Action) > 500 {
		return fmt.Errorf("Field 'action' exceeds maximum length of 500 characters")
	}

	if req.Fallback == "" {
		return fmt.Errorf("Field 'fallback' is required and must be non-empty")
	}
	if len(req.Fallback) > 500 {
		return fmt.Errorf("Field 'fallback' exceeds maximum length of 500 characters")
	}

	return nil
}

func (h *Handler) handleAdapterError(w http.ResponseWriter, err error) {
	var adapterErr *adapter.AdapterError
	if !adapter.IsAdapterError(err, &adapterErr) {
		h.sendError(w, http.StatusInternalServerError, adapter.ErrCodeTransportError, "Internal server error")
		return
	}

	switch adapterErr.Code {
	case adapter.ErrCodeConfigMissing:
		h.sendError(w, http.StatusBadRequest, adapterErr.Code, adapterErr.Message)
	case adapter.ErrCodeTimeout:
		h.sendError(w, http.StatusGatewayTimeout, adapterErr.Code, adapterErr.Message)
	case adapter.ErrCodeTransportError:
		h.sendError(w, http.StatusBadGateway, adapterErr.Code, adapterErr.Message)
	default:
		h.sendError(w, http.StatusInternalServerError, adapter.ErrCodeTransportError, "Internal server error")
	}
}

func (h *Handler) sendSuccess(w http.ResponseWriter, id string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SuccessResponse{
		Sent: true,
		ID:   id,
	})
}

func (h *Handler) sendError(w http.ResponseWriter, statusCode int, code adapter.ErrorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Sent: false,
		Error: ErrorDetail{
			Code:    string(code),
			Message: message,
		},
	})
}

func getEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
