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

// cachedPullRequest fetches a pull request with caching.
func (c *CacheClient) cachedPullRequest(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) (*githubPullRequest, error) {
	cacheKey := c.cacheKey("pr", owner, repo, strconv.Itoa(prNumber))

	var cached cacheEntry
	if c.loadCache(ctx, cacheKey, &cached) {
		if cached.CachedAt.After(referenceTime) || cached.CachedAt.Equal(referenceTime) {
			var pr githubPullRequest
			err := json.Unmarshal(cached.Data, &pr)
			if err != nil {
				c.logger.WarnContext(ctx, "failed to unmarshal cached pull request", logFieldError, err)
				// Continue to fetch from API instead of returning error
				goto fetchFromAPI
			}

			c.logger.InfoContext(ctx, "cache hit: pull request",
				"owner", owner,
				"repo", repo,
				"pr", prNumber,
				"cached_at", cached.CachedAt)
			return &pr, nil
		}
		c.logger.InfoContext(ctx, "cache miss: pull request expired",
			"owner", owner,
			"repo", repo,
			"pr", prNumber,
			"cached_at", cached.CachedAt,
			"reference_time", referenceTime)
	} else {
		c.logger.InfoContext(ctx, "cache miss: pull request not in cache",
			"owner", owner,
			"repo", repo,
			"pr", prNumber)
	}

fetchFromAPI:
	// Fetch from API
	c.logger.InfoContext(ctx, "fetching pull request from GitHub API",
		"owner", owner,
		"repo", repo,
		"pr", prNumber)
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	rawData, _, err := c.github.raw(ctx, path)
	if err != nil {
		return nil, err
	}

	var pr githubPullRequest
	if err := json.Unmarshal(rawData, &pr); err != nil {
		return nil, fmt.Errorf("unmarshaling pull request: %w", err)
	}

	cached = cacheEntry{
		Data:      rawData,
		CachedAt:  time.Now(),
		UpdatedAt: pr.UpdatedAt,
	}
	if err := c.saveCache(ctx, cacheKey, cached); err != nil {
		c.logger.WarnContext(ctx, "failed to save pull request to cache", logFieldError, err)
	}

	return &pr, nil
}

// cachedFetch is a generic function for fetching data with caching support.
func (c *CacheClient) cachedFetch(ctx context.Context, dataType, path string, referenceTime time.Time) (json.RawMessage, error) {
	cacheKey := c.cacheKey(dataType, path)

	var cached cacheEntry
	if c.loadCache(ctx, cacheKey, &cached) {
		if cached.UpdatedAt.After(referenceTime) || cached.UpdatedAt.Equal(referenceTime) {
			c.logger.InfoContext(ctx, "cache hit", "type", dataType, "path", path, "cached_at", cached.CachedAt)
			return cached.Data, nil
		}
		c.logger.InfoContext(ctx, "cache miss: "+dataType+" expired", "cached_at", cached.UpdatedAt, "reference_time", referenceTime)
	} else {
		c.logger.InfoContext(ctx, "cache miss: "+dataType+" not found", "type", dataType, "path", path)
	}

	// Fetch from API
	c.logger.InfoContext(ctx, "fetching from GitHub API", "type", dataType, "path", path)
	rawData, _, err := c.github.raw(ctx, path)
	if err != nil {
		return nil, err
	}

	cached = cacheEntry{
		Data:      rawData,
		UpdatedAt: referenceTime,
		CachedAt:  time.Now(),
	}
	if err := c.saveCache(ctx, cacheKey, cached); err != nil {
		c.logger.WarnContext(ctx, "failed to save to cache", "type", dataType, logFieldError, err)
	}

	return rawData, nil
}

// cachedCommits fetches commits with caching.
func (c *CacheClient) cachedCommits(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	var allEvents []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/commits?page=%d&per_page=%d",
			owner, repo, prNumber, page, maxPerPage)

		rawData, err := c.cachedFetch(ctx, "commits", path, referenceTime)
		if err != nil {
			return nil, err
		}

		var commits []*githubPullRequestCommit
		if err := json.Unmarshal(rawData, &commits); err != nil {
			return nil, fmt.Errorf("unmarshaling commits: %w", err)
		}

		// Process commits into events
		for _, commit := range commits {
			event := Event{
				Kind:      "commit",
				Timestamp: commit.Commit.Author.Date,
				Body:      truncate(commit.Commit.Message),
			}

			if commit.Author != nil {
				event.Actor = commit.Author.Login
				event.Bot = isBot(commit.Author)
			} else {
				event.Actor = "unknown"
			}

			allEvents = append(allEvents, event)
		}

		// Check if there are more pages - if we got less than maxPerPage, we're done
		if len(commits) < maxPerPage {
			break
		}
		page++
	}

	return allEvents, nil
}

