package adapter

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ErrorCode represents the error code types
type ErrorCode string

const (
	ErrCodeConfigMissing   ErrorCode = "CONFIG_MISSING"
	ErrCodeInvalidPayload  ErrorCode = "INVALID_PAYLOAD"
	ErrCodeTransportError  ErrorCode = "TRANSPORT_ERROR"
	ErrCodeTimeout         ErrorCode = "TIMEOUT"
)

// AdapterError represents an error with a code
type AdapterError struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *AdapterError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AdapterError) Unwrap() error {
	return e.Err
}

// NewConfigMissingError creates a CONFIG_MISSING error
func NewConfigMissingError(message string) *AdapterError {
	return &AdapterError{
		Code:    ErrCodeConfigMissing,
		Message: message,
	}
}

// NewTransportError creates a TRANSPORT_ERROR error
func NewTransportError(message string, err error) *AdapterError {
	return &AdapterError{
		Code:    ErrCodeTransportError,
		Message: message,
		Err:     err,
	}
}

// NewTimeoutError creates a TIMEOUT error
func NewTimeoutError(message string) *AdapterError {
	return &AdapterError{
		Code:    ErrCodeTimeout,
		Message: message,
	}
}

// HumanCallPayload represents the structured request payload
type HumanCallPayload struct {
	Need      string
	Blocker   string
	Action    string
	Fallback  string
	ChannelID string
	Urgent    bool
}

// SendResult represents the result of a send operation
type SendResult struct {
	Sent bool
	ID   string
}

// ChannelConfig is the interface for channel-specific configuration
type ChannelConfig interface {
	GetType() string
}

// Adapter is the interface for all channel adapters
type Adapter interface {
	Send(ctx context.Context, channelConfig ChannelConfig, payload HumanCallPayload) (SendResult, error)
}

// Registry holds all registered adapters
type Registry struct {
	adapters map[string]Adapter
	client   *http.Client
}

// NewRegistry creates a new adapter registry with a shared HTTP client
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Register registers an adapter for a channel type
func (r *Registry) Register(channelType string, adapter Adapter) {
	r.adapters[channelType] = adapter
}

// Get returns an adapter for a channel type
func (r *Registry) Get(channelType string) (Adapter, bool) {
	adapter, ok := r.adapters[channelType]
	return adapter, ok
}

// GetHTTPClient returns the shared HTTP client
func (r *Registry) GetHTTPClient() *http.Client {
	return r.client
}

// IsTimeoutError checks if an error is a timeout error
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var adapterErr *AdapterError
	if errors.As(err, &adapterErr) {
		return adapterErr.Code == ErrCodeTimeout
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// generateTimestampID generates a simple timestamp-based ID
func generateTimestampID() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// IsAdapterError checks if an error is an AdapterError and returns it
func IsAdapterError(err error, target **AdapterError) bool {
	return errors.As(err, target)
}
