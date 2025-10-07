package prx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// cacheRetentionPeriod is how long cache files are kept before cleanup.
	cacheRetentionPeriod = 20 * 24 * time.Hour // 20 days
	// cacheDirPerms is the permission for cache directories.
	cacheDirPerms = 0o700
	// cacheFilePerms is the permission for cache files.
	cacheFilePerms = 0o600

	// Log field constants.
	logFieldError = "error"
)

// CacheClient wraps the regular Client and adds disk-based caching.
type CacheClient struct {
	*Client

	cacheDir string
}

// cacheEntry represents a cached API response.
type cacheEntry struct {
	UpdatedAt time.Time       `json:"updated_at"`
	CachedAt  time.Time       `json:"cached_at"`
	Data      json.RawMessage `json:"data"`
}

// NewCacheClient creates a new caching client with the given cache directory.
func NewCacheClient(token string, cacheDir string, opts ...Option) (*CacheClient, error) {
	cleanPath := filepath.Clean(cacheDir)
	if !filepath.IsAbs(cleanPath) {
		return nil, errors.New("cache directory must be absolute path")
	}

	if err := os.MkdirAll(cleanPath, cacheDirPerms); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	// Create client with no cache since CacheClient handles caching
	opts = append(opts, WithNoCache())
	client := NewClient(token, opts...)

	// Initialize permission cache with disk persistence for CacheClient
	client.permissionCache = newPermissionCache(cleanPath)

	cc := &CacheClient{
		Client:   client,
		cacheDir: cleanPath,
	}

	// Schedule cleanup in background
	go cc.cleanOldCaches()

	return cc, nil
}

// PullRequest fetches a pull request with all its events and metadata, with caching support.
func (c *CacheClient) PullRequest(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) (*PullRequestData, error) {
	return c.PullRequestWithReferenceTime(ctx, owner, repo, prNumber, referenceTime)
}

// cachedPullRequestViaGraphQL fetches pull request data via GraphQL with caching support.
func (c *CacheClient) cachedPullRequestViaGraphQL(
	ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time,
) (*PullRequestData, error) {
	cacheKey := c.cacheKey("pr_graphql", owner, repo, strconv.Itoa(prNumber))

	// Try to load from cache
	var cached cacheEntry
	if c.loadCache(ctx, cacheKey, &cached) {
		// Cache is valid if it was cached at or after the reference time
		if cached.CachedAt.After(referenceTime) || cached.CachedAt.Equal(referenceTime) {
			var prData PullRequestData
			err := json.Unmarshal(cached.Data, &prData)
			if err != nil {
				c.logger.WarnContext(ctx, "failed to unmarshal cached GraphQL pull request", logFieldError, err)
				// Continue to fetch from API instead of returning error
				goto fetchFromAPI
			}

			c.logger.InfoContext(ctx, "cache hit: GraphQL pull request",
				"owner", owner,
				"repo", repo,
				"pr", prNumber,
				"cached_at", cached.CachedAt)
			return &prData, nil
		}
		c.logger.InfoContext(ctx, "cache miss: GraphQL pull request expired",
			"owner", owner,
			"repo", repo,
			"pr", prNumber,
			"cached_at", cached.CachedAt,
			"reference_time", referenceTime)
	} else {
		c.logger.InfoContext(ctx, "cache miss: GraphQL pull request not in cache",
			"owner", owner,
			"repo", repo,
			"pr", prNumber)
	}

fetchFromAPI:
	// Fetch from API
	prData, err := c.pullRequestViaGraphQL(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	// Cache the result
	data, err := json.Marshal(prData)
	if err != nil {
		c.logger.WarnContext(ctx, "failed to marshal GraphQL pull request for cache", logFieldError, err)
		return prData, nil
	}

	entry := cacheEntry{
		Data:     data,
		CachedAt: time.Now(),
	}

	err = c.saveCache(ctx, cacheKey, entry)
	if err != nil {
		c.logger.WarnContext(ctx, "failed to save GraphQL pull request to cache", logFieldError, err)
	}

	return prData, nil
}

func (*CacheClient) cacheKey(parts ...string) string {
	// Always use graphql mode now
	allParts := append([]string{"graphql"}, parts...)
	key := strings.Join(allParts, "/")
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

func (c *CacheClient) loadCache(ctx context.Context, key string, v any) bool {
	path := filepath.Join(c.cacheDir, key+".json")

	file, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			c.logger.DebugContext(ctx, "failed to open cache file", "error", err, "path", path)
		}
		return false
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			c.logger.DebugContext(ctx, "failed to close cache file", "error", closeErr, "path", path)
		}
	}()

	if err := json.NewDecoder(file).Decode(v); err != nil {
		c.logger.WarnContext(ctx, "failed to decode cache file", "error", err, "path", path)
		return false
	}

	return true
}

func (c *CacheClient) saveCache(ctx context.Context, key string, v any) error {
	path := filepath.Join(c.cacheDir, key+".json")

	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, cacheFilePerms)
	if err != nil {
		return fmt.Errorf("creating cache file: %w", err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			c.logger.DebugContext(ctx, "failed to close temp file", "error", closeErr, "path", tmpPath)
		}
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			c.logger.DebugContext(ctx, "failed to remove temp file", "error", removeErr, "path", tmpPath)
		}
		return fmt.Errorf("encoding cache data: %w", err)
	}

	if err := file.Close(); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			c.logger.DebugContext(ctx, "failed to remove temp file", "error", removeErr, "path", tmpPath)
		}
		return fmt.Errorf("closing cache file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			c.logger.DebugContext(ctx, "failed to remove temp file", "error", removeErr, "path", tmpPath)
		}
		return fmt.Errorf("renaming cache file: %w", err)
	}

	return nil
}

func (c *CacheClient) cleanOldCaches() {
	c.logger.DebugContext(context.Background(), "cleaning old cache files")

	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		c.logger.ErrorContext(context.Background(), "failed to read cache directory", "error", err)
		return
	}

	cutoff := time.Now().Add(-cacheRetentionPeriod)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(c.cacheDir, entry.Name())
			if err := os.Remove(path); err != nil {
				c.logger.WarnContext(context.Background(), "failed to remove old cache file", "path", path, "error", err)
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		c.logger.InfoContext(context.Background(), "cleaned old cache files", "removed", removed)
	}
}