// cachedComments fetches comments with caching.
func (c *CacheClient) cachedComments(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	var allEvents []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?page=%d&per_page=%d",
			owner, repo, prNumber, page, maxPerPage)

		rawData, err := c.cachedFetch(ctx, "comments", path, referenceTime)
		if err != nil {
			return nil, err
		}

		var comments []*githubComment
		if err := json.Unmarshal(rawData, &comments); err != nil {
			return nil, fmt.Errorf("unmarshaling comments: %w", err)
		}

		for _, comment := range comments {
			event := createEvent("comment", comment.CreatedAt, comment.User, comment.Body)
			event.WriteAccess = c.writeAccess(ctx, owner, repo, comment.User, comment.AuthorAssociation)

			allEvents = append(allEvents, event)
		}

		if len(comments) < maxPerPage {
			break
		}
		page++
	}

	return allEvents, nil
}

// cachedReviews fetches reviews with caching.
func (c *CacheClient) cachedReviews(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	var allEvents []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews?page=%d&per_page=%d",
			owner, repo, prNumber, page, maxPerPage)

		rawData, err := c.cachedFetch(ctx, "reviews", path, referenceTime)
		if err != nil {
			return nil, err
		}

		var reviews []*githubReview
		if err := json.Unmarshal(rawData, &reviews); err != nil {
			return nil, fmt.Errorf("unmarshaling reviews: %w", err)
		}

		for _, review := range reviews {
			if review.State == "" {
				continue
			}

			c.logger.InfoContext(ctx, "processing review",
				"reviewer", review.User.Login,
				"author_association", review.AuthorAssociation,
				"state", review.State)

			event := createEvent("review", review.SubmittedAt, review.User, review.Body)
			event.Outcome = strings.ToLower(review.State) // Convert "APPROVED" -> "approved"
			event.WriteAccess = c.writeAccess(ctx, owner, repo, review.User, review.AuthorAssociation)

			allEvents = append(allEvents, event)
		}

		if len(reviews) < maxPerPage {
			break
		}
		page++
	}

	return allEvents, nil
}

// cachedReviewComments fetches review comments with caching.
func (c *CacheClient) cachedReviewComments(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	var allEvents []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments?page=%d&per_page=%d",
			owner, repo, prNumber, page, maxPerPage)

		rawData, err := c.cachedFetch(ctx, "review_comments", path, referenceTime)
		if err != nil {
			return nil, err
		}

		var comments []*githubReviewComment
		if err := json.Unmarshal(rawData, &comments); err != nil {
			return nil, fmt.Errorf("unmarshaling review comments: %w", err)
		}

		for _, comment := range comments {
			event := createEvent("review_comment", comment.CreatedAt, comment.User, comment.Body)
			event.WriteAccess = c.writeAccess(ctx, owner, repo, comment.User, comment.AuthorAssociation)

			allEvents = append(allEvents, event)
		}

		if len(comments) < maxPerPage {
			break
		}
		page++
	}

	return allEvents, nil
}

// cachedTimelineEvents fetches timeline events with caching.
func (c *CacheClient) cachedTimelineEvents(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) ([]Event, error) {
	var allEvents []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/issues/%d/timeline?page=%d&per_page=%d",
			owner, repo, prNumber, page, maxPerPage)

		rawData, err := c.cachedFetch(ctx, "timeline", path, referenceTime)
		if err != nil {
			return nil, err
		}

		var timelineEvents []*githubTimelineEvent
		if err := json.Unmarshal(rawData, &timelineEvents); err != nil {
			return nil, fmt.Errorf("unmarshaling timeline events: %w", err)
		}

		for _, te := range timelineEvents {
			var event Event
			switch te.Event {
			case "assigned", "unassigned":
				if te.Actor == nil || te.Assignee == nil {
					continue
				}
				event = Event{
					Kind:        te.Event,
					Timestamp:   te.CreatedAt,
					Actor:       te.Actor.Login,
					Bot:         isBot(te.Actor),
					Target:      te.Assignee.Login,
					TargetIsBot: isBot(te.Assignee),
				}
			case "review_requested", "review_request_removed":
				if te.Actor == nil {
					continue
				}
				if te.RequestedReviewer != nil { //nolint:gocritic // This checks different conditions, not suitable for switch
					event = Event{
						Kind:        te.Event,
						Timestamp:   te.CreatedAt,
						Actor:       te.Actor.Login,
						Bot:         isBot(te.Actor),
						Target:      te.RequestedReviewer.Login,
						TargetIsBot: isBot(te.RequestedReviewer),
					}
				} else if te.RequestedTeam.Name != "" {
					event = Event{
						Kind:      te.Event,
						Timestamp: te.CreatedAt,
						Actor:     te.Actor.Login,
						Bot:       isBot(te.Actor),
						Target:    te.RequestedTeam.Name,
					}
				} else {
					continue
				}
			case "labeled", "unlabeled":
				if te.Actor == nil || te.Label.Name == "" {
					continue
				}
				event = Event{
					Kind:      te.Event,
					Timestamp: te.CreatedAt,
					Actor:     te.Actor.Login,
					Bot:       isBot(te.Actor),
					Body:      te.Label.Name, // Store label name in Body field
				}
			case "mentioned":
				if te.Actor == nil {
					continue
				}
				event = Event{
					Kind:      te.Event,
					Timestamp: te.CreatedAt,
					Actor:     te.Actor.Login,
					Bot:       isBot(te.Actor),
				}
			case "convert_to_draft", "ready_for_review":
				if te.Actor == nil {
					continue
				}
				event = Event{
					Kind:      te.Event,
					Timestamp: te.CreatedAt,
					Actor:     te.Actor.Login,
					Bot:       isBot(te.Actor),
				}
			default:
				continue
			}

			allEvents = append(allEvents, event)
		}

		if len(timelineEvents) < maxPerPage {
			break
		}
		page++
	}

	return allEvents, nil
}

