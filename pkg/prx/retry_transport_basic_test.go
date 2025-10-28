package prx

import (
	"net/http"
	"testing"
)

func TestRetryableError_Error(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       string
	}{
		{
			name:       "bad gateway",
			statusCode: http.StatusBadGateway,
			want:       "Bad Gateway",
		},
		{
			name:       "service unavailable",
			statusCode: http.StatusServiceUnavailable,
			want:       "Service Unavailable",
		},
		{
			name:       "gateway timeout",
			statusCode: http.StatusGatewayTimeout,
			want:       "Gateway Timeout",
		},
		{
			name:       "too many requests",
			statusCode: http.StatusTooManyRequests,
			want:       "Too Many Requests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &retryableError{StatusCode: tt.statusCode}
			if got := err.Error(); got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}
