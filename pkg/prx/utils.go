package prx

import (
	"strings"
)

const (
	// maxTruncateLength is the default truncation length for text fields.
	maxTruncateLength = 256
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

func isHexString(s string) bool {
	for i := range s {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func truncate(s string) string {
	if len(s) <= maxTruncateLength {
		return s
	}
	return s[:maxTruncateLength]
}

// buildPullRequest converts a githubPullRequest to our internal PullRequest struct.
func buildPullRequest(pr *githubPullRequest) PullRequest {
	result := PullRequest{
		Number:         pr.Number,
		Title:          pr.Title,
		Body:           truncate(pr.Body),
		State:          pr.State,
		Draft:          pr.Draft,
		Merged:         pr.Merged,
		Mergeable:      pr.Mergeable,
		MergeableState: pr.MergeableState,
		CreatedAt:      pr.CreatedAt,
		UpdatedAt:      pr.UpdatedAt,
		Additions:      pr.Additions,
		Deletions:      pr.Deletions,
		ChangedFiles:   pr.ChangedFiles,
	}

	if pr.User != nil {
		result.Author = pr.User.Login
		result.AuthorBot = isBot(pr.User)
	} else {
		result.Author = "unknown"
		result.AuthorBot = false
	}

	return result
}

func calculateStatusSummary(events []Event, requiredChecks []string) *StatusSummary {
	summary := &StatusSummary{}
	checkStates := make(map[string]string)

	for _, event := range events {
		if (event.Kind == "status_check" || event.Kind == "check_run") && event.Body != "" {
			key := event.Kind + ":" + event.Body
			checkStates[key] = event.Outcome
		}
	}

	// Track which required checks have been seen
	requiredChecksSeen := make(map[string]bool)

	for key, outcome := range checkStates {
		// Extract check name from key (format: "kind:name")
		parts := strings.SplitN(key, ":", 2)
		if len(parts) == 2 {
			checkName := parts[1]
			// Mark this required check as seen
			for _, required := range requiredChecks {
				if required == checkName {
					requiredChecksSeen[required] = true
					break
				}
			}
		}

		switch outcome {
		case "success":
			summary.Success++
		case "failure", "error", "timed_out":
			summary.Failure++
		case "pending", "queued", "in_progress", "waiting", "action_required":
			summary.Pending++
		case "neutral", "cancelled", "skipped", "stale":
			summary.Neutral++
		default:
			// Unknown outcome, ignore
		}
	}

	// Add missing required checks as pending
	for _, required := range requiredChecks {
		if !requiredChecksSeen[required] {
			summary.Pending++
		}
	}

	return summary
}

// calculateRequiredStatusSummary calculates status summary for only required checks.
func calculateRequiredStatusSummary(events []Event, requiredChecks []string) *StatusSummary {
	summary := &StatusSummary{}
	checkStates := make(map[string]string)

	// Only consider required checks
	requiredSet := make(map[string]bool)
	for _, required := range requiredChecks {
		requiredSet[required] = true
	}

	for _, event := range events {
		if (event.Kind == "status_check" || event.Kind == "check_run") && event.Body != "" {
			// Only include if this is a required check
			if requiredSet[event.Body] {
				key := event.Kind + ":" + event.Body
				checkStates[key] = event.Outcome
			}
		}
	}

	// Track which required checks have been seen
	requiredChecksSeen := make(map[string]bool)

	for key, outcome := range checkStates {
		// Extract check name from key (format: "kind:name")
		parts := strings.SplitN(key, ":", 2)
		if len(parts) == 2 {
			checkName := parts[1]
			requiredChecksSeen[checkName] = true
		}

		switch outcome {
		case "success":
			summary.Success++
		case "failure", "error", "timed_out":
			summary.Failure++
		case "pending", "queued", "in_progress", "waiting", "action_required":
			summary.Pending++
		case "neutral", "cancelled", "skipped", "stale":
			summary.Neutral++
		default:
			// Unknown outcome, ignore
		}
	}

	// Add missing required checks as pending
	for _, required := range requiredChecks {
		if !requiredChecksSeen[required] {
			summary.Pending++
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
		default:
			// Ignore other review states like "commented"
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
// for actors who have performed actions that require write access.
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
		default:
			// Other event types don't require write access
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
