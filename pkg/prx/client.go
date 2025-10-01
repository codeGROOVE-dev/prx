// Package prx provides a client for fetching GitHub pull request events.
// It includes support for caching API responses to improve performance and
// reduce API rate limit consumption.
package prx

import (
	"context"
	"encoding/json"
	"errors"
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
	return c.pullRequestImpl(ctx, owner, repo, prNumber, &referenceTime)
}

func (c *Client) pullRequestNonCached(ctx context.Context, owner, repo string, prNumber int) (*PullRequestData, error) {
	return c.pullRequestImpl(ctx, owner, repo, prNumber, nil)
}

// pullRequestImpl is the unified implementation that handles both cached and non-cached requests.
func (c *Client) pullRequestImpl(ctx context.Context, owner, repo string, prNumber int, referenceTime *time.Time) (*PullRequestData, error) {
	// Check if caching is enabled
	if c.cacheDir != "" && referenceTime != nil {
		cacheClient := &CacheClient{Client: c, cacheDir: c.cacheDir}
		return cacheClient.cachedPullRequestViaGraphQL(ctx, owner, repo, prNumber, *referenceTime)
	}
	// Direct GraphQL call without caching
	return c.pullRequestViaGraphQL(ctx, owner, repo, prNumber)
}

// processEvents filters, sorts, and upgrades write access for events.
func processEvents(events []Event) []Event {
	events = filterEvents(events)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	upgradeWriteAccess(events)
	return events
}

// finalizePullRequest applies final calculations and consistency fixes.
func finalizePullRequest(pullRequest *PullRequest, events []Event, requiredChecks []string, testStateFromAPI string) {
	pullRequest.TestState = testStateFromAPI
	pullRequest.CheckSummary = calculateCheckSummary(events, requiredChecks)
	pullRequest.ApprovalSummary = calculateApprovalSummary(events)

	fixTestState(pullRequest)
	fixMergeable(pullRequest)
	setMergeableDescription(pullRequest)
}

// fixTestState ensures test_state is consistent with check_summary.
func fixTestState(pullRequest *PullRequest) {
	switch {
	case len(pullRequest.CheckSummary.Failing) > 0 || len(pullRequest.CheckSummary.Cancelled) > 0:
		pullRequest.TestState = TestStateFailing
	case len(pullRequest.CheckSummary.Pending) > 0:
		pullRequest.TestState = TestStatePending
	case len(pullRequest.CheckSummary.Success) > 0:
		pullRequest.TestState = TestStatePassing
	default:
		pullRequest.TestState = TestStateNone
	}
}

// fixMergeable ensures mergeable is consistent with mergeable_state.
func fixMergeable(pullRequest *PullRequest) {
	if pullRequest.MergeableState == "blocked" || pullRequest.MergeableState == "dirty" || pullRequest.MergeableState == "unstable" {
		falseVal := false
		pullRequest.Mergeable = &falseVal
	}
}

// setMergeableDescription adds human-readable description for mergeable state.
func setMergeableDescription(pullRequest *PullRequest) {
	switch pullRequest.MergeableState {
	case "blocked":
		setBlockedDescription(pullRequest)
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
}

// setBlockedDescription determines what's blocking the PR and sets appropriate description.
func setBlockedDescription(pullRequest *PullRequest) {
	hasApprovals := pullRequest.ApprovalSummary.ApprovalsWithWriteAccess > 0
	hasFailingChecks := len(pullRequest.CheckSummary.Failing) > 0 || len(pullRequest.CheckSummary.Cancelled) > 0
	hasPendingChecks := len(pullRequest.CheckSummary.Pending) > 0

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
}
