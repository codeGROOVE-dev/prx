package prx

import (
	"net/http"

	"github.com/codeGROOVE-dev/prx/pkg/prx/github"
)

// newGitHubClient creates a new github.Client with the given configuration.
func newGitHubClient(httpClient *http.Client, token, baseURL string) *github.Client {
	return &github.Client{
		HTTPClient: httpClient,
		Token:      token,
		BaseURL:    baseURL,
	}
}

// newTestGitHubClient creates a github.Client for testing with custom HTTP client and base URL.
//
//nolint:unparam // token is always "test-token" in tests but should remain a parameter for flexibility
func newTestGitHubClient(httpClient *http.Client, token, baseURL string) *github.Client {
	return newGitHubClient(httpClient, token, baseURL)
}
