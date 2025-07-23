package prevents

import (
	"time"
)

// EventKind represents the type of event that occurred on a pull request.
type EventKind string

// Event types for all possible pull request events.
const (
	Commit               EventKind = "commit"
	Comment              EventKind = "comment"
	Review               EventKind = "review"
	ReviewComment        EventKind = "review_comment"
	StatusCheck          EventKind = "status_check"
	CheckRun             EventKind = "check_run"
	CheckSuite           EventKind = "check_suite"
	PROpened             EventKind = "pr_opened"
	PRClosed             EventKind = "pr_closed"
	PRMerged             EventKind = "pr_merged"
	PRReopened           EventKind = "pr_reopened"
	Assigned             EventKind = "assigned"
	Unassigned           EventKind = "unassigned"
	Labeled              EventKind = "labeled"
	Unlabeled            EventKind = "unlabeled"
	Milestoned           EventKind = "milestoned"
	Demilestoned         EventKind = "demilestoned"
	ReviewRequested      EventKind = "review_requested"
	ReviewRequestRemoved EventKind = "review_request_removed"
)

// Event represents a single event that occurred on a pull request.
type Event struct {
	Kind      EventKind `json:"kind"`
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Bot       bool      `json:"bot,omitempty"`      // True if the actor is a bot
	Targets   []string  `json:"targets,omitempty"`  // Users affected by the action (assignees, reviewers, etc.)
	Outcome   string    `json:"outcome,omitempty"`  // For checks: "success", "failure", "pending", etc. For reviews: "approved", "changes_requested", "commented"
	Body      string    `json:"body,omitempty"`     // For comments and reviews
	Question  bool      `json:"question,omitempty"` // True if the comment/review contains a question
}
