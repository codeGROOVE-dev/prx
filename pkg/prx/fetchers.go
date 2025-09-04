package prx

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	maxPerPage = 100
	countField = "count"
)

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

	c.logger.DebugContext(ctx, "fetched commits", countField, len(events))
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

	c.logger.DebugContext(ctx, "fetched comments", countField, len(events))
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

	c.logger.DebugContext(ctx, "fetched reviews", countField, len(events))
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

	c.logger.DebugContext(ctx, "fetched review comments", countField, len(events))
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

	c.logger.DebugContext(ctx, "fetched timeline events", countField, len(events))
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

func (c *Client) statusChecks(ctx context.Context, owner, repo string, pr *githubPullRequest, _ []string) ([]Event, error) {
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

	c.logger.DebugContext(ctx, "fetched status checks", countField, len(events))
	return events, nil
}

func (c *Client) checkRuns(ctx context.Context, owner, repo string, pr *githubPullRequest, _ []string) ([]Event, string, error) {
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

	c.logger.DebugContext(ctx, "fetched check runs", countField, len(events), "test_state", testState)
	return events, testState, nil
}

func (c *Client) requiredStatusChecks(ctx context.Context, owner, repo string, pr *githubPullRequest) []string {
	c.logger.InfoContext(ctx, "fetching required status checks", "owner", owner, "repo", repo, "base_branch", pr.Base.Ref)

	var allRequired []string

	// Try the combined status API first - this should show what GitHub considers required
	statusPath := fmt.Sprintf("/repos/%s/%s/commits/%s/status", owner, repo, pr.Head.SHA)
	var combinedStatus githubCombinedStatus
	_, err := c.github.get(ctx, statusPath, &combinedStatus)
	if err == nil {
		c.logger.InfoContext(ctx, "fetched combined status", "state", combinedStatus.State, "total_count", combinedStatus.TotalCount)
		// Log what contexts are available in the combined status
		var contexts []string
		for _, status := range combinedStatus.Statuses {
			contexts = append(contexts, status.Context)
		}
		c.logger.InfoContext(ctx, "combined status contexts found", "contexts", contexts)
	} else {
		c.logger.InfoContext(ctx, "failed to get combined status", "error", err)
	}

	// Try branch protection rules - this is the authoritative source
	protectionPath := fmt.Sprintf("/repos/%s/%s/branches/%s/protection", owner, repo, pr.Base.Ref)
	var protection githubBranchProtection
	_, err = c.github.get(ctx, protectionPath, &protection)
	if err == nil {
		c.logger.InfoContext(ctx, "found branch protection", "required_status_checks_enabled", protection.RequiredStatusChecks != nil)
		if protection.RequiredStatusChecks != nil {
			// Add contexts from legacy format
			c.logger.InfoContext(ctx, "branch protection legacy contexts", "contexts", protection.RequiredStatusChecks.Contexts)
			allRequired = append(allRequired, protection.RequiredStatusChecks.Contexts...)

			// Add contexts from newer checks format
			var checkContexts []string
			for _, check := range protection.RequiredStatusChecks.Checks {
				checkContexts = append(checkContexts, check.Context)
				allRequired = append(allRequired, check.Context)
			}
			c.logger.InfoContext(ctx, "branch protection newer checks format", "contexts", checkContexts)
			c.logger.InfoContext(ctx, "found required status checks from branch protection", countField, len(allRequired), "checks", allRequired)
		}
	} else {
		c.logger.InfoContext(ctx, "branch protection endpoint failed", "error", err)
		// Fallback to the specific required_status_checks endpoint
		checksPath := fmt.Sprintf("/repos/%s/%s/branches/%s/protection/required_status_checks", owner, repo, pr.Base.Ref)
		var checks githubRequiredStatusChecks
		_, err = c.github.get(ctx, checksPath, &checks)
		if err == nil {
			// Add contexts from legacy format
			c.logger.InfoContext(ctx, "specific endpoint legacy contexts", "contexts", checks.Contexts)
			allRequired = append(allRequired, checks.Contexts...)

			// Add contexts from newer checks format
			var checkContexts []string
			for _, check := range checks.Checks {
				checkContexts = append(checkContexts, check.Context)
				allRequired = append(allRequired, check.Context)
			}
			c.logger.InfoContext(ctx, "specific endpoint newer checks format", "contexts", checkContexts)
			c.logger.InfoContext(ctx, "found required status checks from specific endpoint", countField, len(allRequired), "checks", allRequired)
		} else {
			c.logger.InfoContext(ctx, "no branch protection status checks found", "error", err)
		}
	}

	// Try repository rulesets (newer approach)
	rulesetPath := fmt.Sprintf("/repos/%s/%s/rulesets", owner, repo)
	var rulesets []githubRuleset
	_, err = c.github.get(ctx, rulesetPath, &rulesets)
	if err == nil {
		c.logger.InfoContext(ctx, "fetched rulesets successfully", countField, len(rulesets))
		for i, ruleset := range rulesets {
			c.logger.InfoContext(ctx, "examining ruleset",
				"index", i,
				"id", ruleset.ID,
				"name", ruleset.Name,
				"target", ruleset.Target,
				"rules", len(ruleset.Rules),
			)
			if ruleset.Target == "branch" {
				for j, rule := range ruleset.Rules {
					c.logger.InfoContext(ctx, "examining rule", "ruleset", i, "rule", j, "type", rule.Type)
					if rule.Type == "required_status_checks" && rule.Parameters.RequiredStatusChecks != nil {
						c.logger.InfoContext(ctx, "found required status checks rule", "checks", len(rule.Parameters.RequiredStatusChecks))
						for _, check := range rule.Parameters.RequiredStatusChecks {
							c.logger.InfoContext(ctx, "adding required check from ruleset", "context", check.Context)
							allRequired = append(allRequired, check.Context)
						}
					}
				}
			}
		}
		c.logger.InfoContext(ctx, "found required status checks from rulesets", "additional_from_rulesets", len(allRequired))
	} else {
		c.logger.InfoContext(ctx, "no rulesets found", "error", err)
	}

	// Check for expected GitHub Actions workflows that should run on this PR
	// But only add them as required if the PR is actually blocked due to missing checks
	if pr.MergeableState == "blocked" {
		expectedFromWorkflows, err := c.getExpectedWorkflowChecks(ctx, owner, repo, pr)
		if err != nil {
			c.logger.WarnContext(ctx, "failed to get expected workflow checks", "error", err)
		} else {
			c.logger.InfoContext(ctx,
				"found expected workflow checks but not treating as required",
				countField, len(expectedFromWorkflows),
				"checks", expectedFromWorkflows,
				"reason", "explicit_requirements_exist_or_pr_not_blocked",
			)
			// any explicit required checks from branch protection or rulesets
			if len(allRequired) == 0 && len(expectedFromWorkflows) > 0 {
				allRequired = append(allRequired, expectedFromWorkflows...)
				c.logger.InfoContext(ctx,
					"PR is blocked with no explicit required checks - treating expected workflows as required",
					countField, len(expectedFromWorkflows),
					"checks", expectedFromWorkflows,
				)
			} else {
				c.logger.InfoContext(ctx,
					"found expected workflow checks but not treating as required",
					countField, len(expectedFromWorkflows),
					"checks", expectedFromWorkflows,
					"reason", "explicit_requirements_exist_or_pr_not_blocked",
				)
			}
		}
	}

	// If we haven't found any required checks but the PR is blocked due to checks,
	// there might be organization-level or merge queue requirements we can't see
	if len(allRequired) == 0 {
		c.logger.InfoContext(ctx, "no required status checks found from any source, checking PR state", "mergeable_state", pr.MergeableState)

		// If the PR is blocked and we know checks are required (based on mergeable_state),
		// we could potentially infer missing checks, but for now we'll just log this scenario
		if pr.MergeableState == "blocked" {
			c.logger.InfoContext(ctx, "PR is blocked - there may be required checks not visible via API")
		}
		return nil
	}

	c.logger.InfoContext(ctx, "total required status checks found", countField, len(allRequired), "checks", allRequired)
	return allRequired
}

