// Package prx provides a client for fetching GitHub pull request events.
// It includes support for caching API responses to improve performance and
// reduce API rate limit consumption.
package prx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// HTTP client configuration constants.
	maxIdleConns        = 100
	maxIdleConnsPerHost = 10
	idleConnTimeoutSec  = 90
	// Concurrency constants.
	maxConcurrentRequests = 8
	// numFetchGoroutines is the actual number of goroutines launched for fetching PR data.
	numFetchGoroutines = 7 // commits, comments, reviews, review comments, timeline events, status checks, check runs
)

// Client provides methods to fetch GitHub pull request events.
type Client struct {
	github interface {
		get(ctx context.Context, path string, v any) (*githubResponse, error)
		raw(ctx context.Context, path string) (json.RawMessage, *githubResponse, error)
		userPermission(ctx context.Context, owner, repo, username string) (string, error)
	}
	logger          *slog.Logger
	token           string // Store token for recreating client with new transport
	permissionCache *permissionCache
	cacheDir        string // empty if caching is disabled
}

// isBot returns true if the user appears to be a bot.
func isBot(user *githubUser) bool {
	if user == nil {
		return false
	}
	return user.Type == "Bot" ||
		strings.HasSuffix(user.Login, "-bot") ||
		strings.HasSuffix(user.Login, "[bot]") ||
		strings.HasSuffix(user.Login, "-robot")
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
		logger:   slog.Default(),
		token:    token,
		cacheDir: defaultCacheDir, // Enable caching by default
		github: &githubClient{
			client: &http.Client{
				Transport: &RetryTransport{Base: transport},
				Timeout:   30 * time.Second,
			},
			token: token,
			api:   githubAPI,
		},
	}
	// Initialize in-memory permission cache (no disk persistence for regular client)
	c.permissionCache = &permissionCache{
		memory: make(map[string]permissionEntry),
		// diskPath is empty, so it won't persist to disk
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// writeAccess returns the write access level for a user.
func (c *Client) writeAccess(ctx context.Context, owner, repo string, user *githubUser, association string) int {
	if user == nil {
		return WriteAccessNA
	}
	// Check association-based access
	switch association {
	case "OWNER", "COLLABORATOR":
		return WriteAccessDefinitely
	case "MEMBER":
		// Need to check via API
		perm, err := c.userPermissionCached(ctx, owner, repo, user.Login, association)
		if err != nil {
			c.logger.DebugContext(ctx, "failed to get user permission", "error", err, "user", user.Login)
			return WriteAccessUnlikely
		}
		if perm == "uncertain" {
			return WriteAccessLikely
		}
		if perm == "admin" || perm == "write" {
			return WriteAccessDefinitely
		}
		return WriteAccessUnlikely
	case "CONTRIBUTOR", "NONE":
		return WriteAccessUnlikely
	default:
		return WriteAccessNA
	}
}

// userPermissionCached checks user permissions with caching.
func (c *Client) userPermissionCached(ctx context.Context, owner, repo, username, authorAssociation string) (string, error) {
	// Check cache first
	if perm, found := c.permissionCache.get(owner, repo, username); found {
		c.logger.InfoContext(ctx, "permission cache hit", "owner", owner, "repo", repo, "user", username, "permission", perm)
		return perm, nil
	}
	// Not in cache, fetch from API
	c.logger.InfoContext(ctx, "permission cache miss - checking user permissions via API",
		"owner", owner,
		"repo", repo,
		"user", username,
		"author_association", authorAssociation,
		"reason", "not in cache")
	perm, err := c.github.userPermission(ctx, owner, repo, username)
	if err != nil {
		// Check if this is a 403 error (no permission to check)
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
			c.logger.InfoContext(ctx, "permission check failed with 403, assuming write access for MEMBER",
				"owner", owner,
				"repo", repo,
				"user", username,
				"author_association", authorAssociation,
				"error", apiErr.Body)
			// For MEMBER association with 403 error, we can't determine permissions
			// Return "uncertain" to indicate we don't know
			if cacheErr := c.permissionCache.set(owner, repo, username, "uncertain"); cacheErr != nil {
				c.logger.WarnContext(ctx, "failed to cache permission", "error", cacheErr)
			}
			return "uncertain", nil // Can't determine access for MEMBER when API returns 403
		}
		return perm, err
	}
	// Cache the result
	if err := c.permissionCache.set(owner, repo, username, perm); err != nil {
		// Log error but don't fail the request
		c.logger.WarnContext(ctx, "failed to cache permission", "error", err)
	}
	return perm, nil
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
		return c.pullRequestWithCache(ctx, owner, repo, prNumber, referenceTime)
	}
	return c.pullRequestNonCached(ctx, owner, repo, prNumber)
}

