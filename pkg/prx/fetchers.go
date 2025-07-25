package prx

import (
	"context"
	"fmt"
	"time"
)

const maxPerPage = 100

func createEvent(kind EventKind, timestamp time.Time, user *githubUser, body, authorAssoc string) Event {
	body = truncate(body, 256)
	return Event{
		Kind:              kind,
		Timestamp:         timestamp,
		Actor:             user.Login,
		Body:              body,
		Question:          containsQuestion(body),
		AuthorAssociation: authorAssoc,
		Bot:               isBot(user),
		Targets:           extractMentions(body),
	}
}

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
				Kind:      Commit,
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
			event := createEvent(Comment, comment.CreatedAt, comment.User, comment.Body, comment.AuthorAssociation)
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
				event := createEvent(Review, review.SubmittedAt, review.User, review.Body, review.AuthorAssociation)
				event.Outcome = review.State // "approved", "changes_requested", "commented"
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
			event := createEvent(ReviewComment, comment.CreatedAt, comment.User, comment.Body, comment.AuthorAssociation)
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

var eventTypeMap = map[string]EventKind{
	"assigned":               Assigned,
	"unassigned":             Unassigned,
	"labeled":                Labeled,
	"unlabeled":              Unlabeled,
	"milestoned":             Milestoned,
	"demilestoned":           Demilestoned,
	"review_requested":       ReviewRequested,
	"review_request_removed": ReviewRequestRemoved,
	"reopened":               PRReopened,
	"head_ref_force_pushed":  HeadRefForcePushed,
}

func (c *Client) parseTimelineEvent(item *githubTimelineEvent) *Event {
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
		if item.RequestedReviewer != nil {
			event.Targets = []string{item.RequestedReviewer.Login}
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
			Kind:      CheckRun,
			Timestamp: timestamp,
			Actor:     actor,
			Outcome:   checkRun.Conclusion, // "success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required"
			Body:      checkRun.Name,        // Store check run name in body field
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
