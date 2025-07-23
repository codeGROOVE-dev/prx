package prevents

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/avast/retry-go/v4"
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

	// Clone the request body if present, as it can only be read once
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		// Limit request body size to prevent memory issues
		const maxRequestSize = 1 * 1024 * 1024 // 1MB - reasonable for API requests
		limitedReader := io.LimitReader(req.Body, maxRequestSize)
		bodyBytes, err = io.ReadAll(limitedReader)
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
			resp, err = t.Base.RoundTrip(req)
			if err != nil {
				lastErr = err
				return err
			}

			// Retry on rate limit (429) or server errors (5xx)
			if shouldRetry(resp.StatusCode) {
				// Preserve response body for caller
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

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
		retry.RetryIf(isRetryableError),
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

// shouldRetry returns true if the HTTP status code indicates the request should be retried.
func shouldRetry(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || (statusCode >= 500 && statusCode < 600)
}

// isRetryableError determines if an error should trigger a retry.
func isRetryableError(err error) bool {
	_, ok := err.(*retryableError)
	return ok
}
