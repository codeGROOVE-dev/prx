// Package prx provides a client for fetching GitHub pull request events.
// It includes support for caching API responses to improve performance and
// reduce API rate limit consumption.
package prx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/codeGROOVE-dev/sfcache"
)

const (
	// HTTP client configuration constants.
	maxIdleConns        = 100
	maxIdleConnsPerHost = 10
	idleConnTimeoutSec  = 90
)

// Client provides methods to fetch GitHub pull request events.
type Client struct {
	github interface {
		get(ctx context.Context, path string, v any) (*githubResponse, error)
		raw(ctx context.Context, path string) (json.RawMessage, *githubResponse, error)
		collaborators(ctx context.Context, owner, repo string) (map[string]string, error)
	}
	logger             *slog.Logger
	token              string // Store token for recreating client with new transport
	collaboratorsCache *sfcache.MemoryCache[string, map[string]string]
	cacheDir           string // empty if caching is disabled
}

// Option is a function that configures a Client.
type Option func(*Client)

// WithLogger sets a custom logger for the client.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		// Wrap the transport with retry logic if not already wrapped
		if httpClient.Transport == nil {
			httpClient.Transport = &RetryTransport{Base: http.DefaultTransport}
		} else if _, ok := httpClient.Transport.(*RetryTransport); !ok {
			httpClient.Transport = &RetryTransport{Base: httpClient.Transport}
		}
		c.github = &githubClient{client: httpClient, token: c.token, api: githubAPI}
	}
}

// WithNoCache disables disk-based caching.
func WithNoCache() Option {
	return func(c *Client) {
		c.cacheDir = ""
	}
}

// NewClient creates a new Client with the given GitHub token.
// Caching is enabled by default - use WithNoCache() to disable.
// If token is empty, WithHTTPClient option must be provided.
func NewClient(token string, opts ...Option) *Client {
	transport := &http.Transport{
		MaxIdleConns:        maxIdleConns,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		IdleConnTimeout:     idleConnTimeoutSec * time.Second,
		DisableCompression:  false,
		DisableKeepAlives:   false,
	}
	// Set up default cache directory
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		userCacheDir = os.TempDir()
	}
	defaultCacheDir := filepath.Join(userCacheDir, "prx")
	c := &Client{
		logger:             slog.Default(),
		token:              token,
		cacheDir:           defaultCacheDir, // Enable caching by default
		collaboratorsCache: sfcache.New[string, map[string]string](sfcache.TTL(collaboratorsCacheTTL)),
		github: &githubClient{
			client: &http.Client{
				Transport: &RetryTransport{Base: transport},
				Timeout:   30 * time.Second,
			},
			token: token,
			api:   githubAPI,
		},
	}

	for _, opt := range opts {
		opt(c)
	}
	return c
}

// PullRequest fetches a pull request with all its events and metadata.
func (c *Client) PullRequest(ctx context.Context, owner, repo string, prNumber int) (*PullRequestData, error) {
	return c.PullRequestWithReferenceTime(ctx, owner, repo, prNumber, time.Now())
}

// PullRequestWithReferenceTime fetches a pull request using the given reference time for caching decisions.
func (c *Client) PullRequestWithReferenceTime(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	referenceTime time.Time,
) (*PullRequestData, error) {
	if c.cacheDir != "" {
		cache, err := newCache(c.cacheDir, c.logger)
		if err != nil {
			c.logger.WarnContext(ctx, "failed to create cache, proceeding without caching", "error", err)
			return c.pullRequestViaGraphQL(ctx, owner, repo, prNumber)
		}
		defer func() {
			if closeErr := cache.close(); closeErr != nil {
				c.logger.WarnContext(ctx, "failed to close cache", "error", closeErr)
			}
		}()
		cacheClient := &CacheClient{Client: c, cache: cache}
		return cacheClient.PullRequestWithReferenceTime(ctx, owner, repo, prNumber, referenceTime)
	}
	return c.pullRequestViaGraphQL(ctx, owner, repo, prNumber)
}
