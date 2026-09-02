package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
)

// HTTPHelper provides common HTTP operations for adapters
type HTTPHelper struct {
	client *http.Client
}

// NewHTTPHelper creates a new HTTP helper
func NewHTTPHelper(client *http.Client) *HTTPHelper {
	return &HTTPHelper{client: client}
}

// PostJSON sends a POST request with JSON body
func (h *HTTPHelper) PostJSON(ctx context.Context, url string, headers map[string]string, body interface{}) ([]byte, int, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal JSON: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		// Check for timeout errors
		if ctx.Err() == context.DeadlineExceeded {
			return nil, 0, NewTimeoutError("HTTP client exceeded 10s timeout")
		}
		// Also check if the error itself indicates a timeout
		if isNetTimeoutError(err) {
			return nil, 0, NewTimeoutError("HTTP client exceeded 10s timeout")
		}
		return nil, 0, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// isNetTimeoutError checks if an error is a network timeout error
func isNetTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	if err == os.ErrDeadlineExceeded {
		return true
	}
	return false
}
