package prevents

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
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

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		// Wrap the transport with retry logic if not already wrapped
		if httpClient.Transport == nil {
			httpClient.Transport = &RetryTransport{Base: http.DefaultTransport}
		} else if _, ok := httpClient.Transport.(*RetryTransport); !ok {
			httpClient.Transport = &RetryTransport{Base: httpClient.Transport}
		}
		c.github = github.NewClient(httpClient)
	}
}

// NewClient creates a new Client with the given GitHub token.
// If token is empty, WithHTTPClient option must be provided.
func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		logger: slog.Default(),
	}
	
	// Apply options first to check if custom HTTP client is provided
	for _, opt := range opts {
		opt(c)
	}
	
	// If no GitHub client was set by options, create one from token
	if c.github == nil {
		if token == "" {
			panic("prevents: token is required when WithHTTPClient option is not provided")
		}
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(context.Background(), ts)
		
		// Wrap the transport with retry logic
		tc.Transport = &RetryTransport{Base: tc.Transport}
		
		c.github = github.NewClient(tc)
	}
	
	return c
}

// PullRequestEvents fetches all events for a pull request and returns them in chronological order.
func (c *Client) PullRequestEvents(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
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
		Kind:      PROpened,
		Timestamp: pr.GetCreatedAt().Time,
		Actor:     pr.GetUser().GetLogin(),
		Bot:       isBot(pr.GetUser()),
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
		e, err := c.statusChecks(gctx, owner, repo, pr)
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
		e, err := c.checkRuns(gctx, owner, repo, pr)
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
	if pr.GetMerged() {
		events = append(events, Event{
			Kind:      PRMerged,
			Timestamp: pr.GetMergedAt().Time,
			Actor:     pr.GetMergedBy().GetLogin(),
			Bot:       isBot(pr.GetMergedBy()),
		})
	} else if pr.GetState() == "closed" {
		events = append(events, Event{
			Kind:      PRClosed,
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