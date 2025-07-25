package prx

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

const (
	// cacheRetentionPeriod is how long cache files are kept before cleanup
	cacheRetentionPeriod = 20 * 24 * time.Hour // 20 days
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
	cleanPath := filepath.Clean(cacheDir)
	if !filepath.IsAbs(cleanPath) {
		return nil, fmt.Errorf("cache directory must be absolute path")
	}

	if err := os.MkdirAll(cleanPath, 0700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	client := NewClient(token, opts...)

	cc := &CacheClient{
		Client:   client,
		cacheDir: cleanPath,
	}

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

	pr, err := c.cachedPullRequest(ctx, owner, repo, prNumber, referenceTime)
	if err != nil {
		return nil, fmt.Errorf("fetching pull request: %w", err)
	}

	var events []Event

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

	events = append(events, Event{
		Kind:      PROpened,
		Timestamp: pr.CreatedAt,
		Actor:     pr.User.Login,
		Bot:       isBot(pr.User),
	})

	prUpdatedAt := pr.UpdatedAt

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

	if len(events) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("failed to fetch any events: %w", errors[0])
	}

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

	// Filter events to exclude non-failure status_check events
	events = filterEvents(events)

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

	var cached cachePullRequest
	if c.loadCache(cacheKey, &cached) {
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

	cached = cachePullRequest{
		PR:       pr,
		CachedAt: time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save pull request to cache", "error", err)
	}

	return &pr, nil
}

// cachedFetch is a generic function for fetching data with caching support.
func (c *CacheClient) cachedFetch(ctx context.Context, dataType, endpoint string, referenceTime time.Time, fetcher func() (interface{}, error)) (interface{}, error) {
	cacheKey := c.cacheKey(dataType, endpoint)

	var cached cacheEntry
	if c.loadCache(cacheKey, &cached) {
		if cached.UpdatedAt.After(referenceTime) || cached.UpdatedAt.Equal(referenceTime) {
			c.logger.Debug("cache hit", "type", dataType, "endpoint", endpoint, "cached_at", cached.CachedAt)
			c.logger.Debug("cached response", "type", dataType, "endpoint", endpoint, "response", string(cached.Data))
			return cached.Data, nil
		}
		c.logCacheMiss(dataType, "", "", 0, true, cached.UpdatedAt, referenceTime)
	} else {
		c.logCacheMiss(dataType, "", "", 0, false, time.Time{}, referenceTime)
	}

	// Fetch from API
	result, err := fetcher()
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(result)
	if err != nil {
		c.logger.Warn("failed to marshal for cache", "type", dataType, "error", err)
		return result, nil
	}

	cached = cacheEntry{
		Data:      data,
		UpdatedAt: referenceTime,
		CachedAt:  time.Now(),
	}
	if err := c.saveCache(cacheKey, cached); err != nil {
		c.logger.Warn("failed to save to cache", "type", dataType, "error", err)
	}

	return result, nil
}

func unmarshalCachedEvents(result interface{}, eventType string) ([]Event, error) {
	if data, ok := result.(json.RawMessage); ok {
		var events []Event
		if err := json.Unmarshal(data, &events); err != nil {
			return nil, fmt.Errorf("unmarshaling cached %s: %w", eventType, err)
		}
		return events, nil
	}
	return result.([]Event), nil
}

// cachedCommits fetches commits with caching.
func (c *CacheClient) cachedCommits(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/pulls/%d/commits", owner, repo, prNumber)
	result, err := c.cachedFetch(ctx, "commits", endpoint, referenceTime, func() (interface{}, error) {
		return c.commits(ctx, owner, repo, prNumber)
	})
	if err != nil {
		return nil, err
	}
	return unmarshalCachedEvents(result, "commits")
}

// cachedComments fetches comments with caching.
func (c *CacheClient) cachedComments(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/issues/%d/comments", owner, repo, prNumber)
	result, err := c.cachedFetch(ctx, "comments", endpoint, referenceTime, func() (interface{}, error) {
		return c.comments(ctx, owner, repo, prNumber)
	})
	if err != nil {
		return nil, err
	}
	return unmarshalCachedEvents(result, "comments")
}

// cachedReviews fetches reviews with caching.
func (c *CacheClient) cachedReviews(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/pulls/%d/reviews", owner, repo, prNumber)
	result, err := c.cachedFetch(ctx, "reviews", endpoint, referenceTime, func() (interface{}, error) {
		return c.reviews(ctx, owner, repo, prNumber)
	})
	if err != nil {
		return nil, err
	}
	return unmarshalCachedEvents(result, "reviews")
}

// cachedReviewComments fetches review comments with caching.
func (c *CacheClient) cachedReviewComments(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/pulls/%d/comments", owner, repo, prNumber)
	result, err := c.cachedFetch(ctx, "review_comments", endpoint, referenceTime, func() (interface{}, error) {
		return c.reviewComments(ctx, owner, repo, prNumber)
	})
	if err != nil {
		return nil, err
	}
	return unmarshalCachedEvents(result, "review comments")
}

// cachedTimelineEvents fetches timeline events with caching.
func (c *CacheClient) cachedTimelineEvents(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/issues/%d/timeline", owner, repo, prNumber)
	result, err := c.cachedFetch(ctx, "timeline", endpoint, referenceTime, func() (interface{}, error) {
		return c.timelineEvents(ctx, owner, repo, prNumber)
	})
	if err != nil {
		return nil, err
	}
	return unmarshalCachedEvents(result, "timeline events")
}

// cachedStatusChecks fetches status checks with caching.
func (c *CacheClient) cachedStatusChecks(ctx context.Context, owner, repo string, pr *githubPullRequest, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/statuses/%s", owner, repo, pr.Head.SHA)
	result, err := c.cachedFetch(ctx, "statuses", endpoint, referenceTime, func() (interface{}, error) {
		return c.statusChecks(ctx, owner, repo, pr)
	})
	if err != nil {
		return nil, err
	}
	return unmarshalCachedEvents(result, "status checks")
}

// cachedCheckRuns fetches check runs with caching.
func (c *CacheClient) cachedCheckRuns(ctx context.Context, owner, repo string, pr *githubPullRequest, referenceTime time.Time) ([]Event, error) {
	endpoint := fmt.Sprintf("%s/%s/check-runs/%s", owner, repo, pr.Head.SHA)
	result, err := c.cachedFetch(ctx, "check_runs", endpoint, referenceTime, func() (interface{}, error) {
		return c.checkRuns(ctx, owner, repo, pr)
	})
	if err != nil {
		return nil, err
	}
	return unmarshalCachedEvents(result, "check runs")
}

func (c *CacheClient) logCacheMiss(resourceType string, owner, repo string, prNumber int, cached bool, cachedAt, referenceTime time.Time) {
	if cached {
		c.logger.Debug("cache miss: "+resourceType+" expired",
			"owner", owner,
			"repo", repo,
			"pr", prNumber,
			"cached_at", cachedAt,
			"reference_time", referenceTime)
	} else {
		c.logger.Debug("cache miss: "+resourceType+" not found",
			"owner", owner,
			"repo", repo,
			"pr", prNumber)
	}
}

func (c *CacheClient) cacheKey(parts ...string) string {
	key := strings.Join(parts, "/")
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash)
}

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

func (c *CacheClient) saveCache(key string, v any) error {
	if len(key) != 64 || !isHexString(key) {
		return fmt.Errorf("invalid cache key format")
	}

	path := filepath.Join(c.cacheDir, key+".json")

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

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming cache file: %w", err)
	}

	return nil
}

func (c *CacheClient) cleanOldCaches() {
	c.logger.Debug("cleaning old cache files")

	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		c.logger.Error("failed to read cache directory", "error", err)
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