func (c *Client) pullRequestWithCache(ctx context.Context, owner, repo string, prNumber int, referenceTime time.Time) (*PullRequestData, error) {
	// Create temporary CacheClient to use cached fetchers
	cacheClient := &CacheClient{
		Client:   c,
		cacheDir: c.cacheDir,
	}
	c.logger.InfoContext(ctx, "fetching pull request with cache",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
		"reference_time", referenceTime,
	)
	pr, err := cacheClient.cachedPullRequest(ctx, owner, repo, prNumber, referenceTime)
	if err != nil {
		return nil, fmt.Errorf("fetching pull request: %w", err)
	}
	var events []Event
	pullRequest := c.buildPullRequestWithMetadata(ctx, pr, owner, repo)
	prOpenedEvent := Event{
		Kind:        "pr_opened",
		Timestamp:   pr.CreatedAt,
		Actor:       pr.User.Login,
		Bot:         isBot(pr.User),
		WriteAccess: c.writeAccess(ctx, owner, repo, pr.User, pr.AuthorAssociation),
	}
	events = append(events, prOpenedEvent)
	prUpdatedAt := pr.UpdatedAt
	// Fetch all event types in parallel using cached fetchers
	results := make(chan fetchResult, maxConcurrentRequests)
	go func() {
		e, err := cacheClient.cachedCommits(ctx, owner, repo, prNumber, prUpdatedAt)
		results <- fetchResult{events: e, err: err, name: "commits", testState: ""}
	}()
	go func() {
		e, err := cacheClient.cachedComments(ctx, owner, repo, prNumber, prUpdatedAt)
		results <- fetchResult{events: e, err: err, name: "comments", testState: ""}
	}()
	go func() {
		e, err := cacheClient.cachedReviews(ctx, owner, repo, prNumber, prUpdatedAt)
		results <- fetchResult{events: e, err: err, name: "reviews", testState: ""}
	}()
	go func() {
		e, err := cacheClient.cachedReviewComments(ctx, owner, repo, prNumber, prUpdatedAt)
		results <- fetchResult{events: e, err: err, name: "review comments", testState: ""}
	}()
	go func() {
		e, err := cacheClient.cachedTimelineEvents(ctx, owner, repo, prNumber, prUpdatedAt)
		results <- fetchResult{events: e, err: err, name: "timeline events", testState: ""}
	}()
	// First, fetch required checks synchronously since other calls depend on it
	requiredChecks := c.requiredStatusChecks(ctx, owner, repo, pr)
	go func() {
		e, err := cacheClient.cachedStatusChecks(ctx, owner, repo, pr, prUpdatedAt, requiredChecks)
		results <- fetchResult{events: e, err: err, name: "status checks", testState: ""}
	}()
	go func() {
		e, testState, err := cacheClient.cachedCheckRuns(ctx, owner, repo, pr, prUpdatedAt, requiredChecks)
		results <- fetchResult{events: e, err: err, name: "check runs", testState: testState}
	}()
	// Collect results
	var errs []error
	var testStateFromAPI string
	numGoroutines := numFetchGoroutines
	for range numGoroutines {
		r := <-results
		if r.err != nil {
			c.logger.ErrorContext(ctx, "failed to fetch "+r.name, "error", r.err)
			errs = append(errs, r.err)
		} else {
			events = append(events, r.events...)
			// Capture test state from check runs
			if r.name == "check runs" && r.testState != "" {
				testStateFromAPI = r.testState
			}
		}
	}
	// If we have no events at all and errors occurred, return the first error
	if len(events) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("failed to fetch any events: %w", errs[0])
	}
	// Log a warning if we had partial failures
	if len(errs) > 0 {
		c.logger.WarnContext(ctx, "some event fetches failed but returning partial data",
			"error_count", len(errs),
			"event_count", len(events))
	}
	if pr.Merged {
		mergedEvent := Event{
			Kind:      "pr_merged",
			Timestamp: pr.MergedAt,
		}
		if pr.MergedBy != nil {
			mergedEvent.Actor = pr.MergedBy.Login
			mergedEvent.Bot = isBot(pr.MergedBy)
		} else {
			mergedEvent.Actor = "unknown"
		}
		events = append(events, mergedEvent)
	} else if pr.State == "closed" {
		closedEvent := Event{
			Kind:        "pr_closed",
			Timestamp:   pr.ClosedAt,
			Actor:       pr.User.Login,
			Bot:         isBot(pr.User),
			WriteAccess: c.writeAccess(ctx, owner, repo, pr.User, pr.AuthorAssociation),
		}
		events = append(events, closedEvent)
	}
	// Filter events to exclude non-failure status_check events
	events = filterEvents(events)
	// Sort events by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	// Upgrade write_access from likely (1) to definitely (2) for actors who performed write-access-requiring actions
	upgradeWriteAccess(events)
	// Use test state from API (calculated from current check run statuses)
	pullRequest.TestState = testStateFromAPI
	pullRequest.StatusSummary = calculateStatusSummary(events, requiredChecks)
	pullRequest.ApprovalSummary = calculateApprovalSummary(events)
	// Fix consistency issues between related fields
	// Fix test_state to be consistent with status_summary
	switch {
	case pullRequest.StatusSummary.Failure > 0:
		pullRequest.TestState = TestStateFailing
	case pullRequest.StatusSummary.Pending > 0:
		pullRequest.TestState = TestStatePending
	case pullRequest.StatusSummary.Success > 0:
		pullRequest.TestState = TestStatePassing
	default:
		pullRequest.TestState = TestStateNone
	}
	// Fix mergeable to be consistent with mergeable_state
	// If mergeable_state indicates blocked/dirty/unstable, mergeable should be false
	if pullRequest.MergeableState == "blocked" || pullRequest.MergeableState == "dirty" || pullRequest.MergeableState == "unstable" {
		falseVal := false
		pullRequest.Mergeable = &falseVal
	}
	// Add human-readable description for mergeable state
	switch pullRequest.MergeableState {
	case "blocked":
		// Determine what's actually blocking the PR
		hasApprovals := pullRequest.ApprovalSummary.ApprovalsWithWriteAccess > 0
		hasFailingChecks := pullRequest.StatusSummary.Failure > 0
		hasPendingChecks := pullRequest.StatusSummary.Pending > 0
		switch {
		case !hasApprovals && !hasFailingChecks:
			if hasPendingChecks {
				pullRequest.MergeableStateDescription = "PR requires approval and has pending status checks"
			} else {
				pullRequest.MergeableStateDescription = "PR requires approval"
			}
		case hasFailingChecks:
			if !hasApprovals {
				pullRequest.MergeableStateDescription = "PR has failing status checks and requires approval"
			} else {
				pullRequest.MergeableStateDescription = "PR is blocked by failing status checks"
			}
		case hasPendingChecks:
			pullRequest.MergeableStateDescription = "PR is blocked by pending status checks"
		default:
			pullRequest.MergeableStateDescription = "PR is blocked by required status checks, reviews, or branch protection rules"
		}
	case "dirty":
		pullRequest.MergeableStateDescription = "PR has merge conflicts that need to be resolved"
	case "unstable":
		pullRequest.MergeableStateDescription = "PR is mergeable but status checks are failing"
	case "clean":
		pullRequest.MergeableStateDescription = "PR is ready to merge"
	case "unknown":
		pullRequest.MergeableStateDescription = "Merge status is being calculated"
	case "draft":
		pullRequest.MergeableStateDescription = "PR is in draft state"
	default:
		pullRequest.MergeableStateDescription = ""
	}
	c.logger.InfoContext(ctx, "successfully fetched pull request with cache",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
		"event_count", len(events),
		"cache_hits", len(events)-len(errs),
	)
	return &PullRequestData{
		PullRequest: pullRequest,
		Events:      events,
	}, nil
}

