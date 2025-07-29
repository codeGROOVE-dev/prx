package prx

import (
	"context"
	"fmt"
)

const maxPerPage = 100

func (c *Client) commits(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching commits", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/commits?page=%d&per_page=%d",
			owner, repo, prNumber, page, maxPerPage)
		var commits []*githubPullRequestCommit
		resp, err := c.github.get(ctx, path, &commits)
		if err != nil {
			return nil, fmt.Errorf("fetching commits: %w", err)
		}

		for _, commit := range commits {
			event := Event{
				Kind:      "commit",
				Timestamp: commit.Commit.Author.Date,
				Body:      truncate(commit.Commit.Message, 256),
			}

			// Handle case where commit.Author might be nil
			if commit.Author != nil {
				event.Actor = commit.Author.Login
				event.Bot = isBot(commit.Author)
			} else {
				// When GitHub can't associate a commit with a user (e.g., different email)
				event.Actor = "unknown"
			}

			events = append(events, event)
		}

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	c.logger.Debug("fetched commits", "count", len(events))
	return events, nil
}

func (c *Client) comments(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching comments", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?page=%d&per_page=%d", owner, repo, prNumber, page, maxPerPage)
		var comments []*githubComment
		resp, err := c.github.get(ctx, path, &comments)
		if err != nil {
			return nil, fmt.Errorf("fetching comments: %w", err)
		}

		for _, comment := range comments {
			body := truncate(comment.Body, 256)
			event := Event{
				Kind:      "comment",
				Timestamp: comment.CreatedAt,
				Actor:     comment.User.Login,
				Body:      body,
				Question:  containsQuestion(body),
				Bot:       isBot(comment.User),
			}
			event.WriteAccess = c.writeAccess(ctx, owner, repo, comment.User, comment.AuthorAssociation)
			events = append(events, event)
		}

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	c.logger.Debug("fetched comments", "count", len(events))
	return events, nil
}

func (c *Client) reviews(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching reviews", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews?page=%d&per_page=%d", owner, repo, prNumber, page, maxPerPage)
		var reviews []*githubReview
		resp, err := c.github.get(ctx, path, &reviews)
		if err != nil {
			return nil, fmt.Errorf("fetching reviews: %w", err)
		}

		for _, review := range reviews {
			if review.State != "" {
				body := truncate(review.Body, 256)
				event := Event{
					Kind:      "review",
					Timestamp: review.SubmittedAt,
					Actor:     review.User.Login,
					Body:      body,
					Question:  containsQuestion(body),
					Bot:       isBot(review.User),
				}
				event.Outcome = review.State // "approved", "changes_requested", "commented"
				event.WriteAccess = c.writeAccess(ctx, owner, repo, review.User, review.AuthorAssociation)
				events = append(events, event)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	c.logger.Debug("fetched reviews", "count", len(events))
	return events, nil
}

func (c *Client) reviewComments(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching review comments", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments?page=%d&per_page=%d", owner, repo, prNumber, page, maxPerPage)
		var comments []*githubReviewComment
		resp, err := c.github.get(ctx, path, &comments)
		if err != nil {
			return nil, fmt.Errorf("fetching review comments: %w", err)
		}

		for _, comment := range comments {
			body := truncate(comment.Body, 256)
			event := Event{
				Kind:      "review_comment",
				Timestamp: comment.CreatedAt,
				Actor:     comment.User.Login,
				Body:      body,
				Question:  containsQuestion(body),
				Bot:       isBot(comment.User),
			}
			event.WriteAccess = c.writeAccess(ctx, owner, repo, comment.User, comment.AuthorAssociation)
			events = append(events, event)
		}

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	c.logger.Debug("fetched review comments", "count", len(events))
	return events, nil
}

