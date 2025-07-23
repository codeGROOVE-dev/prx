package prevents

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

// Client provides methods to fetch GitHub pull request events.
type Client struct {
	github *github.Client
	logger *slog.Logger
}

// isBot returns true if the user appears to be a bot.
func isBot(user *github.User) bool {
	if user == nil {
		return false
	}
	login := user.GetLogin()
	return user.GetType() == "Bot" ||
		strings.HasSuffix(login, "-bot") ||
		strings.HasSuffix(login, "[bot]") ||
		strings.HasSuffix(login, "-robot")
}

// Option is a function that configures a Client.
type Option func(*Client)

// WithLogger sets a custom logger for the client.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// NewClient creates a new Client with the given GitHub token.
func NewClient(token string, opts ...Option) *Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	
	// Wrap the transport with retry logic
	tc.Transport = &RetryTransport{Base: tc.Transport}
	
	c := &Client{
		github: github.NewClient(tc),
		logger: slog.Default(),
	}
	
	for _, opt := range opts {
		opt(c)
	}
	
	return c
}

// NewClientWithHTTP creates a new Client with a custom HTTP client.
func NewClientWithHTTP(httpClient *http.Client, opts ...Option) *Client {
	// Wrap the transport with retry logic if not already wrapped
	if httpClient.Transport == nil {
		httpClient.Transport = &RetryTransport{Base: http.DefaultTransport}
	} else if _, ok := httpClient.Transport.(*RetryTransport); !ok {
		httpClient.Transport = &RetryTransport{Base: httpClient.Transport}
	}
	
	c := &Client{
		github: github.NewClient(httpClient),
		logger: slog.Default(),
	}
	
	for _, opt := range opts {
		opt(c)
	}
	
	return c
}

// FetchPullRequestEvents fetches all events for a pull request and returns them in chronological order.
func (c *Client) FetchPullRequestEvents(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Info("fetching pull request events",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
	)
	
	var events []Event

	// Fetch the pull request to get basic info
	pr, _, err := c.github.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		c.logger.Error("failed to fetch pull request", "error", err)
		return nil, fmt.Errorf("fetching pull request: %w", err)
	}

	// Add PR opened event
	events = append(events, Event{
		Type:      EventTypePROpened,
		Timestamp: pr.GetCreatedAt().Time,
		Actor:     pr.GetUser().GetLogin(),
		Bot:       isBot(pr.GetUser()),
	})

	// Fetch all event types concurrently for better performance
	type result struct {
		events []Event
		err    error
	}
	
	results := make(chan result, 8)
	
	go func() {
		e, err := c.fetchCommits(ctx, owner, repo, prNumber)
		results <- result{e, err}
	}()
	
	go func() {
		e, err := c.fetchComments(ctx, owner, repo, prNumber)
		results <- result{e, err}
	}()
	
	go func() {
		e, err := c.fetchReviews(ctx, owner, repo, prNumber)
		results <- result{e, err}
	}()
	
	go func() {
		e, err := c.fetchReviewComments(ctx, owner, repo, prNumber)
		results <- result{e, err}
	}()
	
	go func() {
		e, err := c.fetchTimelineEvents(ctx, owner, repo, prNumber)
		results <- result{e, err}
	}()
	
	go func() {
		e, err := c.fetchStatusChecks(ctx, owner, repo, pr)
		results <- result{e, err}
	}()
	
	go func() {
		e, err := c.fetchCheckRuns(ctx, owner, repo, pr)
		results <- result{e, err}
	}()
	
	// Collect results - continue on partial failures for graceful degradation
	var lastErr error
	for i := 0; i < 7; i++ {
		r := <-results
		if r.err != nil {
			c.logger.Error("failed to fetch events", "error", r.err)
			lastErr = r.err
			continue
		}
		events = append(events, r.events...)
	}
	
	// Return error only if we have no events at all
	if len(events) == 0 && lastErr != nil {
		return nil, fmt.Errorf("failed to fetch any events: %w", lastErr)
	}

	// Add PR closed/merged event
	if pr.GetMerged() {
		events = append(events, Event{
			Type:      EventTypePRMerged,
			Timestamp: pr.GetMergedAt().Time,
			Actor:     pr.GetMergedBy().GetLogin(),
			Bot:       isBot(pr.GetMergedBy()),
		})
	} else if pr.GetState() == "closed" {
		events = append(events, Event{
			Type:      EventTypePRClosed,
			Timestamp: pr.GetClosedAt().Time,
			Actor:     pr.GetUser().GetLogin(),
			Bot:       isBot(pr.GetUser()),
		})
	}

	// Sort events by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	c.logger.Info("successfully fetched pull request events",
		"owner", owner,
		"repo", repo,
		"pr", prNumber,
		"event_count", len(events),
	)

	return events, nil
}