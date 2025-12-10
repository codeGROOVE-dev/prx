// Package prx provides a client for fetching GitHub pull request events.
// It includes support for caching API responses to improve performance and
// reduce API rate limit consumption.
package prx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx/github"
	"github.com/codeGROOVE-dev/sfcache"
	"github.com/codeGROOVE-dev/sfcache/pkg/store/localfs"
)

const (
	// HTTP client configuration constants.
	maxIdleConns        = 100
	maxIdleConnsPerHost = 10
	idleConnTimeoutSec  = 90

	// Cache TTL constants.
	prCacheTTL            = 20 * 24 * time.Hour // 20 days
	collaboratorsCacheTTL = 4 * time.Hour
)

// PRStore is the interface for PR cache storage backends.
// This is an alias for sfcache.Store with the appropriate type parameters.
type PRStore = sfcache.Store[string, PullRequestData]

// Client provides methods to fetch GitHub pull request events.
type Client struct {
	github             *github.Client
	logger             *slog.Logger
	collaboratorsCache *sfcache.MemoryCache[string, map[string]string]
	prCache            *sfcache.TieredCache[string, PullRequestData]
	token              string // Store token for recreating client with new transport
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
			httpClient.Transport = &github.Transport{Base: http.DefaultTransport}
		} else if _, ok := httpClient.Transport.(*github.Transport); !ok {
			httpClient.Transport = &github.Transport{Base: httpClient.Transport}
		}
		c.github = newGitHubClient(httpClient, c.token, github.API)
	}
}

// WithCacheStore sets a custom cache store for PR data.
// Use null.New[string, prx.PullRequestData]() to disable persistence.
func WithCacheStore(store PRStore) Option {
	return func(c *Client) {
		prCache, err := sfcache.NewTiered(store, sfcache.TTL(prCacheTTL))
		if err != nil {
			c.logger.Warn("failed to create cache from store, using default", "error", err)
			return
		}
		c.prCache = prCache
	}
}

// NewClient creates a new Client with the given GitHub token.
// Caching is enabled by default with disk persistence.
// Use WithCacheStore to provide a custom store (including null.New() to disable persistence).
// If token is empty, WithHTTPClient option must be provided.
func NewClient(token string, opts ...Option) *Client {
	transport := &http.Transport{
		MaxIdleConns:        maxIdleConns,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		IdleConnTimeout:     idleConnTimeoutSec * time.Second,
		DisableCompression:  false,
		DisableKeepAlives:   false,
	}
	c := &Client{
		logger:             slog.Default(),
		token:              token,
		collaboratorsCache: sfcache.New[string, map[string]string](sfcache.TTL(collaboratorsCacheTTL)),
		github: newGitHubClient(
			&http.Client{
				Transport: &github.Transport{Base: transport},
				Timeout:   30 * time.Second,
			},
			token,
			github.API,
		),
	}

	for _, opt := range opts {
		opt(c)
	}

	// Set up default cache if none was configured via options
	if c.prCache == nil {
		c.prCache = createDefaultCache(c.logger)
	}

	return c
}

func createDefaultCache(log *slog.Logger) *sfcache.TieredCache[string, PullRequestData] {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "prx")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Warn("failed to create cache directory, caching disabled", "error", err)
		return nil
	}
	store, err := localfs.New[string, PullRequestData]("prx-pr", dir)
	if err != nil {
		log.Warn("failed to create cache store, caching disabled", "error", err)
		return nil
	}
	cache, err := sfcache.NewTiered(store, sfcache.TTL(prCacheTTL))
	if err != nil {
		log.Warn("failed to create cache, caching disabled", "error", err)
		return nil
	}
	return cache
}

// PullRequest fetches a pull request with all its events and metadata.
func (c *Client) PullRequest(ctx context.Context, owner, repo string, prNumber int) (*PullRequestData, error) {
	return c.PullRequestWithReferenceTime(ctx, owner, repo, prNumber, time.Now())
}

// PullRequestWithReferenceTime fetches a pull request using the given reference time for caching decisions.
func (c *Client) PullRequestWithReferenceTime(
	ctx context.Context,
	owner, repo string,
	pr int,
	refTime time.Time,
) (*PullRequestData, error) {
	if c.prCache == nil {
		return c.pullRequestViaGraphQL(ctx, owner, repo, pr)
	}

	key := prCacheKey(owner, repo, pr)

	if cached, found, err := c.prCache.Get(ctx, key); err != nil {
		c.logger.WarnContext(ctx, "cache get error", "error", err)
	} else if found {
		if !cached.CachedAt.Before(refTime) {
			c.logger.InfoContext(ctx, "cache hit: GraphQL pull request",
				"owner", owner, "repo", repo, "pr", pr, "cached_at", cached.CachedAt)
			return &cached, nil
		}
		c.logger.InfoContext(ctx, "cache miss: GraphQL pull request expired",
			"owner", owner, "repo", repo, "pr", pr,
			"cached_at", cached.CachedAt, "reference_time", refTime)
		if err := c.prCache.Delete(ctx, key); err != nil {
			c.logger.WarnContext(ctx, "failed to delete stale cache entry", "error", err)
		}
	} else {
		c.logger.InfoContext(ctx, "cache miss: GraphQL pull request not in cache",
			"owner", owner, "repo", repo, "pr", pr)
	}

	result, err := c.prCache.GetSet(ctx, key, func(ctx context.Context) (PullRequestData, error) {
		data, err := c.pullRequestViaGraphQL(ctx, owner, repo, pr)
		if err != nil {
			return PullRequestData{}, err
		}
		data.CachedAt = time.Now()
		return *data, nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Close releases cache resources.
func (c *Client) Close() error {
	if c.prCache != nil {
		return c.prCache.Close()
	}
	return nil
}

// NewCacheStore creates a cache store backed by the given directory.
// This is a convenience function for use with WithCacheStore.
func NewCacheStore(dir string) (PRStore, error) {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return nil, errors.New("cache directory must be absolute path")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}
	store, err := localfs.New[string, PullRequestData]("prx-pr", dir)
	if err != nil {
		return nil, fmt.Errorf("creating PR cache store: %w", err)
	}
	return store, nil
}

// prCacheKey generates a cache key for PR data.
func prCacheKey(owner, repo string, prNumber int) string {
	key := strings.Join([]string{"graphql", "pr_graphql", owner, repo, strconv.Itoa(prNumber)}, "/")
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// collaboratorsCacheKey generates a cache key for collaborators data.
func collaboratorsCacheKey(owner, repo string) string {
	return fmt.Sprintf("%s/%s", owner, repo)
}
