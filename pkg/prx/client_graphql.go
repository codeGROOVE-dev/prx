package prx

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// pullRequestViaGraphQL fetches pull request data using GraphQL with minimal REST fallbacks.
// This hybrid approach reduces API calls from 13+ to ~3-4 while maintaining complete data fidelity.
func (c *Client) pullRequestViaGraphQL(ctx context.Context, owner, repo string, prNumber int) (*PullRequestData, error) {
	c.logger.InfoContext(ctx, "fetching pull request via GraphQL", "owner", owner, "repo", repo, "pr", prNumber)

	// Main GraphQL query - gets 90% of the data in one call
	prData, err := c.fetchPullRequestCompleteViaGraphQL(ctx, owner, repo, prNumber)
	if err != nil {
		// Don't fall back to REST - fail with the GraphQL error
		return nil, fmt.Errorf("GraphQL query failed: %w", err)
	}

	// REST API calls for missing data (minimal)
	// 1. Fetch rulesets (not available in GraphQL)
	additionalRequired, err := c.fetchRulesetsREST(ctx, owner, repo)
	if err != nil {
		c.logger.WarnContext(ctx, "failed to fetch rulesets", "error", err)
	} else if prData.PullRequest.CheckSummary != nil && len(additionalRequired) > 0 {
		// Add to existing required checks
		// Would need to recalculate with new required checks
		c.logger.InfoContext(ctx, "added required checks from rulesets", "count", len(additionalRequired))
	}

	// Get existing required checks from GraphQL
	existingRequired := c.existingRequiredChecks(prData)

	// Combine with additional required checks from rulesets
	existingRequired = append(existingRequired, additionalRequired...)

	// 2. Fetch check runs via REST for all commits (GraphQL's statusCheckRollup is often null)
	// This ensures we capture check run history including failures from earlier commits
	checkRunEvents := c.fetchAllCheckRunsREST(ctx, owner, repo, prData)

	// Mark check runs as required based on combined list
	for i := range checkRunEvents {
		if slices.Contains(existingRequired, checkRunEvents[i].Body) {
			checkRunEvents[i].Required = true
		}
	}

	// Add check run events to the events list
	prData.Events = append(prData.Events, checkRunEvents...)

	// Recalculate check summary with the new check run data
	if len(checkRunEvents) > 0 {
		c.recalculateCheckSummaryWithCheckRuns(ctx, prData, checkRunEvents)
	}

	c.logger.InfoContext(ctx, "fetched check runs via REST", "count", len(checkRunEvents))

	// Sort all events chronologically (oldest to newest)
	sort.Slice(prData.Events, func(i, j int) bool {
		return prData.Events[i].Timestamp.Before(prData.Events[j].Timestamp)
	})

	apiCallsUsed := 2 // GraphQL + rulesets
	if len(checkRunEvents) > 0 {
		apiCallsUsed++ // + check runs
	}

	c.logger.InfoContext(ctx, "successfully fetched pull request via hybrid GraphQL+REST",
		"owner", owner, "repo", repo, "pr", prNumber,
		"event_count", len(prData.Events),
		"api_calls_made", fmt.Sprintf("%d (vs 13+ with REST)", apiCallsUsed))

	return prData, nil
}

// fetchRulesetsREST fetches repository rulesets via REST API (not available in GraphQL).
func (c *Client) fetchRulesetsREST(ctx context.Context, owner, repo string) ([]string, error) {
	rulesetPath := fmt.Sprintf("/repos/%s/%s/rulesets", owner, repo)
	var rulesets []githubRuleset

	_, err := c.github.get(ctx, rulesetPath, &rulesets)
	if err != nil {
		return nil, err
	}

	var requiredChecks []string
	for _, ruleset := range rulesets {
		if ruleset.Target == "branch" {
			for _, rule := range ruleset.Rules {
				if rule.Type == "required_status_checks" && rule.Parameters.RequiredStatusChecks != nil {
					for _, check := range rule.Parameters.RequiredStatusChecks {
						requiredChecks = append(requiredChecks, check.Context)
					}
				}
			}
		}
	}

	c.logger.InfoContext(ctx, "fetched required checks from rulesets",
		"count", len(requiredChecks), "checks", requiredChecks)

	return requiredChecks, nil
}

// fetchCheckRunsREST fetches check runs via REST API for a specific commit.
func (c *Client) fetchCheckRunsREST(ctx context.Context, owner, repo, sha string) ([]Event, error) {
	if sha == "" {
		return nil, nil
	}

	path := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs?per_page=100", owner, repo, sha)
	var checkRuns githubCheckRuns
	if _, err := c.github.get(ctx, path, &checkRuns); err != nil {
		return nil, fmt.Errorf("fetching check runs: %w", err)
	}

	var events []Event
	for _, run := range checkRuns.CheckRuns {
		if run == nil {
			continue
		}

		var timestamp time.Time
		var outcome string

		switch {
		case !run.CompletedAt.IsZero():
			timestamp = run.CompletedAt
			outcome = strings.ToLower(run.Conclusion)
		case !run.StartedAt.IsZero():
			timestamp = run.StartedAt
			outcome = strings.ToLower(run.Status)
		default:
			// No timestamp available, skip this check run
			continue
		}

		event := Event{
			Kind:      "check_run",
			Timestamp: timestamp,
			Actor:     "github",
			Bot:       true,
			Body:      run.Name,
			Outcome:   outcome,
		}

		// Build description from output
		switch {
		case run.Output.Title != "" && run.Output.Summary != "":
			event.Description = fmt.Sprintf("%s: %s", run.Output.Title, run.Output.Summary)
		case run.Output.Title != "":
			event.Description = run.Output.Title
		case run.Output.Summary != "":
			event.Description = run.Output.Summary
		default:
			// No description available
		}

		events = append(events, event)
	}

	return events, nil
}