func (c *Client) timelineEvents(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching timeline events", "owner", owner, "repo", repo, "pr", prNumber)

	var events []Event
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/issues/%d/timeline?page=%d&per_page=%d", owner, repo, prNumber, page, maxPerPage)
		var timeline []*githubTimelineEvent
		resp, err := c.github.get(ctx, path, &timeline)
		if err != nil {
			return nil, fmt.Errorf("fetching timeline events: %w", err)
		}

		for _, item := range timeline {
			event := c.parseTimelineEvent(ctx, owner, repo, item)
			if event != nil {
				events = append(events, *event)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	c.logger.Debug("fetched timeline events", "count", len(events))
	return events, nil
}

func (c *Client) parseTimelineEvent(ctx context.Context, owner, repo string, item *githubTimelineEvent) *Event {
	event := &Event{
		Kind:      item.Event,
		Timestamp: item.CreatedAt,
		Actor:     item.Actor.Login,
		Bot:       isBot(item.Actor),
	}
	
	// Set write_access if we have author association
	if item.AuthorAssociation != "" {
		event.WriteAccess = c.writeAccess(ctx, owner, repo, item.Actor, item.AuthorAssociation)
	}

	// Set target based on event type
	switch item.Event {
	case "assigned", "unassigned":
		if item.Assignee != nil {
			event.Target = item.Assignee.Login
			event.TargetIsBot = isBot(item.Assignee)
		}
	case "labeled", "unlabeled":
		event.Target = item.Label.Name
	case "milestoned", "demilestoned":
		event.Target = item.Milestone.Title
	case "review_requested", "review_request_removed":
		if item.RequestedReviewer != nil {
			event.Target = item.RequestedReviewer.Login
			event.TargetIsBot = isBot(item.RequestedReviewer)
		} else {
			event.Target = item.RequestedTeam.Name
		}
	case "mentioned":
		event.Body = "User was mentioned"
	}

	return event
}

func (c *Client) statusChecks(ctx context.Context, owner, repo string, pr *githubPullRequest) ([]Event, error) {
	c.logger.Debug("fetching status checks", "owner", owner, "repo", repo, "sha", pr.Head.SHA)

	var events []Event

	if pr.Head.SHA == "" {
		c.logger.Debug("no SHA available for status checks")
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
			Actor:     status.Creator.Login,
			Outcome:   status.State,   // "success", "failure", "pending", "error"
			Body:      status.Context, // The status check name
		}
		if isBot(status.Creator) {
			event.Bot = true
		}
		events = append(events, event)
	}

	c.logger.Debug("fetched status checks", "count", len(events))
	return events, nil
}

func (c *Client) checkRuns(ctx context.Context, owner, repo string, pr *githubPullRequest) ([]Event, error) {
	c.logger.Debug("fetching check runs", "owner", owner, "repo", repo, "sha", pr.Head.SHA)

	var events []Event

	if pr.Head.SHA == "" {
		c.logger.Debug("no SHA available for check runs")
		return events, nil
	}

	path := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs?per_page=%d", owner, repo, pr.Head.SHA, maxPerPage)
	var checkRuns githubCheckRuns
	if _, err := c.github.get(ctx, path, &checkRuns); err != nil {
		return nil, fmt.Errorf("fetching check runs: %w", err)
	}

	for _, checkRun := range checkRuns.CheckRuns {
		timestamp := checkRun.StartedAt
		if !checkRun.CompletedAt.IsZero() {
			timestamp = checkRun.CompletedAt
		}

		var actor string
		if checkRun.App.Owner != nil {
			actor = checkRun.App.Owner.Login
		}

		event := Event{
			Kind:      "check_run",
			Timestamp: timestamp,
			Actor:     actor,
			Outcome:   checkRun.Conclusion, // "success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required"
			Body:      checkRun.Name,       // Store check run name in body field
		}
		// GitHub Apps are always considered bots
		if checkRun.App.Owner != nil {
			event.Bot = true
		}
		events = append(events, event)
	}

	c.logger.Debug("fetched check runs", "count", len(events))
	return events, nil
}
