package prx

import (
	"sort"
	"strings"
)

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

func containsQuestion(text string) bool {
	if strings.Contains(text, "?") {
		return true
	}
	lowerText := strings.ToLower(text)
	for _, pattern := range questionPatterns {
		if strings.Contains(lowerText, pattern) {
			return true
		}
	}
	return false
}

func sortEventsByTimestamp(events []Event) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
}

func isHexString(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func calculateTestSummary(events []Event) *TestSummary {
	summary := &TestSummary{}
	checkStates := make(map[string]string)

	for _, event := range events {
		if event.Kind == "check_run" && event.Body != "" {
			checkStates[event.Body] = event.Outcome
		}
	}

	for _, outcome := range checkStates {
		switch outcome {
		case "success":
			summary.Passing++
		case "failure", "timed_out", "action_required":
			summary.Failing++
		case "", "neutral", "cancelled", "skipped", "stale", "queued", "in_progress", "pending":
			summary.Pending++
		}
	}

	return summary
}

func calculateStatusSummary(events []Event) *StatusSummary {
	summary := &StatusSummary{}
	checkStates := make(map[string]string)

	for _, event := range events {
		if (event.Kind == "status_check" || event.Kind == "check_run") && event.Body != "" {
			key := string(event.Kind) + ":" + event.Body
			checkStates[key] = event.Outcome
		}
	}

	for _, outcome := range checkStates {
		switch outcome {
		case "success":
			summary.Success++
		case "failure", "error", "timed_out", "action_required":
			summary.Failure++
		case "pending", "queued", "in_progress", "waiting":
			summary.Pending++
		case "neutral", "cancelled", "skipped", "stale":
			summary.Neutral++
		}
	}

	return summary
}

func calculateApprovalSummary(events []Event) *ApprovalSummary {
	summary := &ApprovalSummary{}

	// Track the latest review state from each user
	latestReviews := make(map[string]Event)

	for _, event := range events {
		if event.Kind == "review" && event.Outcome != "" {
			latestReviews[event.Actor] = event
		}
	}

	// Check permissions for each reviewer and categorize their reviews
	for _, review := range latestReviews {
		switch review.Outcome {
		case "approved":
			// Use the WriteAccess field that was already populated in the event
			if review.WriteAccess == WriteAccessDefinitely {
				summary.ApprovalsWithWriteAccess++
			} else {
				summary.ApprovalsWithoutWriteAccess++
			}
		case "changes_requested":
			summary.ChangesRequested++
		}
	}

	return summary
}

func filterEvents(events []Event) []Event {
	filtered := make([]Event, 0, len(events))

	for _, event := range events {
		// Include all non-status_check events
		if event.Kind != "status_check" {
			filtered = append(filtered, event)
			continue
		}

		// For status_check events, only include if outcome is failure
		if event.Outcome == "failure" {
			filtered = append(filtered, event)
		}
	}

	return filtered
}

// upgradeWriteAccess scans through events and upgrades write_access from 1 (likely) to 2 (definitely)
// for actors who have performed actions that require write access
func upgradeWriteAccess(events []Event) {
	// Track actors who have definitely demonstrated write access
	confirmedWriteAccess := make(map[string]bool)

	// First pass: identify actors who have performed write-access-requiring actions
	for _, event := range events {
		switch event.Kind {
		case "pr_merged", "labeled", "unlabeled", "assigned", "unassigned", "milestoned", "demilestoned":
			// These actions require write access to the repository
			if event.Actor != "" {
				confirmedWriteAccess[event.Actor] = true
			}
		}
	}

	// Second pass: upgrade write_access from 1 to 2 for confirmed actors
	for i := range events {
		if events[i].WriteAccess == WriteAccessLikely {
			if confirmedWriteAccess[events[i].Actor] {
				events[i].WriteAccess = WriteAccessDefinitely
			}
		}
	}
}
