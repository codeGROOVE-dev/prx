package prevents

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CacheClient wraps the regular Client and adds disk-based caching.
type CacheClient struct {
	*Client
	cacheDir string
}

// cacheEntry represents a cached API response.
type cacheEntry struct {
	Data      json.RawMessage `json:"data"`
	UpdatedAt time.Time       `json:"updated_at"`
	CachedAt  time.Time       `json:"cached_at"`
}

// cachePullRequest stores the pull request data with cache metadata.
type cachePullRequest struct {
	PR       githubPullRequest `json:"pr"`
	CachedAt time.Time         `json:"cached_at"`
}

// NewCacheClient creates a new caching client with the given cache directory.
func NewCacheClient(token string, cacheDir string, opts ...Option) (*CacheClient, error) {
	// Validate cache directory path to prevent directory traversal
	cleanPath := filepath.Clean(cacheDir)
	if !filepath.IsAbs(cleanPath) {
		return nil, fmt.Errorf("cache directory must be absolute path")
	}

	// Ensure cache directory exists with secure permissions
	if err := os.MkdirAll(cleanPath, 0700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	client := NewClient(token, opts...)

	cc := &CacheClient{
		Client:   client,
		cacheDir: cleanPath,
	}

	// Clean up old caches on creation
	go cc.cleanOldCaches()

	return cc, nil
}

// PullRequest fetches a pull request with all its events and metadata, with caching support.
func (c *CacheClient) PullRequest(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) (*PullRequestData, error) {
	c.logger.Info("fetching pull request with cache",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
		"reference_time", referenceTime,
	)

	// First, fetch the pull request itself
	pr, err := c.cachedPullRequest(ctx, owner, repo, prNumber, referenceTime)
	if err != nil {
		return nil, fmt.Errorf("fetching pull request: %w", err)
	}

	var events []Event

	// Build PullRequest metadata
	pullRequest := PullRequest{
		Number:            pr.Number,
		Title:             pr.Title,
		Body:              pr.Body,
		State:             pr.State,
		Draft:             pr.Draft,
		Merged:            pr.Merged,
		Mergeable:         pr.Mergeable,
		MergeableState:    pr.MergeableState,
		CreatedAt:         pr.CreatedAt,
		UpdatedAt:         pr.UpdatedAt,
		Author:            pr.User.Login,
		AuthorAssociation: pr.AuthorAssociation,
		AuthorBot:         isBot(pr.User),
		Additions:         pr.Additions,
		Deletions:         pr.Deletions,
		ChangedFiles:      pr.ChangedFiles,
	}

	if !pr.ClosedAt.IsZero() {
		pullRequest.ClosedAt = &pr.ClosedAt
	}
	if !pr.MergedAt.IsZero() {
		pullRequest.MergedAt = &pr.MergedAt
	}
	if pr.MergedBy != nil {
		pullRequest.MergedBy = pr.MergedBy.Login
	}

	// Add PR opened event
	events = append(events, Event{
		Kind:      PROpened,
		Timestamp: pr.CreatedAt,
		Actor:     pr.User.Login,
		Bot:       isBot(pr.User),
	})

	// For all other API calls, use the PR's updated_at timestamp as reference
	prUpdatedAt := pr.UpdatedAt

	// Fetch all event types using cached versions
	var errors []error

	// Commits
	commits, err := c.cachedCommits(ctx, owner, repo, prNumber, prUpdatedAt)
	if err != nil {
		c.logger.Error("failed to fetch commits", "error", err)
		errors = append(errors, err)
	} else {
		events = append(events, commits...)
	}

	// Comments
	comments, err := c.cachedComments(ctx, owner, repo, prNumber, prUpdatedAt)
	if err != nil {
		c.logger.Error("failed to fetch comments", "error", err)
		errors = append(errors, err)
	} else {
		events = append(events, comments...)
	}

	// Reviews
	reviews, err := c.cachedReviews(ctx, owner, repo, prNumber, prUpdatedAt)
	if err != nil {
		c.logger.Error("failed to fetch reviews", "error", err)
		errors = append(errors, err)
	} else {
		events = append(events, reviews...)
	}

	// Review comments
	reviewComments, err := c.cachedReviewComments(ctx, owner, repo, prNumber, prUpdatedAt)
	if err != nil {
		c.logger.Error("failed to fetch review comments", "error", err)
		errors = append(errors, err)
	} else {
		events = append(events, reviewComments...)
	}

	// Timeline events
	timelineEvents, err := c.cachedTimelineEvents(ctx, owner, repo, prNumber, prUpdatedAt)
	if err != nil {
		c.logger.Error("failed to fetch timeline events", "error", err)
		errors = append(errors, err)
	} else {
		events = append(events, timelineEvents...)
	}

	// Status checks
	statusChecks, err := c.cachedStatusChecks(ctx, owner, repo, pr, prUpdatedAt)
	if err != nil {
		c.logger.Error("failed to fetch status checks", "error", err)
		errors = append(errors, err)
	} else {
		events = append(events, statusChecks...)
	}

	// Check runs
	checkRuns, err := c.cachedCheckRuns(ctx, owner, repo, pr, prUpdatedAt)
	if err != nil {
		c.logger.Error("failed to fetch check runs", "error", err)
		errors = append(errors, err)
	} else {
		events = append(events, checkRuns...)
	}

	// Return error only if we have no events at all
	if len(events) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("failed to fetch any events: %w", errors[0])
	}

	// Add PR closed/merged event
	if pr.Merged {
		events = append(events, Event{
			Kind:      PRMerged,
			Timestamp: pr.MergedAt,
			Actor:     pr.MergedBy.Login,
			Bot:       isBot(pr.MergedBy),
		})
	} else if pr.State == "closed" {
		events = append(events, Event{
			Kind:      PRClosed,
			Timestamp: pr.ClosedAt,
			Actor:     pr.User.Login,
			Bot:       isBot(pr.User),
		})
	}

	// Sort events by timestamp
	sortEventsByTimestamp(events)

	c.logger.Info("successfully fetched pull request with cache",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
		"event_count", len(events),
		"cache_hits", len(events)-len(errors),
	)

	return &PullRequestData{
		PullRequest: pullRequest,
		Events:      events,
	}, nil
}