func (c *Client) pullRequestNonCached(ctx context.Context, owner, repo string, prNumber int) (*PullRequestData, error) {
	c.logger.InfoContext(ctx, "fetching pull request",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
	)
	var events []Event
	// Fetch the pull request to get basic info
	var pr githubPullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	if _, err := c.github.get(ctx, path, &pr); err != nil {
		c.logger.ErrorContext(ctx, "failed to fetch pull request", "error", err)
		return nil, fmt.Errorf("fetching pull request: %w", err)
	}
	c.logger.InfoContext(ctx, "pull request metadata",
		"mergeable", pr.Mergeable,
		"mergeable_state", pr.MergeableState,
		"draft", pr.Draft,
		"additions", pr.Additions,
		"deletions", pr.Deletions,
		"changed_files", pr.ChangedFiles,
		"pr", prNumber)
	pullRequest := c.buildPullRequestWithMetadata(ctx, &pr, owner, repo)
	prOpenedEvent := c.createPROpenedEvent(ctx, &pr, owner, repo)
	events = append(events, prOpenedEvent)
	// Fetch all event types in parallel
	results := make(chan fetchResult, maxConcurrentRequests)
	go func() {
		e, err := c.commits(ctx, owner, repo, prNumber)
		results <- fetchResult{events: e, err: err, name: "commits", testState: ""}
	}()
	go func() {
		e, err := c.comments(ctx, owner, repo, prNumber)
		results <- fetchResult{events: e, err: err, name: "comments", testState: ""}
	}()
	go func() {
		e, err := c.reviews(ctx, owner, repo, prNumber)
		results <- fetchResult{events: e, err: err, name: "reviews", testState: ""}
	}()
	go func() {
		e, err := c.reviewComments(ctx, owner, repo, prNumber)
		results <- fetchResult{events: e, err: err, name: "review comments", testState: ""}
	}()
	go func() {
		e, err := c.timelineEvents(ctx, owner, repo, prNumber)
		results <- fetchResult{events: e, err: err, name: "timeline events", testState: ""}
	}()
	// First, fetch required checks synchronously since other calls depend on it
	requiredChecks := c.requiredStatusChecks(ctx, owner, repo, &pr)
	go func() {
		e, err := c.statusChecks(ctx, owner, repo, &pr, requiredChecks)
		results <- fetchResult{events: e, err: err, name: "status checks", testState: ""}
	}()
	go func() {
		e, testState, err := c.checkRuns(ctx, owner, repo, &pr, requiredChecks)
		results <- fetchResult{events: e, err: err, name: "check runs", testState: testState}
	}()
	// Collect results
	var errs []error
	var testStateFromAPI string
	numGoroutines := numFetchGoroutines
	for range numGoroutines {
		r := <-results
		if r.err != nil {
			c.logger.ErrorContext(ctx, "failed to fetch "+r.name, "error", r.err)
			errs = append(errs, r.err)
		} else {
			events = append(events, r.events...)
			// Capture test state from check runs
			if r.name == "check runs" && r.testState != "" {
				testStateFromAPI = r.testState
			}
		}
	}
	// If we have no events at all and errors occurred, return the first error
	if len(events) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("failed to fetch any events: %w", errs[0])
	}
	// Log a warning if we had partial failures
	if len(errs) > 0 {
		c.logger.WarnContext(ctx, "some event fetches failed but returning partial data",
			"error_count", len(errs),
			"event_count", len(events))
	}
	if pr.Merged {
		mergedEvent := Event{
			Kind:      "pr_merged",
			Timestamp: pr.MergedAt,
		}
		if pr.MergedBy != nil {
			mergedEvent.Actor = pr.MergedBy.Login
			mergedEvent.Bot = isBot(pr.MergedBy)
		} else {
			mergedEvent.Actor = "unknown"
		}
		events = append(events, mergedEvent)
	} else if pr.State == "closed" {
		closedEvent := Event{
			Kind:      "pr_closed",
			Timestamp: pr.ClosedAt,
		}
		if pr.User != nil {
			closedEvent.Actor = pr.User.Login
			closedEvent.Bot = isBot(pr.User)
			closedEvent.WriteAccess = c.writeAccess(ctx, owner, repo, pr.User, pr.AuthorAssociation)
		} else {
			closedEvent.Actor = "unknown"
			closedEvent.Bot = false
			closedEvent.WriteAccess = WriteAccessNA
		}
		events = append(events, closedEvent)
	}
	// Filter events to exclude non-failure status_check events
	events = filterEvents(events)
	// Sort events by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	// Upgrade write_access from likely (1) to definitely (2) for actors who performed write-access-requiring actions
	upgradeWriteAccess(events)
	// Use test state from API (calculated from current check run statuses)
	pullRequest.TestState = testStateFromAPI
	pullRequest.StatusSummary = calculateStatusSummary(events, requiredChecks)
	pullRequest.ApprovalSummary = calculateApprovalSummary(events)
	c.logger.InfoContext(ctx, "successfully fetched pull request",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
		"event_count", len(events),
	)
	return &PullRequestData{
		PullRequest: pullRequest,
		Events:      events,
	}, nil
}

