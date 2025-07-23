package prevents

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v57/github"
)

const maxPerPage = 100

// mentionRegex matches GitHub usernames in the format @username
// GitHub usernames can contain alphanumeric characters and hyphens, but not consecutive hyphens
var mentionRegex = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])@([a-zA-Z0-9][a-zA-Z0-9\-]{0,38}[a-zA-Z0-9]|[a-zA-Z0-9])`)

// questionPatterns contains common patterns that indicate a question or request for advice
var questionPatterns = []string{
	"how can",
	"how do",
	"how would",
	"how should",
	"should i",
	"should we",
	"can i",
	"can we",
	"can you",
	"could you",
	"would you",
	"what do you think",
	"what's the best",
	"what is the best",
	"any suggestions",
	"any ideas",
	"any thoughts",
	"anyone know",
	"does anyone",
	"is it possible",
	"is there a way",
	"wondering if",
	"thoughts on",
	"advice on",
	"help with",
	"need help",
}

// extractMentions extracts all @username mentions from a text string
func extractMentions(text string) []string {
	matches := mentionRegex.FindAllStringSubmatch(text, -1)
	mentions := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	
	for _, match := range matches {
		if len(match) > 1 {
			username := match[1]
			if !seen[username] {
				mentions = append(mentions, username)
				seen[username] = true
			}
		}
	}
	
	return mentions
}

// containsQuestion checks if the text contains patterns that indicate a question or request for advice
func containsQuestion(text string) bool {
	// Convert to lowercase for case-insensitive matching
	lowerText := strings.ToLower(text)
	
	// Check for question mark
	if strings.Contains(text, "?") {
		return true
	}
	
	// Check for common question patterns
	for _, pattern := range questionPatterns {
		if strings.Contains(lowerText, pattern) {
			return true
		}
	}
	
	return false
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
				Kind:      EventTypeCommit,
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
			body := comment.GetBody()
			event := Event{
				Kind:      EventTypeComment,
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
				body := review.GetBody()
				event := Event{
					Kind:      EventTypeReview,
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
			body := comment.GetBody()
			event := Event{
				Kind:      EventTypeReviewComment,
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
	// Map of event types to our event types
	eventTypeMap := map[string]EventType{
		"assigned":               EventTypeAssigned,
		"unassigned":             EventTypeUnassigned,
		"labeled":                EventTypeLabeled,
		"unlabeled":              EventTypeUnlabeled,
		"milestoned":             EventTypeMilestoned,
		"demilestoned":           EventTypeDemilestoned,
		"review_requested":       EventTypeReviewRequested,
		"review_request_removed": EventTypeReviewRequestRemoved,
		"reopened":               EventTypePRReopened,
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
			Kind:      EventTypeStatusCheck,
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
			Kind:      EventTypeCheckRun,
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