// cachedPullRequest fetches a pull request with caching.
func (c *CacheClient) cachedPullRequest(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) (*githubPullRequest, error) {
	cacheKey := c.cacheKey("pr", owner, repo, fmt.Sprintf("%d", prNumber))

	// Try to load from cache
	var cached cachePullRequest
	if c.loadCache(cacheKey, &cached) {
		// For the initial PR request, use referenceTime to determine validity
		if cached.CachedAt.After(referenceTime) || cached.CachedAt.Equal(referenceTime) {
			return &cached.PR, nil
		}
		c.logger.Info("cache miss: pull request expired",
			"owner", owner,
			"repo", repo,
			"pr", prNumber,
			"cached_at", cached.CachedAt,
			"reference_time", referenceTime)
	}

	// Fetch from API
	var pr githubPullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	if _, err := c.github.get(ctx, path, &pr); err != nil {
		return nil, err
	}

	// Save to cache
	cached = cachePullRequest{
		PR:       pr,
		CachedAt: time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save pull request to cache", "error", err)
	}

	return &pr, nil
}

// cachedCommits fetches commits with caching.
func (c *CacheClient) cachedCommits(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/pulls/%d/commits", owner, repo, prNumber)
	cacheKey := c.cacheKey("commits", endpoint)

	// Try to load from cache
	var cached cacheEntry
	if c.loadCache(cacheKey, &cached) {
		if cached.UpdatedAt.After(referenceTime) || cached.UpdatedAt.Equal(referenceTime) {
			var events []Event
			if err := json.Unmarshal(cached.Data, &events); err != nil {
				c.logger.Warn("failed to unmarshal cached commits", "error", err)
			} else {
				return events, nil
			}
		}
		c.logCacheMiss("commits", owner, repo, prNumber, true, cached.UpdatedAt, referenceTime)
	} else {
		c.logCacheMiss("commits", owner, repo, prNumber, false, time.Time{}, referenceTime)
	}

	// Fetch from API - log happens inside c.commits
	events, err := c.commits(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	// Save to cache
	data, err := json.Marshal(events)
	if err != nil {
		c.logger.Warn("failed to marshal commits for cache", "error", err)
		return events, nil
	}

	cached = cacheEntry{
		Data:      data,
		UpdatedAt: referenceTime,
		CachedAt:  time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save commits to cache", "error", err)
	}

	return events, nil
}

// cachedComments fetches comments with caching.
func (c *CacheClient) cachedComments(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/issues/%d/comments", owner, repo, prNumber)
	cacheKey := c.cacheKey("comments", endpoint)

	// Try to load from cache
	var cached cacheEntry
	if c.loadCache(cacheKey, &cached) {
		if cached.UpdatedAt.After(referenceTime) || cached.UpdatedAt.Equal(referenceTime) {
			var events []Event
			if err := json.Unmarshal(cached.Data, &events); err != nil {
				c.logger.Warn("failed to unmarshal cached comments", "error", err)
			} else {
				return events, nil
			}
		}
		c.logCacheMiss("comments", owner, repo, prNumber, true, cached.UpdatedAt, referenceTime)
	} else {
		c.logCacheMiss("comments", owner, repo, prNumber, false, time.Time{}, referenceTime)
	}

	// Fetch from API
	events, err := c.comments(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	// Save to cache
	data, err := json.Marshal(events)
	if err != nil {
		c.logger.Warn("failed to marshal comments for cache", "error", err)
		return events, nil
	}

	cached = cacheEntry{
		Data:      data,
		UpdatedAt: referenceTime,
		CachedAt:  time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save comments to cache", "error", err)
	}

	return events, nil
}

// cachedReviews fetches reviews with caching.
func (c *CacheClient) cachedReviews(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/pulls/%d/reviews", owner, repo, prNumber)
	cacheKey := c.cacheKey("reviews", endpoint)

	// Try to load from cache
	var cached cacheEntry
	if c.loadCache(cacheKey, &cached) {
		if cached.UpdatedAt.After(referenceTime) || cached.UpdatedAt.Equal(referenceTime) {
			var events []Event
			if err := json.Unmarshal(cached.Data, &events); err != nil {
				c.logger.Warn("failed to unmarshal cached reviews", "error", err)
			} else {
				return events, nil
			}
		}
		c.logCacheMiss("reviews", owner, repo, prNumber, true, cached.UpdatedAt, referenceTime)
	} else {
		c.logCacheMiss("reviews", owner, repo, prNumber, false, time.Time{}, referenceTime)
	}

	// Fetch from API
	events, err := c.reviews(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	// Save to cache
	data, err := json.Marshal(events)
	if err != nil {
		c.logger.Warn("failed to marshal reviews for cache", "error", err)
		return events, nil
	}

	cached = cacheEntry{
		Data:      data,
		UpdatedAt: referenceTime,
		CachedAt:  time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save reviews to cache", "error", err)
	}

	return events, nil
}

// cachedReviewComments fetches review comments with caching.
func (c *CacheClient) cachedReviewComments(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/pulls/%d/comments", owner, repo, prNumber)
	cacheKey := c.cacheKey("review_comments", endpoint)

	// Try to load from cache
	var cached cacheEntry
	if c.loadCache(cacheKey, &cached) {
		if cached.UpdatedAt.After(referenceTime) || cached.UpdatedAt.Equal(referenceTime) {
			var events []Event
			if err := json.Unmarshal(cached.Data, &events); err != nil {
				c.logger.Warn("failed to unmarshal cached review comments", "error", err)
			} else {
				return events, nil
			}
		}
		c.logCacheMiss("review comments", owner, repo, prNumber, true, cached.UpdatedAt, referenceTime)
	} else {
		c.logCacheMiss("review comments", owner, repo, prNumber, false, time.Time{}, referenceTime)
	}

	// Fetch from API
	events, err := c.reviewComments(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	// Save to cache
	data, err := json.Marshal(events)
	if err != nil {
		c.logger.Warn("failed to marshal review comments for cache", "error", err)
		return events, nil
	}

	cached = cacheEntry{
		Data:      data,
		UpdatedAt: referenceTime,
		CachedAt:  time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save review comments to cache", "error", err)
	}

	return events, nil
}

// cachedTimelineEvents fetches timeline events with caching.
func (c *CacheClient) cachedTimelineEvents(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/issues/%d/timeline", owner, repo, prNumber)
	cacheKey := c.cacheKey("timeline", endpoint)

	// Try to load from cache
	var cached cacheEntry
	if c.loadCache(cacheKey, &cached) {
		if cached.UpdatedAt.After(referenceTime) || cached.UpdatedAt.Equal(referenceTime) {
			var events []Event
			if err := json.Unmarshal(cached.Data, &events); err != nil {
				c.logger.Warn("failed to unmarshal cached timeline events", "error", err)
			} else {
				return events, nil
			}
		}
		c.logCacheMiss("timeline events", owner, repo, prNumber, true, cached.UpdatedAt, referenceTime)
	} else {
		c.logCacheMiss("timeline events", owner, repo, prNumber, false, time.Time{}, referenceTime)
	}

	// Fetch from API
	events, err := c.timelineEvents(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	// Save to cache
	data, err := json.Marshal(events)
	if err != nil {
		c.logger.Warn("failed to marshal timeline events for cache", "error", err)
		return events, nil
	}

	cached = cacheEntry{
		Data:      data,
		UpdatedAt: referenceTime,
		CachedAt:  time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save timeline events to cache", "error", err)
	}

	return events, nil
}

// cachedStatusChecks fetches status checks with caching.
func (c *CacheClient) cachedStatusChecks(ctx context.Context, owner, repo string, pr *githubPullRequest, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/statuses/%s", owner, repo, pr.Head.SHA)
	cacheKey := c.cacheKey("statuses", endpoint)

	// Try to load from cache
	var cached cacheEntry
	if c.loadCache(cacheKey, &cached) {
		if cached.UpdatedAt.After(referenceTime) || cached.UpdatedAt.Equal(referenceTime) {
			var events []Event
			if err := json.Unmarshal(cached.Data, &events); err != nil {
				c.logger.Warn("failed to unmarshal cached status checks", "error", err)
			} else {
				return events, nil
			}
		}
		c.logger.Info("cache miss: status checks expired",
			"owner", owner,
			"repo", repo,
			"sha", pr.Head.SHA,
			"cached_at", cached.UpdatedAt,
			"reference_time", referenceTime)
	} else {
		c.logger.Info("cache miss: status checks not found",
			"owner", owner,
			"repo", repo,
			"sha", pr.Head.SHA)
	}

	// Fetch from API
	events, err := c.statusChecks(ctx, owner, repo, pr)
	if err != nil {
		return nil, err
	}

	// Save to cache
	data, err := json.Marshal(events)
	if err != nil {
		c.logger.Warn("failed to marshal status checks for cache", "error", err)
		return events, nil
	}

	cached = cacheEntry{
		Data:      data,
		UpdatedAt: referenceTime,
		CachedAt:  time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save status checks to cache", "error", err)
	}

	return events, nil
}

// cachedCheckRuns fetches check runs with caching.
func (c *CacheClient) cachedCheckRuns(ctx context.Context, owner, repo string, pr *githubPullRequest, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/check-runs/%s", owner, repo, pr.Head.SHA)
	cacheKey := c.cacheKey("check_runs", endpoint)

	// Try to load from cache
	var cached cacheEntry
	if c.loadCache(cacheKey, &cached) {
		if cached.UpdatedAt.After(referenceTime) || cached.UpdatedAt.Equal(referenceTime) {
			var events []Event
			if err := json.Unmarshal(cached.Data, &events); err != nil {
				c.logger.Warn("failed to unmarshal cached check runs", "error", err)
			} else {
				return events, nil
			}
		}
		c.logger.Info("cache miss: check runs expired",
			"owner", owner,
			"repo", repo,
			"sha", pr.Head.SHA,
			"cached_at", cached.UpdatedAt,
			"reference_time", referenceTime)
	} else {
		c.logger.Info("cache miss: check runs not found",
			"owner", owner,
			"repo", repo,
			"sha", pr.Head.SHA)
	}

	// Fetch from API
	events, err := c.checkRuns(ctx, owner, repo, pr)
	if err != nil {
		return nil, err
	}

	// Save to cache
	data, err := json.Marshal(events)
	if err != nil {
		c.logger.Warn("failed to marshal check runs for cache", "error", err)
		return events, nil
	}

	cached = cacheEntry{
		Data:      data,
		UpdatedAt: referenceTime,
		CachedAt:  time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save check runs to cache", "error", err)
	}

	return events, nil
}

// logCacheMiss logs cache miss with appropriate context
func (c *CacheClient) logCacheMiss(resourceType string, owner, repo string, prNumber int, cached bool, cachedAt, referenceTime time.Time) {
	if cached {
		c.logger.Info("cache miss: "+resourceType+" expired",
			"owner", owner,
			"repo", repo,
			"pr", prNumber,
			"cached_at", cachedAt,
			"reference_time", referenceTime)
	} else {
		c.logger.Info("cache miss: "+resourceType+" not found",
			"owner", owner,
			"repo", repo,
			"pr", prNumber)
	}
}

// cacheKey generates a unique cache key for the given parameters.
func (c *CacheClient) cacheKey(parts ...string) string {
	// Use SHA256 to ensure consistent, safe filenames
	key := strings.Join(parts, "/")
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash)
}

// loadCache loads cached data from disk.
func (c *CacheClient) loadCache(key string, v any) bool {
	path := filepath.Join(c.cacheDir, key+".json")

	file, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			c.logger.Debug("failed to open cache file", "error", err, "path", path)
		}
		return false
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(v); err != nil {
		c.logger.Warn("failed to decode cache file", "error", err, "path", path)
		return false
	}

	return true
}

// saveCache saves data to disk cache.
func (c *CacheClient) saveCache(key string, v any) error {
	// Validate key format (should be hex string from SHA256)
	if len(key) != 64 || !isHexString(key) {
		return fmt.Errorf("invalid cache key format")
	}

	path := filepath.Join(c.cacheDir, key+".json")

	// Create temporary file with secure permissions
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating cache file: %w", err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("encoding cache data: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing cache file: %w", err)
	}

	// Atomically replace the old file
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming cache file: %w", err)
	}

	return nil
}

// cleanOldCaches removes cache files older than 28 days.
func (c *CacheClient) cleanOldCaches() {
	c.logger.Debug("cleaning old cache files")

	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		c.logger.Error("failed to read cache directory", "error", err)
		return
	}

	cutoff := time.Now().Add(-28 * 24 * time.Hour)
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
				c.logger.Warn("failed to remove old cache file", "path", path, "error", err)
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		c.logger.Info("cleaned old cache files", "removed", removed)
	}
}
