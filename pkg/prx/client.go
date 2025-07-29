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
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Client provides methods to fetch GitHub pull request events.
type Client struct {
	github          interface {
		get(ctx context.Context, path string, v any) (*githubResponse, error)
		raw(ctx context.Context, path string) (json.RawMessage, *githubResponse, error)
		userPermission(ctx context.Context, owner, repo, username string) (string, error)
	}
	logger          *slog.Logger
	token           string // Store token for recreating client with new transport
	permissionCache *permissionCache
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

// writeAccess returns the write access level for a user
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
		perm, _ := c.userPermissionCached(ctx, owner, repo, user.Login, association)
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
		c.github = newGithubClient(httpClient, c.token)
	}
}

// NewClient creates a new Client with the given GitHub token.
// If token is empty, WithHTTPClient option must be provided.
func NewClient(token string, opts ...Option) *Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		DisableKeepAlives:   false,
	}

	c := &Client{
		logger: slog.Default(),
		token:  token,
		github: newGithubClient(&http.Client{
			Transport: &RetryTransport{Base: transport},
			Timeout:   30 * time.Second,
		}, token),
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

// userPermissionCached checks user permissions with caching.
func (c *Client) userPermissionCached(ctx context.Context, owner, repo, username, authorAssociation string) (string, error) {
	// Check cache first
	if perm, found := c.permissionCache.get(owner, repo, username); found {
		c.logger.Info("permission cache hit", "owner", owner, "repo", repo, "user", username, "permission", perm)
		return perm, nil
	}

	// Not in cache, fetch from API
	c.logger.Info("permission cache miss - checking user permissions via API",
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
			c.logger.Info("permission check failed with 403, assuming write access for MEMBER",
				"owner", owner,
				"repo", repo,
				"user", username,
				"author_association", authorAssociation,
				"error", apiErr.Body)
			// For MEMBER association with 403 error, we can't determine permissions
			// Return "uncertain" to indicate we don't know
			if cacheErr := c.permissionCache.set(owner, repo, username, "uncertain"); cacheErr != nil {
				c.logger.Warn("failed to cache permission", "error", cacheErr)
			}
			return "uncertain", nil // Can't determine access for MEMBER when API returns 403
		}
		return perm, err
	}

	// Cache the result
	if err := c.permissionCache.set(owner, repo, username, perm); err != nil {
		// Log error but don't fail the request
		c.logger.Warn("failed to cache permission", "error", err)
	}

	return perm, nil
}

// PullRequest fetches a pull request with all its events and metadata.
func (c *Client) PullRequest(ctx context.Context, owner, repo string, prNumber int) (*PullRequestData, error) {
	c.logger.Info("fetching pull request",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
	)

	var events []Event

	// Fetch the pull request to get basic info
	var pr githubPullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	if _, err := c.github.get(ctx, path, &pr); err != nil {
		c.logger.Error("failed to fetch pull request", "error", err)
		return nil, fmt.Errorf("fetching pull request: %w", err)
	}

	c.logger.Info("pull request metadata",
		"mergeable", pr.Mergeable,
		"mergeable_state", pr.MergeableState,
		"draft", pr.Draft,
		"additions", pr.Additions,
		"deletions", pr.Deletions,
		"changed_files", pr.ChangedFiles,
		"pr", prNumber)

	pullRequest := PullRequest{
		Number:         pr.Number,
		Title:          pr.Title,
		Body:           truncate(pr.Body, 256),
		State:          pr.State,
		Draft:          pr.Draft,
		Merged:         pr.Merged,
		Mergeable:      pr.Mergeable,
		MergeableState: pr.MergeableState,
		CreatedAt:      pr.CreatedAt,
		UpdatedAt:      pr.UpdatedAt,
		Author:         pr.User.Login,
		AuthorBot:      isBot(pr.User),
		Additions:      pr.Additions,
		Deletions:      pr.Deletions,
		ChangedFiles:   pr.ChangedFiles,
	}

	// Check if PR author has write access
	if pr.User != nil {
		writeAccess := c.writeAccess(ctx, owner, repo, pr.User, pr.AuthorAssociation)
		// For AuthorHasWriteAccess bool field, treat Likely and Definitely as true
		pullRequest.AuthorHasWriteAccess = (writeAccess > WriteAccessNA)
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

	prOpenedEvent := Event{
		Kind:        "pr_opened",
		Timestamp:   pr.CreatedAt,
		Actor:       pr.User.Login,
		Bot:         isBot(pr.User),
		WriteAccess: c.writeAccess(ctx, owner, repo, pr.User, pr.AuthorAssociation),
	}
	events = append(events, prOpenedEvent)

	g, gctx := errgroup.WithContext(ctx)
	var mu sync.Mutex
	var errors []error

	// Helper to run fetch functions in parallel
	fetch := func(name string, fn func(context.Context, string, string, int) ([]Event, error)) {
		g.Go(func() error {
			e, err := fn(gctx, owner, repo, prNumber)
			if err != nil {
				c.logger.Error("failed to fetch "+name, "error", err)
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
				return nil
			}
			mu.Lock()
			events = append(events, e...)
			mu.Unlock()
			return nil
		})
	}

	// Fetch all data in parallel
	fetch("commits", c.commits)

	fetch("comments", c.comments)

	fetch("reviews", c.reviews)

	fetch("review comments", c.reviewComments)

	fetch("timeline events", c.timelineEvents)

	// Fetch status checks
	g.Go(func() error {
		e, err := c.statusChecks(gctx, owner, repo, &pr)
		if err != nil {
			c.logger.Error("failed to fetch status checks", "error", err)
			mu.Lock()
			errors = append(errors, err)
			mu.Unlock()
			return nil
		}
		mu.Lock()
		events = append(events, e...)
		mu.Unlock()
		return nil
	})

	// Fetch check runs
	g.Go(func() error {
		e, err := c.checkRuns(gctx, owner, repo, &pr)
		if err != nil {
			c.logger.Error("failed to fetch check runs", "error", err)
			mu.Lock()
			errors = append(errors, err)
			mu.Unlock()
			return nil
		}
		mu.Lock()
		events = append(events, e...)
		mu.Unlock()
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if len(events) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("failed to fetch any events: %w", errors[0])
	}

	if pr.Merged {
		events = append(events, Event{
			Kind:      "pr_merged",
			Timestamp: pr.MergedAt,
			Actor:     pr.MergedBy.Login,
			Bot:       isBot(pr.MergedBy),
		})
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

	sortEventsByTimestamp(events)

	// Upgrade write_access from likely (1) to definitely (2) for actors who performed write-access-requiring actions
	upgradeWriteAccess(events)

	testSummary := calculateTestSummary(events)
	if testSummary.Passing > 0 || testSummary.Failing > 0 || testSummary.Pending > 0 {
		pullRequest.TestSummary = testSummary
	}

	statusSummary := calculateStatusSummary(events)
	if statusSummary.Success > 0 || statusSummary.Failure > 0 || statusSummary.Pending > 0 || statusSummary.Neutral > 0 {
		pullRequest.StatusSummary = statusSummary
	}

	approvalSummary := calculateApprovalSummary(events)
	if approvalSummary.ApprovalsWithWriteAccess > 0 || approvalSummary.ApprovalsWithoutWriteAccess > 0 || approvalSummary.ChangesRequested > 0 {
		pullRequest.ApprovalSummary = approvalSummary
	}

	c.logger.Info("successfully fetched pull request",
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
