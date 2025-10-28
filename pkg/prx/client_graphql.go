package prx

import (
	"context"
	"fmt"
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

	// 2. Fetch check runs via REST (GraphQL's statusCheckRollup is often null)
	checkRunEvents, err := c.fetchCheckRunsREST(ctx, owner, repo, prData.PullRequest.HeadSHA)
	if err != nil {
		c.logger.WarnContext(ctx, "failed to fetch check runs via REST", "error", err)
	} else {
		// Mark check runs as required based on combined list
		for i := range checkRunEvents {
			for _, req := range existingRequired {
				if checkRunEvents[i].Body == req {
					checkRunEvents[i].Required = true
					break
				}
			}
		}

		// Add check run events to the events list
		prData.Events = append(prData.Events, checkRunEvents...)

		// Recalculate check summary with the new check run data
		if len(checkRunEvents) > 0 {
			c.recalculateCheckSummaryWithCheckRuns(ctx, prData, checkRunEvents)
		}

		c.logger.InfoContext(ctx, "fetched check runs via REST", "count", len(checkRunEvents))
	}

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
			found := false
			for _, r := range required {
				if r == check {
					found = true
					break
				}
			}
			if !found {
				required = append(required, check)
			}
		}
	}

	return required
}

// recalculateCheckSummaryWithCheckRuns updates the check summary with REST-fetched check runs.
func (c *Client) recalculateCheckSummaryWithCheckRuns(_ /* ctx */ context.Context, prData *PullRequestData, checkRunEvents []Event) {
	if prData.PullRequest.CheckSummary == nil {
		prData.PullRequest.CheckSummary = &CheckSummary{
			Success:   make(map[string]string),
			Failing:   make(map[string]string),
			Pending:   make(map[string]string),
			Cancelled: make(map[string]string),
			Skipped:   make(map[string]string),
			Stale:     make(map[string]string),
			Neutral:   make(map[string]string),
		}
	}

	summary := prData.PullRequest.CheckSummary

	// Process check runs we fetched via REST
	for i := range checkRunEvents {
		event := &checkRunEvents[i]
		if event.Kind != "check_run" {
			continue
		}

		desc := event.Description
		if desc == "" {
			desc = event.Outcome
		}

		switch event.Outcome {
		case "success":
			summary.Success[event.Body] = desc
			// Remove from pending if it was there (GraphQL might have marked it as pending)
			delete(summary.Pending, event.Body)
		case "failure", "timed_out", "action_required":
			summary.Failing[event.Body] = desc
			// Remove from pending if it was there
			delete(summary.Pending, event.Body)
		case "cancelled":
			summary.Cancelled[event.Body] = desc
			delete(summary.Pending, event.Body)
		case "skipped":
			summary.Skipped[event.Body] = desc
			delete(summary.Pending, event.Body)
		case "stale":
			summary.Stale[event.Body] = desc
			delete(summary.Pending, event.Body)
		case "neutral":
			summary.Neutral[event.Body] = desc
			delete(summary.Pending, event.Body)
		case "queued", "in_progress", "pending", "waiting":
			// Don't overwrite if already counted by GraphQL
			if _, exists := summary.Pending[event.Body]; !exists {
				summary.Pending[event.Body] = desc
			}
		default:
			// Unknown outcome, ignore
		}
	}

	// Update test state based on ALL events (not just check runs)
	prData.PullRequest.TestState = c.calculateTestStateFromAllEvents(prData.Events)
}

// calculateTestStateFromAllEvents determines test state from ALL check events.
func (*Client) calculateTestStateFromAllEvents(events []Event) string {
	var hasFailure, hasRunning, hasQueued, hasSuccess bool

	for i := range events {
		event := &events[i]
		if event.Kind != "check_run" && event.Kind != "status_check" {
			continue
		}

		switch event.Outcome {
		case "failure", "timed_out", "action_required":
			hasFailure = true
		case "in_progress":
			hasRunning = true
		case "queued", "pending", "waiting":
			hasQueued = true
		case "success":
			hasSuccess = true
		default:
			// Other outcomes don't affect test state
		}
	}

	// Any failure means tests are failing
	if hasFailure {
		return TestStateFailing
	}
	if hasRunning {
		return TestStateRunning
	}
	if hasQueued {
		return TestStateQueued
	}
	if hasSuccess {
		return TestStatePassing
	}
	return TestStateNone
}
