package prevents

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
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/commits?page=%d&per_page=%d", owner, repo, prNumber, page, maxPerPage)
		var commits []*githubPullRequestCommit
		resp, err := c.github.get(ctx, path, &commits)
		if err != nil {
			return nil, fmt.Errorf("fetching commits: %w", err)
		}

		for _, commit := range commits {
			event := Event{
				Kind:      Commit,
				Timestamp: commit.Commit.Author.Date,
				Actor:     commit.Author.Login,
				Body:      commit.Commit.Message,
			}
			if isBot(commit.Author) {
				event.Bot = true
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
			event := Event{
				Kind:      Comment,
				Timestamp: comment.CreatedAt,
				Actor:     comment.User.Login,
				Body:      comment.Body,
				Question:  containsQuestion(comment.Body),
			}
			if isBot(comment.User) {
				event.Bot = true
			}
			// Extract mentions and add to targets
			mentions := extractMentions(comment.Body)
			if len(mentions) > 0 {
				event.Targets = mentions
			}
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
				event := Event{
					Kind:      Review,
					Timestamp: review.SubmittedAt,
					Actor:     review.User.Login,
					Outcome:   review.State, // "approved", "changes_requested", "commented"
					Body:      review.Body,
					Question:  containsQuestion(review.Body),
				}
				if isBot(review.User) {
					event.Bot = true
				}
				// Extract mentions and add to targets
				mentions := extractMentions(review.Body)
				if len(mentions) > 0 {
					event.Targets = mentions
				}
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
			event := Event{
				Kind:      ReviewComment,
				Timestamp: comment.CreatedAt,
				Actor:     comment.User.Login,
				Body:      comment.Body,
				Question:  containsQuestion(comment.Body),
			}
			if isBot(comment.User) {
				event.Bot = true
			}
			// Extract mentions and add to targets
			mentions := extractMentions(comment.Body)
			if len(mentions) > 0 {
				event.Targets = mentions
			}
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
			event := c.parseTimelineEvent(item)
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

func (c *Client) parseTimelineEvent(item *githubTimelineEvent) *Event {
	// Map of event types to our event types
	eventTypeMap := map[string]EventKind{
		"assigned":               Assigned,
		"unassigned":             Unassigned,
		"labeled":                Labeled,
		"unlabeled":              Unlabeled,
		"milestoned":             Milestoned,
		"demilestoned":           Demilestoned,
		"review_requested":       ReviewRequested,
		"review_request_removed": ReviewRequestRemoved,
		"reopened":               PRReopened,
	}

	eventType, ok := eventTypeMap[item.Event]
	if !ok {
		return nil
	}

	event := &Event{
		Kind:      eventType,
		Timestamp: item.CreatedAt,
		Actor:     item.Actor.Login,
		Bot:       isBot(item.Actor),
	}

	// Extract targets based on event type
	switch item.Event {
	case "assigned", "unassigned":
		if item.Assignee != nil {
			event.Targets = []string{item.Assignee.Login}
		}
	case "labeled", "unlabeled":
		if item.Label.Name != "" {
			event.Targets = []string{item.Label.Name}
		}
	case "milestoned", "demilestoned":
		if item.Milestone.Title != "" {
			event.Targets = []string{item.Milestone.Title}
		}
	case "review_requested", "review_request_removed":
		// GitHub API returns reviewer in different fields based on type
		if item.Reviewer != nil {
			event.Targets = []string{item.Reviewer.Login}
		} else if item.RequestedTeam.Name != "" {
			event.Targets = []string{item.RequestedTeam.Name}
		}
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
			Kind:      StatusCheck,
			Timestamp: status.CreatedAt,
			Actor:     status.Creator.Login,
			Outcome:   status.State, // "success", "failure", "pending", "error"
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

		event := Event{
			Kind:      CheckRun,
			Timestamp: timestamp,
			Actor:     checkRun.App.Owner.Login,
			Outcome:   checkRun.Conclusion, // "success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required"
		}
		// GitHub Apps are always considered bots
		if &checkRun.App != nil {
			event.Bot = true
		}
		events = append(events, event)
	}

	c.logger.Debug("fetched check runs", "count", len(events))
	return events, nil
}

