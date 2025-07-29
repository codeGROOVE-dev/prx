package prx

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/codeGROOVE-dev/retry-go"
)

const (
	// retryAttempts is the maximum number of retry attempts
	retryAttempts = 10
	// retryDelay is the initial retry delay
	retryDelay = 1 * time.Second
	// retryMaxDelay is the maximum retry delay
	retryMaxDelay = 2 * time.Minute
	// retryMaxJitter adds randomness to prevent thundering herd
	retryMaxJitter = 1 * time.Second
	// maxRequestSize limits request body size to prevent memory issues
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
	slog.Info("HTTP request starting",
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
		req.Body.Close()
	}

	var resp *http.Response
	var lastErr error

	err := retry.Do(
		func() error {
			// Reset the body for each retry attempt
			if bodyBytes != nil {
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}

			var err error
			start := time.Now()
			resp, err = t.Base.RoundTrip(req)
			elapsed := time.Since(start)
			if err != nil {
				slog.Error("HTTP request failed",
					"url", req.URL.String(),
					"error", err,
					"elapsed", elapsed)
				lastErr = err
				return err
			}

			slog.Info("HTTP response received",
				"status", resp.StatusCode,
				"url", req.URL.String(),
				"elapsed", elapsed)

			// Retry on 429 (rate limit) or 5xx server errors
			if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				slog.Info("HTTP request will be retried",
					"status", resp.StatusCode,
					"url", req.URL.String(),
					"reason", "retryable status code")
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
		retry.RetryIf(func(err error) bool {
			_, ok := err.(*retryableError)
			return ok
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

