package prx

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/codeGROOVE-dev/retry"
)

const (
	// retryAttempts is the maximum number of retry attempts.
	retryAttempts = 10
	// retryDelay is the initial retry delay.
	retryDelay = 1 * time.Second
	// retryMaxDelay is the maximum retry delay.
	retryMaxDelay = 2 * time.Minute
	// retryMaxJitter adds randomness to prevent thundering herd.
	retryMaxJitter = 1 * time.Second
	// maxRequestSize limits request body size to prevent memory issues.
	maxRequestSize = 1 * 1024 * 1024 // 1MB - reasonable for API requests
)

// RetryTransport wraps an http.RoundTripper with retry logic using exponential backoff with jitter.
type RetryTransport struct {
	Base http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface with retry logic.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Base == nil {
		t.Base = http.DefaultTransport
	}

	// Log the outgoing request
	slog.InfoContext(req.Context(), "HTTP request starting",
		"method", req.Method,
		"url", req.URL.String(),
		"host", req.URL.Host)

	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(io.LimitReader(req.Body, maxRequestSize))
		if err != nil {
			return nil, err
		}
		if closeErr := req.Body.Close(); closeErr != nil {
			slog.DebugContext(req.Context(), "failed to close request body", "error", closeErr, "url", req.URL.String())
		}
	}

	var resp *http.Response
	var lastErr error

	err := retry.Do(
		func() error { //nolint:contextcheck // Context is accessed via closure from req.Context()
			// Reset the body for each retry attempt
			if bodyBytes != nil {
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}

			var err error
			start := time.Now()
			resp, err = t.Base.RoundTrip(req) //nolint:bodyclose // Response body is handled by caller in successful cases
			elapsed := time.Since(start)
			if err != nil {
				slog.ErrorContext(req.Context(), "HTTP request failed",
					"url", req.URL.String(),
					"error", err,
					"elapsed", elapsed)
				lastErr = err
				return err
			}

			slog.InfoContext(req.Context(), "HTTP response received",
				"status", resp.StatusCode,
				"url", req.URL.String(),
				"elapsed", elapsed)

			// Check if this is a retryable error
			shouldRetry := false
			retryReason := ""

			// Retry on 429 (rate limit) or 5xx server errors
			if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
				shouldRetry = true
				retryReason = "retryable status code"
			}

			// GitHub returns 403 for rate limit errors - check headers to confirm
			if resp.StatusCode == http.StatusForbidden {
				if remaining := resp.Header.Get("X-Ratelimit-Remaining"); remaining == "0" {
					shouldRetry = true
					retryReason = "GitHub rate limit exceeded"
				}
			}

			if shouldRetry {
				bodyBytes, readErr := io.ReadAll(resp.Body)
				if readErr != nil {
					slog.DebugContext(req.Context(), "failed to read response body for retry", "error", readErr)
					bodyBytes = nil
				}
				if closeErr := resp.Body.Close(); closeErr != nil {
					slog.DebugContext(req.Context(), "failed to close response body for retry", "error", closeErr)
				}
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				slog.InfoContext(req.Context(), "HTTP request will be retried",
					"status", resp.StatusCode,
					"url", req.URL.String(),
					"reason", retryReason)
				lastErr = &retryableError{StatusCode: resp.StatusCode}
				return lastErr
			}

			return nil
		},
		retry.Context(req.Context()),
		retry.Attempts(retryAttempts),
		retry.Delay(retryDelay),
		retry.MaxDelay(retryMaxDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxJitter(retryMaxJitter),
		retry.RetryIf(func(err error) bool { //nolint:contextcheck // Context is accessed via closure from req.Context()
			var retryErr *retryableError
			if errors.As(err, &retryErr) {
				return true
			}
			// For any other error, ensure the response body is closed if it exists
			if resp != nil && resp.Body != nil {
				if closeErr := resp.Body.Close(); closeErr != nil {
					slog.DebugContext(req.Context(), "failed to close response body on error", "error", closeErr)
				}
			}
			return false
		}),
	)
	if err != nil {
		if lastErr != nil {
			return resp, lastErr
		}
		return nil, err
	}

	return resp, nil
}

// retryableError indicates an error that should be retried.
type retryableError struct {
	StatusCode int
}

func (e *retryableError) Error() string {
	return http.StatusText(e.StatusCode)
}
