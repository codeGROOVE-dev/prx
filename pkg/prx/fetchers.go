package prx

import (
	"context"
	"fmt"
	"time"
)

const maxPerPage = 100

// paginate fetches all pages of results from a GitHub API endpoint.
// The fetch function should unmarshal the response and return the next page number.
func paginate[T any](ctx context.Context, c *Client, path string, process func(*T) error) error {
	page := 1
	for {
		pagePath := fmt.Sprintf("%s?page=%d&per_page=%d", path, page, maxPerPage)
		var items []T
		resp, err := c.github.get(ctx, pagePath, &items)
		if err != nil {
			return err
		}

		for i := range items {
			if err := process(&items[i]); err != nil {
				return err
			}
		}

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	return nil
}

func (c *Client) commits(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.DebugContext(ctx, "fetching commits", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/commits", owner, repo, prNumber)

	err := paginate(ctx, c, path, func(commit *githubPullRequestCommit) error {
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

		events = append(events, event)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fetching commits: %w", err)
	}

	c.logger.DebugContext(ctx, "fetched commits", "count", len(events))
	return events, nil
}

func (c *Client) comments(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.DebugContext(ctx, "fetching comments", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, prNumber)

	err := paginate(ctx, c, path, func(comment *githubComment) error {
		body := truncate(comment.Body)
		event := Event{
			Kind:        "comment",
			Timestamp:   comment.CreatedAt,
			Actor:       comment.User.Login,
			Body:        body,
			Question:    containsQuestion(body),
			Bot:         isBot(comment.User),
			WriteAccess: c.writeAccess(ctx, owner, repo, comment.User, comment.AuthorAssociation),
		}
		events = append(events, event)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fetching comments: %w", err)
	}

	c.logger.DebugContext(ctx, "fetched comments", "count", len(events))
	return events, nil
}

func (c *Client) reviews(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.DebugContext(ctx, "fetching reviews", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)

	err := paginate(ctx, c, path, func(review *githubReview) error {
		if review.State == "" {
			return nil
		}

		body := truncate(review.Body)
		event := Event{
			Kind:        "review",
			Timestamp:   review.SubmittedAt,
			Actor:       review.User.Login,
			Body:        body,
			Question:    containsQuestion(body),
			Bot:         isBot(review.User),
			Outcome:     review.State,
			WriteAccess: c.writeAccess(ctx, owner, repo, review.User, review.AuthorAssociation),
		}
		events = append(events, event)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fetching reviews: %w", err)
	}

	c.logger.DebugContext(ctx, "fetched reviews", "count", len(events))
	return events, nil
}

func (c *Client) reviewComments(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.DebugContext(ctx, "fetching review comments", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", owner, repo, prNumber)

	err := paginate(ctx, c, path, func(comment *githubReviewComment) error {
		body := truncate(comment.Body)
		event := Event{
			Kind:        "review_comment",
			Timestamp:   comment.CreatedAt,
			Actor:       comment.User.Login,
			Body:        body,
			Question:    containsQuestion(body),
			Bot:         isBot(comment.User),
			WriteAccess: c.writeAccess(ctx, owner, repo, comment.User, comment.AuthorAssociation),
		}
		events = append(events, event)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fetching review comments: %w", err)
	}

	c.logger.DebugContext(ctx, "fetched review comments", "count", len(events))
	return events, nil
}

func (c *Client) timelineEvents(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.DebugContext(ctx, "fetching timeline events", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/timeline", owner, repo, prNumber)

	err := paginate(ctx, c, path, func(item *githubTimelineEvent) error {
		if event := c.parseTimelineEvent(ctx, owner, repo, item); event != nil {
			events = append(events, *event)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fetching timeline events: %w", err)
	}

	c.logger.DebugContext(ctx, "fetched timeline events", "count", len(events))
	return events, nil
}

func (c *Client) parseTimelineEvent(ctx context.Context, owner, repo string, item *githubTimelineEvent) *Event {
	event := &Event{
		Kind:      item.Event,
		Timestamp: item.CreatedAt,
	}

	// Handle actor
	if item.Actor != nil {
		event.Actor = item.Actor.Login
		event.Bot = isBot(item.Actor)
		if item.AuthorAssociation != "" {
			event.WriteAccess = c.writeAccess(ctx, owner, repo, item.Actor, item.AuthorAssociation)
		}
	} else {
		event.Actor = "unknown"
	}

	// Handle event-specific fields
	switch item.Event {
	case "assigned", "unassigned":
		if item.Assignee == nil {
			return nil
		}
		event.Target = item.Assignee.Login
		event.TargetIsBot = isBot(item.Assignee)
	case "labeled", "unlabeled":
		if item.Label.Name == "" {
			return nil
		}
		event.Target = item.Label.Name
	case "milestoned", "demilestoned":
		if item.Milestone.Title == "" {
			return nil
		}
		event.Target = item.Milestone.Title
	case "review_requested", "review_request_removed":
		if item.RequestedReviewer != nil { //nolint:gocritic // This checks different conditions, not suitable for switch
			event.Target = item.RequestedReviewer.Login
			event.TargetIsBot = isBot(item.RequestedReviewer)
		} else if item.RequestedTeam.Name != "" {
			event.Target = item.RequestedTeam.Name
		} else {
			return nil
		}
	case "mentioned":
		event.Body = "User was mentioned"
	default:
		// Unknown event type, ignore
		return nil
	}

	return event
}

func (c *Client) statusChecks(ctx context.Context, owner, repo string, pr *githubPullRequest) ([]Event, error) {
	c.logger.DebugContext(ctx, "fetching status checks", "owner", owner, "repo", repo, "sha", pr.Head.SHA)

	var events []Event

	if pr.Head.SHA == "" {
		c.logger.DebugContext(ctx, "no SHA available for status checks")
		return events, nil
	}

	path := fmt.Sprintf("/repos/%s/%s/statuses/%s?per_page=%d", owner, repo, pr.Head.SHA, maxPerPage)
	var statuses []*githubStatus
	if _, err := c.github.get(ctx, path, &statuses); err != nil {
		return nil, fmt.Errorf("fetching status checks: %w", err)
	}

	for _, status := range statuses {
		event := Event{
			Kind:      "status_check",
			Timestamp: status.CreatedAt,
			Outcome:   status.State,   // "success", "failure", "pending", "error"
			Body:      status.Context, // The status check name
		}
		if status.Creator != nil {
			event.Actor = status.Creator.Login
			event.Bot = isBot(status.Creator)
		} else {
			event.Actor = "unknown"
		}
		events = append(events, event)
	}

	c.logger.DebugContext(ctx, "fetched status checks", "count", len(events))
	return events, nil
}

func (c *Client) checkRuns(ctx context.Context, owner, repo string, pr *githubPullRequest) ([]Event, string, error) {
	c.logger.DebugContext(ctx, "fetching check runs", "owner", owner, "repo", repo, "sha", pr.Head.SHA)

	var events []Event

	if pr.Head.SHA == "" {
		c.logger.DebugContext(ctx, "no SHA available for check runs")
		return events, TestStateNone, nil
	}

	path := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs?per_page=%d", owner, repo, pr.Head.SHA, maxPerPage)
	var checkRuns githubCheckRuns
	if _, err := c.github.get(ctx, path, &checkRuns); err != nil {
		return nil, TestStateNone, fmt.Errorf("fetching check runs: %w", err)
	}

	// Track current states for test state calculation
	hasQueued := false
	hasRunning := false
	hasFailing := false
	hasPassing := false

	for _, checkRun := range checkRuns.CheckRuns {
		var actor string
		if checkRun.App.Owner != nil {
			actor = checkRun.App.Owner.Login
		}

		// Determine the current status/outcome for the check run
		var outcome string
		var timestamp time.Time
		if !checkRun.CompletedAt.IsZero() { //nolint:gocritic // This checks time fields and different conditions, not suitable for switch
			// Test has completed
			outcome = checkRun.Conclusion // "success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required"
			timestamp = checkRun.CompletedAt

			// Track state for completed tests
			switch outcome {
			case "success":
				hasPassing = true
			case "failure", "timed_out", "action_required":
				hasFailing = true
			default:
				// Other conclusions like "neutral", "cancelled", "skipped" don't affect test state
			}
		} else if checkRun.Status == "queued" {
			// Test is queued
			outcome = "queued"
			hasQueued = true
			timestamp = checkRun.StartedAt
			// If we don't have a timestamp, we'll use zero time which will sort first
		} else if checkRun.Status == "in_progress" {
			// Test is running
			outcome = "in_progress"
			hasRunning = true
			timestamp = checkRun.StartedAt
			// If we don't have a timestamp, we'll use zero time which will sort first
		} else {
			// Unknown status, skip
			continue
		}

		event := Event{
			Kind:      "check_run",
			Timestamp: timestamp,
			Actor:     actor,
			Outcome:   outcome,
			Body:      checkRun.Name, // Store check run name in body field
		}
		// GitHub Apps are always considered bots
		if checkRun.App.Owner != nil {
			event.Bot = true
		}
		events = append(events, event)
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

	c.logger.DebugContext(ctx, "fetched check runs", "count", len(events), "test_state", testState)
	return events, testState, nil
}
