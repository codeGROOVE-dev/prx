package prx

import (
	"regexp"
	"strings"
	"sync"
)

const (
	// maxTruncateLength is the default truncation length for text fields.
	maxTruncateLength = 256
)

// questionPatterns defines phrases that typically indicate a question is being asked.
// Each pattern will be compiled into a regex with word boundaries to avoid false positives.
var questionPatterns = []string{
	// Direct questions
	"how can",
	"how do",
	"how would",
	"how should",
	"how about",
	"how to",
	// Modal questions
	"should i",
	"should we",
	"should this",
	"can i",
	"can we",
	"can you",
	"can someone",
	"can anyone",
	"could i",
	"could we",
	"could you",
	"could someone",
	"could anyone",
	"would you",
	"would someone",
	"would anyone",
	"will you",
	"will this",
	"may i",
	// What questions
	"what do you think",
	"what's the",
	"what is the",
	"what are",
	"what about",
	// Request patterns
	"any suggestions",
	"any ideas",
	"any thoughts",
	"any feedback",
	"anyone know",
	"anyone else",
	"someone know",
	"does anyone",
	"does someone",
	"does this",
	"do we",
	"do you",
	// Possibility questions
	"is it possible",
	"is there a way",
	"is this",
	"is that",
	"are there",
	"are we",
	// Indirect questions
	"wondering if",
	"thoughts on",
	"advice on",
	"help with",
	"need help",
	// Why/when/where/who questions
	"why is",
	"why does",
	"when should",
	"when can",
	"when will",
	"where should",
	"where can",
	"where is",
	"which one",
	"which is",
	"who can",
	"who is",
	"who knows",
	// Have/has questions
	"have you",
	"has anyone",
	"has someone",
}

// questionRegexCache caches compiled regexes for performance.
var (
	questionRegexCache map[string]*regexp.Regexp
	questionRegexOnce  sync.Once
)

// initQuestionRegexes compiles all question patterns into regexes with word boundaries.
// This is done once on first use to avoid repeated compilation.
func initQuestionRegexes() {
	questionRegexCache = make(map[string]*regexp.Regexp)
	for _, pattern := range questionPatterns {
		// Create regex with word boundaries
		// Use \b for word boundaries at the start and end of the pattern
		// Split the pattern into words and ensure each word has proper boundaries
		words := strings.Fields(pattern)
		regexParts := make([]string, len(words))
		for i, word := range words {
			// Escape any regex special characters in the word
			escapedWord := regexp.QuoteMeta(word)
			if i == 0 {
				// First word needs boundary at start
				regexParts[i] = "\\b" + escapedWord
			} else if i == len(words)-1 {
				// Last word needs boundary at end
				regexParts[i] = escapedWord + "\\b"
			} else {
				// Middle words just need to match
				regexParts[i] = escapedWord
			}
		}
		// Join with flexible whitespace matching (one or more spaces)
		regexStr := strings.Join(regexParts, "\\s+")
		// Compile with case-insensitive flag
		questionRegexCache[pattern] = regexp.MustCompile("(?i)" + regexStr)
	}
}

// containsQuestion determines if text contains a question based on:
// 1. Presence of a question mark
// 2. Common question patterns with proper word boundaries.
func containsQuestion(text string) bool {
	// Quick check for question mark
	if strings.Contains(text, "?") {
		return true
	}

	// Return false for empty or very short text
	if len(text) < 3 {
		return false
	}

	// Initialize regex cache once
	questionRegexOnce.Do(initQuestionRegexes)

	// Check against compiled patterns
	for _, regex := range questionRegexCache {
		if regex.MatchString(text) {
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
		HeadSHA:        pr.Head.SHA,
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

func calculateCheckSummary(events []Event, requiredChecks []string) *CheckSummary {
	summary := &CheckSummary{
		FailingStatuses: make(map[string]string),
		PendingStatuses: make(map[string]string),
	}

	// Track latest state for each check (deduplicates multiple runs of same check)
	type checkInfo struct {
		outcome     string
		description string
	}
	latestChecks := make(map[string]checkInfo)

	// Collect latest state for each check
	for _, event := range events {
		if (event.Kind == "status_check" || event.Kind == "check_run") && event.Body != "" {
			latestChecks[event.Body] = checkInfo{
				outcome:     event.Outcome,
				description: event.Description,
			}
		}
	}

	// Count checks and collect status descriptions
	seenRequired := make(map[string]bool)
	for checkName, info := range latestChecks {
		// Track required checks we've seen
		for _, required := range requiredChecks {
			if required == checkName {
				seenRequired[required] = true
				break
			}
		}

		// Count and categorize the check
		switch info.outcome {
		case "success":
			summary.Success++
		case "failure", "error", "timed_out", "action_required":
			summary.Failure++
			summary.FailingStatuses[checkName] = info.description
		case "pending", "queued", "in_progress", "waiting":
			summary.Pending++
			summary.PendingStatuses[checkName] = info.description
		case "neutral", "cancelled", "skipped", "stale":
			summary.Neutral++
		default:
			// Unknown outcome, ignore
		}
	}

	// Add missing required checks as pending
	for _, required := range requiredChecks {
		if !seenRequired[required] {
			summary.Pending++
			summary.PendingStatuses[required] = "Expected — Waiting for status to be reported"
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