// buildPullRequestWithMetadata creates a PullRequest object with all metadata populated.
func (c *Client) buildPullRequestWithMetadata(ctx context.Context, pr *githubPullRequest, owner, repo string) PullRequest {
	pullRequest := buildPullRequest(pr)
	// Check if PR author has write access
	if pr.User != nil {
		pullRequest.AuthorWriteAccess = c.writeAccess(ctx, owner, repo, pr.User, pr.AuthorAssociation)
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
	for _, assignee := range pr.Assignees {
		if assignee != nil {
			pullRequest.Assignees = append(pullRequest.Assignees, assignee.Login)
		}
	}
	for _, reviewer := range pr.RequestedReviewers {
		if reviewer != nil {
			pullRequest.RequestedReviewers = append(pullRequest.RequestedReviewers, reviewer.Login)
		}
	}
	for _, label := range pr.Labels {
		if label.Name != "" {
			pullRequest.Labels = append(pullRequest.Labels, label.Name)
		}
	}
	return pullRequest
}

// createPROpenedEvent creates the initial PR opened event.
func (c *Client) createPROpenedEvent(ctx context.Context, pr *githubPullRequest, owner, repo string) Event {
	prOpenedEvent := Event{
		Kind:      "pr_opened",
		Timestamp: pr.CreatedAt,
	}
	if pr.User != nil {
		prOpenedEvent.Actor = pr.User.Login
		prOpenedEvent.Bot = isBot(pr.User)
		prOpenedEvent.WriteAccess = c.writeAccess(ctx, owner, repo, pr.User, pr.AuthorAssociation)
	} else {
		prOpenedEvent.Actor = "unknown"
		prOpenedEvent.Bot = false
		prOpenedEvent.WriteAccess = WriteAccessNA
	}
	return prOpenedEvent
}
