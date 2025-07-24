// Package prx provides a client for fetching GitHub pull request events.
// It includes support for caching API responses to improve performance and
// reduce API rate limit consumption.
package prx

import (
	"context"
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
	github githubAPIClient
	logger *slog.Logger
	token  string // Store token for recreating client with new transport
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
		c.github = newGithubClient(httpClient, c.token)
	}
}

// NewClient creates a new Client with the given GitHub token.
// If token is empty, WithHTTPClient option must be provided.
func NewClient(token string, opts ...Option) *Client {
	// Configure optimized transport with connection pooling
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

	for _, opt := range opts {
		opt(c)
	}

	return c
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

	// Log merge status for debugging
	c.logger.Info("pull request metadata",
		"mergeable", pr.Mergeable,
		"mergeable_state", pr.MergeableState,
		"draft", pr.Draft,
		"additions", pr.Additions,
		"deletions", pr.Deletions,
		"changed_files", pr.ChangedFiles,
		"pr", prNumber)

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

	// Fetch all event types concurrently for better performance
	g, gctx := errgroup.WithContext(ctx)
	var mu sync.Mutex
	var errors []error

	// Fetch commits
	g.Go(func() error {
		e, err := c.commits(gctx, owner, repo, prNumber)
		if err != nil {
			c.logger.Error("failed to fetch commits", "error", err)
			mu.Lock()
			errors = append(errors, err)
			mu.Unlock()
			return nil // Continue on error for graceful degradation
		}
		mu.Lock()
		events = append(events, e...)
		mu.Unlock()
		return nil
	})

	// Fetch comments
	g.Go(func() error {
		e, err := c.comments(gctx, owner, repo, prNumber)
		if err != nil {
			c.logger.Error("failed to fetch comments", "error", err)
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

	// Fetch reviews
	g.Go(func() error {
		e, err := c.reviews(gctx, owner, repo, prNumber)
		if err != nil {
			c.logger.Error("failed to fetch reviews", "error", err)
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

	// Fetch review comments
	g.Go(func() error {
		e, err := c.reviewComments(gctx, owner, repo, prNumber)
		if err != nil {
			c.logger.Error("failed to fetch review comments", "error", err)
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

	// Fetch timeline events
	g.Go(func() error {
		e, err := c.timelineEvents(gctx, owner, repo, prNumber)
		if err != nil {
			c.logger.Error("failed to fetch timeline events", "error", err)
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

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return nil, err
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
