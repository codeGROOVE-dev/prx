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
	questionRegexCache = make(map[string]*regexp.Regexp, len(questionPatterns))
	for _, p := range questionPatterns {
		// Split pattern into words and add word boundaries
		w := strings.Fields(p)
		if len(w) == 0 {
			continue // Skip empty patterns (defensive programming)
		}
		for i, word := range w {
			w[i] = regexp.QuoteMeta(word)
		}
		// Add word boundaries: \b at start of first word, \b at end of last word
		w[0] = "\\b" + w[0]
		w[len(w)-1] = w[len(w)-1] + "\\b"
		// Join with flexible whitespace and compile as case-insensitive
		questionRegexCache[p] = regexp.MustCompile("(?i)" + strings.Join(w, "\\s+"))
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
	for _, re := range questionRegexCache {
		if re.MatchString(text) {
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
	for _, e := range events {
		if (e.Kind == "status_check" || e.Kind == "check_run") && e.Body != "" {
			latestChecks[e.Body] = checkInfo{
				outcome:     e.Outcome,
				description: e.Description,
			}
		}
	}

	// Count checks and collect status descriptions
	seen := make(map[string]bool)
	for name, info := range latestChecks {
		// Track required checks we've seen
		for _, req := range requiredChecks {
			if req == name {
				seen[req] = true
				break
			}
		}

		// Count and categorize the check
		switch info.outcome {
		case "success":
			summary.Success++
		case "failure", "error", "timed_out", "action_required":
			summary.Failure++
			summary.FailingStatuses[name] = info.description
		case "pending", "queued", "in_progress", "waiting":
			summary.Pending++
			summary.PendingStatuses[name] = info.description
		case "neutral", "cancelled", "skipped", "stale":
			summary.Neutral++
		default:
			// Unknown outcome, ignore
		}
	}

	// Add missing required checks as pending
	for _, req := range requiredChecks {
		if !seen[req] {
			summary.Pending++
			summary.PendingStatuses[req] = "Expected — Waiting for status to be reported"
		}
	}

	return summary
}

func calculateApprovalSummary(events []Event) *ApprovalSummary {
	summary := &ApprovalSummary{}

	// Track the latest review state from each user
	latestReviews := make(map[string]Event)

	for _, e := range events {
		if e.Kind == "review" && e.Outcome != "" {
			latestReviews[e.Actor] = e
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

	for _, e := range events {
		// Include all non-status_check events
		if e.Kind != "status_check" {
			filtered = append(filtered, e)
			continue
		}

		// For status_check events, only include if outcome is failure
		if e.Outcome == "failure" {
			filtered = append(filtered, e)
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
	for _, e := range events {
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