// cachedStatusChecks fetches status checks with caching.
func (c *CacheClient) cachedStatusChecks(ctx context.Context, owner, repo string, pr *githubPullRequest, referenceTime time.Time, requiredChecks []string) ([]Event, error) {
	var allEvents []Event
	page := 1

	// Create a set for quick lookup of required checks
	requiredSet := make(map[string]bool)
	for _, required := range requiredChecks {
		requiredSet[required] = true
	}

	for {
		path := fmt.Sprintf("/repos/%s/%s/statuses/%s?page=%d&per_page=%d",
			owner, repo, pr.Head.SHA, page, maxPerPage)

		rawData, err := c.cachedFetch(ctx, "statuses", path, referenceTime)
		if err != nil {
			return nil, err
		}

		var statuses []*githubStatus
		if err := json.Unmarshal(rawData, &statuses); err != nil {
			return nil, fmt.Errorf("unmarshaling statuses: %w", err)
		}

		for _, status := range statuses {
			event := Event{
				Kind:      "status_check",
				Timestamp: status.CreatedAt,
				Actor:     status.Creator.Login,
				Bot:       isBot(status.Creator),
				Body:      status.Context, // Store check name in Body
				Outcome:   status.State,   // Store state in Outcome
				Required:  requiredSet[status.Context],
			}
			// Include description if available
			if status.Description != "" {
				event.Body = event.Body + ": " + truncate(status.Description)
			}
			allEvents = append(allEvents, event)
		}

		if len(statuses) < maxPerPage {
			break
		}
		page++
	}

	return allEvents, nil
}

// cachedCheckRuns fetches check runs with caching.
func (c *CacheClient) cachedCheckRuns(
	ctx context.Context, owner, repo string, pr *githubPullRequest, referenceTime time.Time, requiredChecks []string,
) ([]Event, string, error) {
	var allEvents []Event
	page := 1

	// Create a set for quick lookup of required checks
	requiredSet := make(map[string]bool)
	for _, required := range requiredChecks {
		requiredSet[required] = true
	}

	// Track current states for test state calculation
	hasQueued := false
	hasRunning := false
	hasFailing := false
	hasPassing := false

	for {
		path := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs?page=%d&per_page=%d",
			owner, repo, pr.Head.SHA, page, maxPerPage)

		rawData, err := c.cachedFetch(ctx, "check_runs", path, referenceTime)
		if err != nil {
			return nil, TestStateNone, err
		}

		var response githubCheckRuns
		if err := json.Unmarshal(rawData, &response); err != nil {
			return nil, TestStateNone, fmt.Errorf("unmarshaling check runs: %w", err)
		}

		for _, run := range response.CheckRuns {
			var outcome string
			var timestamp time.Time
			if !run.CompletedAt.IsZero() { //nolint:gocritic // This checks time fields and different conditions, not suitable for switch
				// Test has completed
				outcome = run.Conclusion
				timestamp = run.CompletedAt

				// Track state for completed tests
				switch outcome {
				case "success":
					hasPassing = true
				case "failure", "timed_out", "action_required":
					hasFailing = true
				default:
					// Other conclusions like "neutral", "cancelled", "skipped" don't affect test state
				}
			} else if run.Status == "queued" {
				// Test is queued
				outcome = "queued"
				hasQueued = true
				timestamp = run.StartedAt
				// If we don't have a timestamp, we'll use zero time which will sort first
			} else if run.Status == "in_progress" {
				// Test is running
				outcome = "in_progress"
				hasRunning = true
				timestamp = run.StartedAt
				// If we don't have a timestamp, we'll use zero time which will sort first
			} else {
				// Unknown status, use what we have
				outcome = run.Status
				timestamp = run.StartedAt
				// If we don't have a timestamp, we'll use zero time which will sort first
			}

			event := Event{
				Kind:      "check_run",
				Timestamp: timestamp,
				Actor:     "github",
				Bot:       true,
				Outcome:   outcome,
				Body:      run.Name, // Store check run name in body field
				Required:  requiredSet[run.Name],
			}
			allEvents = append(allEvents, event)
		}

		if len(response.CheckRuns) < maxPerPage {
			break
		}
		page++
	}

	// Calculate overall test state based on current API data
	var testState string
	if hasFailing { //nolint:gocritic // This checks priority order of boolean flags, switch would be less readable
		testState = TestStateFailing
	} else if hasRunning {
		testState = TestStateRunning
	} else if hasQueued {
		testState = TestStateQueued
	} else if hasPassing {
		testState = TestStatePassing
	} else {
		testState = TestStateNone
	}

	return allEvents, testState, nil
}

func (*CacheClient) cacheKey(parts ...string) string {
	key := strings.Join(parts, "/")
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
