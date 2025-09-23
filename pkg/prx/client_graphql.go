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
	} else {
		// Add to existing required checks
		if prData.PullRequest.CheckSummary != nil && len(additionalRequired) > 0 {
			// Would need to recalculate with new required checks
			c.logger.InfoContext(ctx, "added required checks from rulesets", "count", len(additionalRequired))
		}
	}

	// Get existing required checks from GraphQL
	existingRequired := c.getExistingRequiredChecks(prData)

	// Combine with additional required checks from rulesets
	allRequired := append(existingRequired, additionalRequired...)

	// 2. Fetch check runs via REST (GraphQL's statusCheckRollup is often null)
	checkRunEvents, err := c.fetchCheckRunsREST(ctx, owner, repo, prData.PullRequest.HeadSHA)
	if err != nil {
		c.logger.WarnContext(ctx, "failed to fetch check runs via REST", "error", err)
	} else {
		// Mark check runs as required based on combined list
		for i := range checkRunEvents {
			for _, req := range allRequired {
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

// fetchRulesetsREST fetches repository rulesets via REST API (not available in GraphQL)
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

// fixWriteAccessForMembers updates write access for MEMBER users (requires REST API)
func (c *Client) fixWriteAccessForMembers(ctx context.Context, owner, repo string, events []Event) {
	// Track which users need permission checks
	memberUsers := make(map[string]bool)

	for i := range events {
		// Check if this event has MEMBER association that needs verification
		if events[i].WriteAccess == WriteAccessLikely {
			memberUsers[events[i].Actor] = true
		}
	}

	// Batch check permissions for all MEMBER users
	for user := range memberUsers {
		perm, err := c.userPermissionCached(ctx, owner, repo, user, "MEMBER")
		if err != nil {
			c.logger.DebugContext(ctx, "failed to get user permission", "error", err, "user", user)
			continue
		}

		// Update all events for this user
		writeAccess := WriteAccessUnlikely
		if perm == "admin" || perm == "write" {
			writeAccess = WriteAccessDefinitely
		}

		for i := range events {
			if events[i].Actor == user && events[i].WriteAccess == WriteAccessLikely {
				events[i].WriteAccess = writeAccess
			}
		}
	}
}

// verifyBotStatus verifies bot status using REST API if needed
func (c *Client) verifyBotStatus(ctx context.Context, owner, repo string, events []Event) {
	// GraphQL bot detection is based on login patterns and fragments
	// For critical bot detection, we might want to verify with REST API
	// However, for most cases, the GraphQL detection should be sufficient

	// Only verify if we have ambiguous cases
	ambiguousUsers := make(map[string]bool)

	for _, event := range events {
		// Check if we need to verify this user
		if event.Actor != "" && !event.Bot {
			// If it looks like it might be a bot but we're not sure
			if strings.Contains(event.Actor, "bot") ||
			   strings.Contains(event.Actor, "Bot") ||
			   strings.Contains(event.Actor, "robot") {
				ambiguousUsers[event.Actor] = true
			}
		}
	}

	// For now, we'll trust the GraphQL bot detection
	// Add REST verification here if needed in the future
	if len(ambiguousUsers) > 0 {
		c.logger.DebugContext(ctx, "ambiguous bot users detected", "users", ambiguousUsers)
	}
}

// fetchCheckRunsREST fetches check runs via REST API for a specific commit
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

		if !run.CompletedAt.IsZero() {
			timestamp = run.CompletedAt
			outcome = strings.ToLower(run.Conclusion)
		} else if !run.StartedAt.IsZero() {
			timestamp = run.StartedAt
			outcome = strings.ToLower(run.Status)
		} else {
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
		if run.Output.Title != "" && run.Output.Summary != "" {
			event.Description = fmt.Sprintf("%s: %s", run.Output.Title, run.Output.Summary)
		} else if run.Output.Title != "" {
			event.Description = run.Output.Title
		} else if run.Output.Summary != "" {
			event.Description = run.Output.Summary
		}

		events = append(events, event)
	}

	return events, nil
}

// getExistingRequiredChecks extracts required checks that were already identified
func (c *Client) getExistingRequiredChecks(prData *PullRequestData) []string {
	var required []string

	// Extract from existing events that are marked as required
	for _, event := range prData.Events {
		if event.Required && (event.Kind == "check_run" || event.Kind == "status_check") {
			required = append(required, event.Body)
		}
	}

	// Also extract from pending statuses in check summary (these are required but haven't run)
	if prData.PullRequest.CheckSummary != nil {
		for check := range prData.PullRequest.CheckSummary.PendingStatuses {
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

// recalculateCheckSummaryWithCheckRuns updates the check summary with REST-fetched check runs
func (c *Client) recalculateCheckSummaryWithCheckRuns(ctx context.Context, prData *PullRequestData, checkRunEvents []Event) {
	if prData.PullRequest.CheckSummary == nil {
		prData.PullRequest.CheckSummary = &CheckSummary{
			FailingStatuses: make(map[string]string),
			PendingStatuses: make(map[string]string),
		}
	}

	summary := prData.PullRequest.CheckSummary

	// Count check runs we fetched via REST
	for _, event := range checkRunEvents {
		if event.Kind != "check_run" {
			continue
		}

		switch event.Outcome {
		case "success":
			summary.Success++
			// Remove from pending if it was there (GraphQL might have marked it as pending)
			delete(summary.PendingStatuses, event.Body)
		case "failure", "timed_out", "action_required":
			summary.Failure++
			if event.Description != "" {
				summary.FailingStatuses[event.Body] = event.Description
			} else {
				summary.FailingStatuses[event.Body] = event.Outcome
			}
			// Remove from pending if it was there
			delete(summary.PendingStatuses, event.Body)
		case "neutral", "cancelled", "skipped", "stale":
			summary.Neutral++
			// Remove from pending if it was there
			delete(summary.PendingStatuses, event.Body)
		case "queued", "in_progress", "pending", "waiting":
			// Don't increment pending count if already counted by GraphQL
			if _, exists := summary.PendingStatuses[event.Body]; !exists {
				summary.Pending++
				if event.Description != "" {
					summary.PendingStatuses[event.Body] = event.Description
				} else {
					summary.PendingStatuses[event.Body] = "In progress"
				}
			}
		}
	}

	// Recalculate the pending count based on what's actually in PendingStatuses
	// This fixes the issue where GraphQL initially marks all required checks as pending
	summary.Pending = len(summary.PendingStatuses)

	// Update test state based on ALL events (not just check runs)
	prData.PullRequest.TestState = c.calculateTestStateFromAllEvents(prData.Events)
}

// calculateTestStateFromAllEvents determines test state from ALL check events
func (c *Client) calculateTestStateFromAllEvents(events []Event) string {
	var hasFailure, hasRunning, hasQueued, hasSuccess bool

	for _, event := range events {
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