package prevents

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v57/github"
)

const maxPerPage = 100

// isBot checks if a GitHub user is a bot
func isBot(user *github.User) bool {
	if user == nil {
		return false
	}
	
	// Check if explicitly marked as bot
	if user.GetType() == "Bot" {
		return true
	}
	
	// Check if username ends with bot patterns
	login := user.GetLogin()
	return strings.HasSuffix(login, "-bot") || 
		strings.HasSuffix(login, "[bot]") || 
		strings.HasSuffix(login, "-robot")
}

func (c *Client) fetchCommits(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
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
				Type:      EventTypeCommit,
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

func (c *Client) fetchComments(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching comments", "owner", owner, "repo", repo, "pr", prNumber)
	
	var events []Event
	opts := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: maxPerPage}}

	for {
		comments, resp, err := c.github.Issues.ListComments(ctx, owner, repo, prNumber, opts)
		if err != nil {
			return nil, fmt.Errorf("fetching comments: %w", err)
		}

		for _, comment := range comments {
			event := Event{
				Type:      EventTypeComment,
				Timestamp: comment.GetCreatedAt().Time,
				Actor:     comment.GetUser().GetLogin(),
				Body:      comment.GetBody(),
			}
			if isBot(comment.GetUser()) {
				event.Bot = true
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

func (c *Client) fetchReviews(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
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
				event := Event{
					Type:      EventTypeReview,
					Timestamp: review.GetSubmittedAt().Time,
					Actor:     review.GetUser().GetLogin(),
					Outcome:   review.GetState(), // "approved", "changes_requested", "commented"
					Body:      review.GetBody(),
				}
				if isBot(review.GetUser()) {
					event.Bot = true
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

func (c *Client) fetchReviewComments(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
	c.logger.Debug("fetching review comments", "owner", owner, "repo", repo, "pr", prNumber)
	
	var events []Event
	opts := &github.PullRequestListCommentsOptions{ListOptions: github.ListOptions{PerPage: maxPerPage}}

	for {
		comments, resp, err := c.github.PullRequests.ListComments(ctx, owner, repo, prNumber, opts)
		if err != nil {
			return nil, fmt.Errorf("fetching review comments: %w", err)
		}

		for _, comment := range comments {
			event := Event{
				Type:      EventTypeReviewComment,
				Timestamp: comment.GetCreatedAt().Time,
				Actor:     comment.GetUser().GetLogin(),
				Body:      comment.GetBody(),
			}
			if isBot(comment.GetUser()) {
				event.Bot = true
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

func (c *Client) fetchTimelineEvents(ctx context.Context, owner, repo string, prNumber int) ([]Event, error) {
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
	switch item.GetEvent() {
	case "assigned":
		event := &Event{
			Type:      EventTypeAssigned,
			Timestamp: item.GetCreatedAt().Time,
			Actor:     item.GetActor().GetLogin(),
		}
		if item.GetAssignee() != nil {
			event.Targets = []string{item.GetAssignee().GetLogin()}
		}
		if isBot(item.GetActor()) {
			event.Bot = true
		}
		return event
	case "unassigned":
		event := &Event{
			Type:      EventTypeUnassigned,
			Timestamp: item.GetCreatedAt().Time,
			Actor:     item.GetActor().GetLogin(),
		}
		if item.GetAssignee() != nil {
			event.Targets = []string{item.GetAssignee().GetLogin()}
		}
		if isBot(item.GetActor()) {
			event.Bot = true
		}
		return event
	case "labeled":
		event := &Event{
			Type:      EventTypeLabeled,
			Timestamp: item.GetCreatedAt().Time,
			Actor:     item.GetActor().GetLogin(),
		}
		if item.GetLabel() != nil {
			event.Targets = []string{item.GetLabel().GetName()}
		}
		if isBot(item.GetActor()) {
			event.Bot = true
		}
		return event
	case "unlabeled":
		event := &Event{
			Type:      EventTypeUnlabeled,
			Timestamp: item.GetCreatedAt().Time,
			Actor:     item.GetActor().GetLogin(),
		}
		if item.GetLabel() != nil {
			event.Targets = []string{item.GetLabel().GetName()}
		}
		if isBot(item.GetActor()) {
			event.Bot = true
		}
		return event
	case "milestoned":
		event := &Event{
			Type:      EventTypeMilestoned,
			Timestamp: item.GetCreatedAt().Time,
			Actor:     item.GetActor().GetLogin(),
		}
		if item.GetMilestone() != nil {
			event.Targets = []string{item.GetMilestone().GetTitle()}
		}
		if isBot(item.GetActor()) {
			event.Bot = true
		}
		return event
	case "demilestoned":
		event := &Event{
			Type:      EventTypeDemilestoned,
			Timestamp: item.GetCreatedAt().Time,
			Actor:     item.GetActor().GetLogin(),
		}
		if item.GetMilestone() != nil {
			event.Targets = []string{item.GetMilestone().GetTitle()}
		}
		if isBot(item.GetActor()) {
			event.Bot = true
		}
		return event
	case "review_requested":
		event := &Event{
			Type:      EventTypeReviewRequested,
			Timestamp: item.GetCreatedAt().Time,
			Actor:     item.GetActor().GetLogin(),
		}
		// GitHub API returns reviewer in different fields based on type
		if item.Reviewer != nil {
			event.Targets = []string{item.Reviewer.GetLogin()}
		} else if item.RequestedTeam != nil {
			event.Targets = []string{item.RequestedTeam.GetName()}
		}
		if isBot(item.GetActor()) {
			event.Bot = true
		}
		return event
	case "review_request_removed":
		event := &Event{
			Type:      EventTypeReviewRequestRemoved,
			Timestamp: item.GetCreatedAt().Time,
			Actor:     item.GetActor().GetLogin(),
		}
		// GitHub API returns reviewer in different fields based on type
		if item.Reviewer != nil {
			event.Targets = []string{item.Reviewer.GetLogin()}
		} else if item.RequestedTeam != nil {
			event.Targets = []string{item.RequestedTeam.GetName()}
		}
		if isBot(item.GetActor()) {
			event.Bot = true
		}
		return event
	case "reopened":
		event := &Event{
			Type:      EventTypePRReopened,
			Timestamp: item.GetCreatedAt().Time,
			Actor:     item.GetActor().GetLogin(),
		}
		if isBot(item.GetActor()) {
			event.Bot = true
		}
		return event
	}
	return nil
}

func (c *Client) fetchStatusChecks(ctx context.Context, owner, repo string, pr *github.PullRequest) ([]Event, error) {
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
			Type:      EventTypeStatusCheck,
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

func (c *Client) fetchCheckRuns(ctx context.Context, owner, repo string, pr *github.PullRequest) ([]Event, error) {
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
			Type:      EventTypeCheckRun,
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

