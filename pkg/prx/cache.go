package prx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/sfcache"
	"github.com/codeGROOVE-dev/sfcache/pkg/persist/localfs"
)

const (
	// prCacheTTL is how long PR data is cached.
	prCacheTTL = 20 * 24 * time.Hour // 20 days
	// collaboratorsCacheTTL is how long collaborators data is cached.
	collaboratorsCacheTTL = 4 * time.Hour
)

// prxCache provides tiered caching with disk persistence using sfcache.
type prxCache struct {
	pr     *sfcache.TieredCache[string, PullRequestData]
	logger *slog.Logger
}

// newCache creates a new cache with disk persistence at the given directory.
func newCache(cacheDir string, logger *slog.Logger) (*prxCache, error) {
	if logger == nil {
		logger = slog.Default()
	}

	cleanPath := filepath.Clean(cacheDir)
	if !filepath.IsAbs(cleanPath) {
		return nil, errors.New("cache directory must be absolute path")
	}

	if err := os.MkdirAll(cleanPath, 0o700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	prStore, err := localfs.New[string, PullRequestData]("prx-pr", cleanPath)
	if err != nil {
		return nil, fmt.Errorf("creating PR cache store: %w", err)
	}

	prCache, err := sfcache.NewTiered(prStore, sfcache.TTL(prCacheTTL))
	if err != nil {
		return nil, fmt.Errorf("creating PR cache: %w", err)
	}

	return &prxCache{
		pr:     prCache,
		logger: logger,
	}, nil
}

// close releases cache resources.
func (c *prxCache) close() error {
	if c.pr != nil {
		return c.pr.Close()
	}
	return nil
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

// CacheClient wraps the regular Client and adds disk-based caching.
type CacheClient struct {
	*Client

	cache *prxCache
}

// NewCacheClient creates a new caching client with the given cache directory.
func NewCacheClient(token string, cacheDir string, opts ...Option) (*CacheClient, error) {
	// Create a temporary logger to use during cache creation
	logger := slog.Default()

	cache, err := newCache(cacheDir, logger)
	if err != nil {
		return nil, err
	}

	// Create client with no cache since CacheClient handles caching
	opts = append(opts, WithNoCache())
	client := NewClient(token, opts...)

	// Update logger to the one from client options
	cache.logger = client.logger

	return &CacheClient{
		Client: client,
		cache:  cache,
	}, nil
}

// Close releases cache resources.
func (c *CacheClient) Close() error {
	if c.cache != nil {
		return c.cache.close()
	}
	return nil
}

// PullRequest fetches a pull request with all its events and metadata, with caching support.
func (c *CacheClient) PullRequest(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) (*PullRequestData, error) {
	return c.PullRequestWithReferenceTime(ctx, owner, repo, prNumber, referenceTime)
}

// PullRequestWithReferenceTime fetches PR data using GetSet for thundering herd protection.
func (c *CacheClient) PullRequestWithReferenceTime(
	ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time,
) (*PullRequestData, error) {
	if c.cache == nil || c.cache.pr == nil {
		return c.pullRequestViaGraphQL(ctx, owner, repo, prNumber)
	}

	cacheKey := prCacheKey(owner, repo, prNumber)

	// Try to get from cache first with time validation
	cached, found, err := c.cache.pr.Get(ctx, cacheKey)
	if err != nil {
		c.logger.WarnContext(ctx, "cache get error",
			"owner", owner,
			"repo", repo,
			"pr", prNumber,
			"error", err)
	}
	if found {
		// Check if cache entry is fresh enough (cached after reference time)
		// Note: sfcache handles TTL expiration; we check freshness against referenceTime
		if cached.CachedAt.After(referenceTime) || cached.CachedAt.Equal(referenceTime) {
			c.logger.InfoContext(ctx, "cache hit: GraphQL pull request",
				"owner", owner,
				"repo", repo,
				"pr", prNumber,
				"cached_at", cached.CachedAt)
			return &cached, nil
		}
		c.logger.InfoContext(ctx, "cache miss: GraphQL pull request expired",
			"owner", owner,
			"repo", repo,
			"pr", prNumber,
			"cached_at", cached.CachedAt,
			"reference_time", referenceTime)
		// Delete stale entry so GetSet will fetch fresh data
		if delErr := c.cache.pr.Delete(ctx, cacheKey); delErr != nil {
			c.logger.WarnContext(ctx, "failed to delete stale cache entry", "error", delErr)
		}
	} else {
		c.logger.InfoContext(ctx, "cache miss: GraphQL pull request not in cache",
			"owner", owner,
			"repo", repo,
			"pr", prNumber)
	}

	// Fetch from API using GetSet for thundering herd protection
	result, err := c.cache.pr.GetSet(ctx, cacheKey, func(ctx context.Context) (PullRequestData, error) {
		prData, err := c.pullRequestViaGraphQL(ctx, owner, repo, prNumber)
		if err != nil {
			return PullRequestData{}, err
		}
		prData.CachedAt = time.Now()
		return *prData, nil
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}