// fetchAllCheckRunsREST fetches check runs for all commits in the PR.
// This ensures we capture the full history including failures from earlier commits
// that may have been superseded by successful runs on later commits.
// Errors fetching individual commits are logged but don't stop the overall process.
func (c *Client) fetchAllCheckRunsREST(ctx context.Context, owner, repo string, prData *PullRequestData) []Event {
	// Collect all unique commit SHAs from the PR
	commitSHAs := make([]string, 0)

	// Add HEAD SHA (most important)
	if prData.PullRequest.HeadSHA != "" {
		commitSHAs = append(commitSHAs, prData.PullRequest.HeadSHA)
	}

	// Add all other commit SHAs from commit events
	for i := range prData.Events {
		e := &prData.Events[i]
		if e.Kind == "commit" && e.Body != "" {
			// Body contains the commit SHA for commit events
			commitSHAs = append(commitSHAs, e.Body)
		}
	}

	// Deduplicate SHAs (in case HEAD is also in commit events)
	uniqueSHAs := make(map[string]bool)
	for _, sha := range commitSHAs {
		uniqueSHAs[sha] = true
	}

	// Fetch check runs for each unique commit
	var allEvents []Event
	seenCheckRuns := make(map[string]bool) // Track unique check runs by "name:timestamp"

	for sha := range uniqueSHAs {
		events, err := c.fetchCheckRunsREST(ctx, owner, repo, sha)
		if err != nil {
			c.logger.WarnContext(ctx, "failed to fetch check runs for commit", "sha", sha, "error", err)
			continue
		}

		// Add only unique check runs (same check can run on multiple commits)
		for i := range events {
			event := &events[i]
			// Create a unique key based on check name and timestamp
			key := fmt.Sprintf("%s:%s", event.Body, event.Timestamp.Format(time.RFC3339Nano))
			if !seenCheckRuns[key] {
				seenCheckRuns[key] = true
				// Add the commit SHA to the Target field
				event.Target = sha
				allEvents = append(allEvents, *event)
			}
		}
	}

	return allEvents
}

// existingRequiredChecks extracts required checks that were already identified.
func (*Client) existingRequiredChecks(prData *PullRequestData) []string {
	var required []string

	// Extract from existing events that are marked as required
	for i := range prData.Events {
		event := &prData.Events[i]
		if event.Required && (event.Kind == "check_run" || event.Kind == "status_check") {
			required = append(required, event.Body)
		}
	}

	// Also extract from pending checks in check summary (these are required but haven't run)
	if prData.PullRequest.CheckSummary != nil {
		for check := range prData.PullRequest.CheckSummary.Pending {
			// Check if it's not already in the list
			found := slices.Contains(required, check)
			if !found {
				required = append(required, check)
			}
		}
	}

	return required
}

// recalculateCheckSummaryWithCheckRuns updates the check summary with REST-fetched check runs.
// This recalculates the entire check summary from ALL events to ensure we have the latest state.
func (c *Client) recalculateCheckSummaryWithCheckRuns(_ /* ctx */ context.Context, prData *PullRequestData, _ /* checkRunEvents */ []Event) {
	// Get existing required checks before we overwrite the summary
	var requiredChecks []string
	if prData.PullRequest.CheckSummary != nil {
		for check := range prData.PullRequest.CheckSummary.Pending {
			requiredChecks = append(requiredChecks, check)
		}
	}

	// Recalculate the entire check summary from ALL events (including the new check runs)
	// This ensures we get the latest state based on timestamps
	prData.PullRequest.CheckSummary = calculateCheckSummary(prData.Events, requiredChecks)

	// Update test state based on the recalculated check summary
	prData.PullRequest.TestState = c.calculateTestStateFromCheckSummary(prData.PullRequest.CheckSummary)
}

// calculateTestStateFromCheckSummary determines test state from a CheckSummary.
// This looks at the LATEST state of checks (after deduplication) rather than all events.
func (*Client) calculateTestStateFromCheckSummary(summary *CheckSummary) string {
	if summary == nil {
		return TestStateNone
	}

	// Any failing checks means tests are failing
	if len(summary.Failing) > 0 {
		return TestStateFailing
	}

	// Any pending checks means tests are pending
	if len(summary.Pending) > 0 {
		return TestStatePending
	}

	// If we have successful checks and nothing failing/pending, tests are passing
	if len(summary.Success) > 0 {
		return TestStatePassing
	}

	return TestStateNone
}
