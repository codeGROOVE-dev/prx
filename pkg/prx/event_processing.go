package prx

// filterEvents removes non-essential events to reduce noise.
// Currently filters out successful status_check events (keeps failures).
func filterEvents(events []Event) []Event {
	filtered := make([]Event, 0, len(events))

	for i := range events {
		e := &events[i]
		// Include all non-status_check events
		if e.Kind != "status_check" {
			filtered = append(filtered, *e)
			continue
		}

		// For status_check events, only include if outcome is failure
		if e.Outcome == "failure" {
			filtered = append(filtered, *e)
		}
	}

	return filtered
}

// upgradeWriteAccess scans through events and upgrades write_access from 1 (likely) to 2 (definitely)
// for actors who have performed actions that require write access.
func upgradeWriteAccess(events []Event) {
	// Track actors who have definitely demonstrated write access
	confirmed := make(map[string]bool)

	// First pass: identify actors who have performed write-access-requiring actions
	for i := range events {
		e := &events[i]
		switch e.Kind {
		case "pr_merged", "labeled", "unlabeled", "assigned", "unassigned", "milestoned", "demilestoned":
			// These actions require write access to the repository
			if e.Actor != "" {
				confirmed[e.Actor] = true
			}
		default:
			// Other event types don't require write access
		}
	}

	// Second pass: upgrade write_access from 1 to 2 for confirmed actors
	for i := range events {
		if events[i].WriteAccess == WriteAccessLikely {
			if confirmed[events[i].Actor] {
				events[i].WriteAccess = WriteAccessDefinitely
			}
		}
	}
}

// calculateCheckSummary analyzes check/status events and categorizes them by outcome.
func calculateCheckSummary(events []Event, requiredChecks []string) *CheckSummary {
	summary := &CheckSummary{
		Success:   make(map[string]string),
		Failing:   make(map[string]string),
		Pending:   make(map[string]string),
		Cancelled: make(map[string]string),
		Skipped:   make(map[string]string),
		Stale:     make(map[string]string),
		Neutral:   make(map[string]string),
	}

	// Track latest state for each check (deduplicates multiple runs of same check)
	type checkInfo struct {
		outcome     string
		description string
	}
	latestChecks := make(map[string]checkInfo)

	// Collect latest state for each check
	for i := range events {
		e := &events[i]
		if (e.Kind == "status_check" || e.Kind == "check_run") && e.Body != "" {
			latestChecks[e.Body] = checkInfo{
				outcome:     e.Outcome,
				description: e.Description,
			}
		}
	}

	// Collect checks and categorize them
	seen := make(map[string]bool)
	for name, info := range latestChecks {
		// Track required checks we've seen
		for _, req := range requiredChecks {
			if req == name {
				seen[req] = true
				break
			}
		}

		// Categorize the check (each check goes into exactly one category)
		switch info.outcome {
		case "success":
			summary.Success[name] = info.description
		case "failure", "error", "timed_out", "action_required":
			summary.Failing[name] = info.description
		case "cancelled":
			summary.Cancelled[name] = info.description
		case "pending", "queued", "in_progress", "waiting":
			summary.Pending[name] = info.description
		case "skipped":
			summary.Skipped[name] = info.description
		case "stale":
			summary.Stale[name] = info.description
		case "neutral":
			summary.Neutral[name] = info.description
		default:
			// Unknown outcome, ignore
		}
	}

	// Add missing required checks as pending
	for _, req := range requiredChecks {
		if !seen[req] {
			summary.Pending[req] = "Expected — Waiting for status to be reported"
		}
	}

	return summary
}

// calculateApprovalSummary analyzes review events and categorizes approvals by reviewer's write access.
func calculateApprovalSummary(events []Event) *ApprovalSummary {
	summary := &ApprovalSummary{}

	// Track the latest review state from each user
	latestReviews := make(map[string]Event)

	for i := range events {
		e := &events[i]
		if e.Kind == "review" && e.Outcome != "" {
			latestReviews[e.Actor] = *e
		}
	}

	// Check permissions for each reviewer and categorize their reviews
	for actor := range latestReviews {
		review := latestReviews[actor]
		switch review.Outcome {
		case "approved":
			// Use the WriteAccess field that was already populated in the event
			switch review.WriteAccess {
			case WriteAccessDefinitely:
				// Confirmed write access (OWNER, COLLABORATOR, or verified MEMBER)
				summary.ApprovalsWithWriteAccess++
			case WriteAccessLikely, WriteAccessNA, WriteAccessUnlikely:
				// Unknown/uncertain write access (unverified MEMBER, CONTRIBUTOR with unknown status, NA)
				summary.ApprovalsWithUnknownAccess++
			case WriteAccessNo:
				// Confirmed no write access (explicitly denied)
				summary.ApprovalsWithoutWriteAccess++
			default:
				// Fallback for any unexpected values - treat as unknown
				summary.ApprovalsWithUnknownAccess++
			}
		case "changes_requested":
			summary.ChangesRequested++
		default:
			// Ignore other review states like "commented"
		}
	}

	return summary
}

// calculateParticipantAccess builds a map of all PR participants to their write access levels.
// Includes the PR author, assignees, reviewers, and all event actors.
func calculateParticipantAccess(events []Event, pr *PullRequest) map[string]int {
	participants := make(map[string]int)

	// Add the PR author
	if pr.Author != "" {
		participants[pr.Author] = pr.AuthorWriteAccess
	}

	// Add assignees (write access unknown)
	for _, assignee := range pr.Assignees {
		if assignee != "" {
			if _, exists := participants[assignee]; !exists {
				participants[assignee] = WriteAccessNA
			}
		}
	}

	// Add reviewers (write access unknown at this point)
	for reviewer := range pr.Reviewers {
		if reviewer != "" {
			if _, exists := participants[reviewer]; !exists {
				participants[reviewer] = WriteAccessNA
			}
		}
	}

	// Collect all unique actors from events and upgrade write access where known
	for i := range events {
		e := &events[i]
		if e.Actor != "" {
			// Keep the highest write access level if we see the same actor multiple times
			if existing, ok := participants[e.Actor]; !ok {
				// New participant
				participants[e.Actor] = e.WriteAccess
			} else if e.WriteAccess > existing {
				// Upgrade to higher write access level
				participants[e.Actor] = e.WriteAccess
			}
		}
	}

	return participants
}
