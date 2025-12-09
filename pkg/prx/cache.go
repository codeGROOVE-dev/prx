package prx

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/sfcache/pkg/store/localfs"
)

const (
	// prCacheTTL is how long PR data is cached.
	prCacheTTL = 20 * 24 * time.Hour // 20 days
	// collaboratorsCacheTTL is how long collaborators data is cached.
	collaboratorsCacheTTL = 4 * time.Hour
)

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

// CacheClient is an alias for Client, kept for backward compatibility.
//
// Deprecated: Use NewClient with WithCacheStore instead.
type CacheClient = Client

// NewCacheClient creates a client with a specific cache directory.
//
// Deprecated: Use NewClient with WithCacheStore instead.
func NewCacheClient(token, dir string, opts ...Option) (*Client, error) {
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
	opts = append(opts, WithCacheStore(store))
	return NewClient(token, opts...), nil
}

// Close releases cache resources.
func (c *Client) Close() error {
	if c.prCache != nil {
		return c.prCache.Close()
	}
	return nil
}
