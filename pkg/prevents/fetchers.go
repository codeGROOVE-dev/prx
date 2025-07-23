package prevents

import (
	"context"
	"fmt"

	"github.com/google/go-github/v57/github"
)

const maxPerPage = 100

func (c *Client) commits(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching commits", "owner", owner, "repo", repo, "pr", prNumber)
	
	var events []Event
	opts := &github.ListOptions{PerPage: maxPerPage}

	for {
		commits, resp, err := c.github.PullRequests.ListCommits(ctx, owner, repo, prNumber, opts)
		if err != nil {
			return nil, fmt.Errorf("fetching commits: %w", err)
		}

		for _, commit := range commits {
			event := Event{
				Kind:      Commit,
				Timestamp: commit.GetCommit().GetAuthor().GetDate().Time,
				Actor:     commit.GetAuthor().GetLogin(),
				Body:      commit.GetCommit().GetMessage(),
			}
			if isBot(commit.GetAuthor()) {
				event.Bot = true
			}
			events = append(events, event)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.logger.Debug("fetched commits", "count", len(events))
	return events, nil
}

func (c *Client) comments(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching comments", "owner", owner, "repo", repo, "pr", prNumber)
	
	var events []Event
	opts := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: maxPerPage}}

	for {
		comments, resp, err := c.github.Issues.ListComments(ctx, owner, repo, prNumber, opts)
		if err != nil {
			return nil, fmt.Errorf("fetching comments: %w", err)
		}

		for _, comment := range comments {
			body := comment.GetBody()
			event := Event{
				Kind:      Comment,
				Timestamp: comment.GetCreatedAt().Time,
				Actor:     comment.GetUser().GetLogin(),
				Body:      body,
				Question:  containsQuestion(body),
			}
			if isBot(comment.GetUser()) {
				event.Bot = true
			}
			// Extract mentions and add to targets
			mentions := extractMentions(body)
			if len(mentions) > 0 {
				event.Targets = mentions
			}
			events = append(events, event)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.logger.Debug("fetched comments", "count", len(events))
	return events, nil
}

func (c *Client) reviews(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching reviews", "owner", owner, "repo", repo, "pr", prNumber)
	
	var events []Event
	opts := &github.ListOptions{PerPage: maxPerPage}

	for {
		reviews, resp, err := c.github.PullRequests.ListReviews(ctx, owner, repo, prNumber, opts)
		if err != nil {
			return nil, fmt.Errorf("fetching reviews: %w", err)
		}

		for _, review := range reviews {
			if review.GetState() != "" {
				body := review.GetBody()
				event := Event{
					Kind:      Review,
					Timestamp: review.GetSubmittedAt().Time,
					Actor:     review.GetUser().GetLogin(),
					Outcome:   review.GetState(), // "approved", "changes_requested", "commented"
					Body:      body,
					Question:  containsQuestion(body),
				}
				if isBot(review.GetUser()) {
					event.Bot = true
				}
				// Extract mentions and add to targets
				mentions := extractMentions(body)
				if len(mentions) > 0 {
					event.Targets = mentions
				}
				events = append(events, event)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.logger.Debug("fetched reviews", "count", len(events))
	return events, nil
}

func (c *Client) reviewComments(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching review comments", "owner", owner, "repo", repo, "pr", prNumber)
	
	var events []Event
	opts := &github.PullRequestListCommentsOptions{ListOptions: github.ListOptions{PerPage: maxPerPage}}

	for {
		comments, resp, err := c.github.PullRequests.ListComments(ctx, owner, repo, prNumber, opts)
		if err != nil {
			return nil, fmt.Errorf("fetching review comments: %w", err)
		}

		for _, comment := range comments {
			body := comment.GetBody()
			event := Event{
				Kind:      ReviewComment,
				Timestamp: comment.GetCreatedAt().Time,
				Actor:     comment.GetUser().GetLogin(),
				Body:      body,
				Question:  containsQuestion(body),
			}
			if isBot(comment.GetUser()) {
				event.Bot = true
			}
			// Extract mentions and add to targets
			mentions := extractMentions(body)
			if len(mentions) > 0 {
				event.Targets = mentions
			}
			events = append(events, event)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	c.logger.Debug("fetched review comments", "count", len(events))
	return events, nil
}

func (c *Client) timelineEvents(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching timeline events", "owner", owner, "repo", repo, "pr", prNumber)
	
	var events []Event
	opts := &github.ListOptions{PerPage: maxPerPage}

	for {
		timeline, resp, err := c.github.Issues.ListIssueTimeline(ctx, owner, repo, prNumber, opts)
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
		opts.Page = resp.NextPage
	}

	c.logger.Debug("fetched timeline events", "count", len(events))
	return events, nil
}

func (c *Client) parseTimelineEvent(item *github.Timeline) *Event {
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

	eventType, ok := eventTypeMap[item.GetEvent()]
	if !ok {
		return nil
	}

	event := &Event{
		Kind:      eventType,
		Timestamp: item.GetCreatedAt().Time,
		Actor:     item.GetActor().GetLogin(),
		Bot:       isBot(item.GetActor()),
	}

	// Extract targets based on event type
	switch item.GetEvent() {
	case "assigned", "unassigned":
		if item.GetAssignee() != nil {
			event.Targets = []string{item.GetAssignee().GetLogin()}
		}
	case "labeled", "unlabeled":
		if item.GetLabel() != nil {
			event.Targets = []string{item.GetLabel().GetName()}
		}
	case "milestoned", "demilestoned":
		if item.GetMilestone() != nil {
			event.Targets = []string{item.GetMilestone().GetTitle()}
		}
	case "review_requested", "review_request_removed":
		// GitHub API returns reviewer in different fields based on type
		if item.Reviewer != nil {
			event.Targets = []string{item.Reviewer.GetLogin()}
		} else if item.RequestedTeam != nil {
			event.Targets = []string{item.RequestedTeam.GetName()}
		}
	}

	return event
}

func (c *Client) statusChecks(ctx context.Context, owner, repo string, pr *github.PullRequest) ([]Event, error) {
	c.logger.Debug("fetching status checks", "owner", owner, "repo", repo, "sha", pr.GetHead().GetSHA())
	
	var events []Event
	
	if pr.GetHead().GetSHA() == "" {
		c.logger.Debug("no SHA available for status checks")
		return events, nil
	}

	statuses, _, err := c.github.Repositories.ListStatuses(ctx, owner, repo, pr.GetHead().GetSHA(), &github.ListOptions{PerPage: maxPerPage})
	if err != nil {
		return nil, fmt.Errorf("fetching status checks: %w", err)
	}

	for _, status := range statuses {
		event := Event{
			Kind:      StatusCheck,
			Timestamp: status.GetCreatedAt().Time,
			Actor:     status.GetCreator().GetLogin(),
			Outcome:   status.GetState(), // "success", "failure", "pending", "error"
		}
		if isBot(status.GetCreator()) {
			event.Bot = true
		}
		events = append(events, event)
	}

	c.logger.Debug("fetched status checks", "count", len(events))
	return events, nil
}

func (c *Client) checkRuns(ctx context.Context, owner, repo string, pr *github.PullRequest) ([]Event, error) {
	c.logger.Debug("fetching check runs", "owner", owner, "repo", repo, "sha", pr.GetHead().GetSHA())
	
	var events []Event
	
	if pr.GetHead().GetSHA() == "" {
		c.logger.Debug("no SHA available for check runs")
		return events, nil
	}

	checkRuns, _, err := c.github.Checks.ListCheckRunsForRef(ctx, owner, repo, pr.GetHead().GetSHA(), &github.ListCheckRunsOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetching check runs: %w", err)
	}

	for _, checkRun := range checkRuns.CheckRuns {
		timestamp := checkRun.GetStartedAt().Time
		if !checkRun.GetCompletedAt().IsZero() {
			timestamp = checkRun.GetCompletedAt().Time
		}
		
		event := Event{
			Kind:      CheckRun,
			Timestamp: timestamp,
			Actor:     checkRun.GetApp().GetOwner().GetLogin(),
			Outcome:   checkRun.GetConclusion(), // "success", "failure", "neutral", "cancelled", "skipped", "timed_out", "action_required"
		}
		// GitHub Apps are always considered bots
		if checkRun.GetApp() != nil {
			event.Bot = true
		}
		events = append(events, event)
	}

	c.logger.Debug("fetched check runs", "count", len(events))
	return events, nil
}