func (c *Client) getExpectedWorkflowChecks(ctx context.Context, owner, repo string, pr *githubPullRequest) ([]string, error) {
	c.logger.InfoContext(ctx, "checking for expected workflow checks", "owner", owner, "repo", repo, "head_sha", pr.Head.SHA)

	// Get all workflows in the repository
	workflowPath := fmt.Sprintf("/repos/%s/%s/actions/workflows", owner, repo)
	var workflows githubWorkflows
	_, err := c.github.get(ctx, workflowPath, &workflows)
	if err != nil {
		return nil, fmt.Errorf("fetching workflows: %w", err)
	}

	c.logger.InfoContext(ctx, "found workflows", countField, len(workflows.Workflows))

	// Get workflow runs for this specific commit to see which workflows should have run
	runsPath := fmt.Sprintf("/repos/%s/%s/actions/runs?head_sha=%s", owner, repo, pr.Head.SHA)
	var runs githubWorkflowRuns
	_, err = c.github.get(ctx, runsPath, &runs)
	if err != nil {
		c.logger.WarnContext(ctx, "failed to get workflow runs for commit", "error", err, "head_sha", pr.Head.SHA)
		// Continue without workflow run data - we'll try to infer from workflow names
	}

	c.logger.InfoContext(ctx, "found workflow runs for commit", countField, len(runs.WorkflowRuns), "head_sha", pr.Head.SHA)

	var expectedChecks []string

	// Look for workflows that should have run but either didn't run or are expected to create checks
	workflowRunsByName := make(map[string]githubWorkflowRun)
	for _, run := range runs.WorkflowRuns {
		workflowRunsByName[run.Name] = run
	}

	for _, workflow := range workflows.Workflows {
		if workflow.State != "active" {
			continue // Skip disabled workflows
		}

		c.logger.InfoContext(ctx, "examining workflow", "name", workflow.Name, "path", workflow.Path, "state", workflow.State)

		// Check if this workflow has a run for our commit
		if run, exists := workflowRunsByName[workflow.Name]; exists {
			c.logger.InfoContext(ctx, "workflow has run", "name", workflow.Name, "status", run.Status, "conclusion", run.Conclusion)
			// If workflow is still running, it should create checks
			if run.Status == "queued" || run.Status == "in_progress" || run.Status == "waiting" {
				expectedChecks = append(expectedChecks, workflow.Name)
				c.logger.InfoContext(ctx, "adding pending workflow as expected check", "name", workflow.Name, "status", run.Status)
			} else if run.Status == "completed" && c.isWorkflowExpectedForPR(workflow.Name) {
				// If workflow completed and looks like a required CI workflow, consider it as required
				expectedChecks = append(expectedChecks, workflow.Name)
				c.logger.InfoContext(ctx, "adding completed workflow as expected check",
					"name", workflow.Name,
					"conclusion", run.Conclusion,
					"reason", "completed_required_workflow",
				)
			}
		} else if c.isWorkflowExpectedForPR(workflow.Name) {
			// Workflow hasn't run for this commit - it might be expected to run
			// This is a heuristic: if the workflow name suggests it should run on PRs, add it as expected
			expectedChecks = append(expectedChecks, workflow.Name)
			c.logger.InfoContext(ctx, "adding missing workflow as expected check", "name", workflow.Name, "reason", "expected_for_pr")
		}
	}

	c.logger.InfoContext(ctx, "found expected workflow checks", countField, len(expectedChecks), "checks", expectedChecks)
	return expectedChecks, nil
}

func (*Client) isWorkflowExpectedForPR(workflowName string) bool {
	// Conservative heuristic to determine if a workflow is likely to be required for PRs
	// Only matches workflows with names that strongly suggest they're blocking CI workflows
	lowerName := strings.ToLower(workflowName)

	// These are patterns that typically indicate required CI workflows
	requiredPatterns := []string{
		"build", "test", "ci",
	}

	// Exact matches for common required workflow names
	exactMatches := []string{
		"ci", "build", "test", "tests", "checks", "main",
	}

	// Check exact matches first
	for _, exact := range exactMatches {
		if lowerName == exact {
			return true
		}
	}

	// Check if the workflow name contains required patterns
	for _, pattern := range requiredPatterns {
		if strings.Contains(lowerName, pattern) {
			return true
		}
	}

	return false
}
