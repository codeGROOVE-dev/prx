package prx

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
	HeadRefForcePushed   EventKind = "head_ref_force_pushed"
)

// WriteAccess constants for the Event.WriteAccess field
const (
	WriteAccessUnlikely   = 0 // User unlikely to have write access (CONTRIBUTOR, NONE, etc.)
	WriteAccessLikely     = 1 // User likely has write access but unable to confirm (MEMBER with 403 API response)
	WriteAccessDefinitely = 2 // User definitely has write access (OWNER, COLLABORATOR, or confirmed via API)
)

// String returns a human-readable string for the EventKind
func (k EventKind) String() string {
	switch k {
	case Commit:
		return "Commit"
	case Comment:
		return "Comment"
	case Review:
		return "Review"
	case ReviewComment:
		return "Review Comment"
	case StatusCheck:
		return "Status Check"
	case CheckRun:
		return "Check Run"
	case CheckSuite:
		return "Check Suite"
	case PROpened:
		return "PR Opened"
	case PRClosed:
		return "PR Closed"
	case PRMerged:
		return "PR Merged"
	case PRReopened:
		return "PR Reopened"
	case Assigned:
		return "Assigned"
	case Unassigned:
		return "Unassigned"
	case Labeled:
		return "Labeled"
	case Unlabeled:
		return "Unlabeled"
	case Milestoned:
		return "Milestoned"
	case Demilestoned:
		return "Demilestoned"
	case ReviewRequested:
		return "Review Requested"
	case ReviewRequestRemoved:
		return "Review Request Removed"
	case HeadRefForcePushed:
		return "Head Ref Force Pushed"
	default:
		return string(k)
	}
}

// WriteAccessString returns a human-readable string for write access values
func WriteAccessString(access *int) string {
	if access == nil {
		return "N/A"
	}
	switch *access {
	case WriteAccessUnlikely:
		return "Unlikely"
	case WriteAccessLikely:
		return "Likely"
	case WriteAccessDefinitely:
		return "Definitely"
	default:
		return "Unknown"
	}
}

// Event represents a single event that occurred on a pull request.
// Each event captures who did what and when, with additional context depending on the event type.
type Event struct {
	// Kind specifies the type of event (commit, comment, review, etc.)
	Kind EventKind `json:"kind"`
	
	// Timestamp indicates when this event occurred
	Timestamp time.Time `json:"timestamp"`
	
	// Actor is the GitHub username who performed this action
	Actor string `json:"actor"`
	
	// Bot indicates if the actor is an automated bot account
	Bot bool `json:"bot,omitempty"`
	
	// Targets lists users affected by this action
	// - For assigned/unassigned: the assignee username
	// - For review_requested: the reviewer username
	// - For labeled/unlabeled: the label name
	Targets []string `json:"targets,omitempty"`
	
	// Outcome stores the result of the event
	// - For checks: "success", "failure", "pending", "neutral", "cancelled", "skipped", "timed_out", "action_required"
	// - For reviews: "approved", "changes_requested", "commented"
	// - For status checks: "success", "failure", "pending", "error"
	Outcome string `json:"outcome,omitempty"`
	
	// Body contains the main content of the event
	// - For comments/reviews: the text content (truncated to 256 chars)
	// - For commits: the commit message
	// - For check runs/status checks: the check name
	// - For labeled/unlabeled: the label name
	Body string `json:"body,omitempty"`
	
	// Question indicates if this comment/review appears to be asking a question
	Question bool `json:"question,omitempty"`
	
	// WriteAccess indicates the actor's repository permissions
	// - WriteAccessUnlikely (0): User unlikely to have write access
	// - WriteAccessLikely (1): User likely has write access but unable to confirm
	// - WriteAccessDefinitely (2): User definitely has write access
	// - nil: Not applicable for this event type
	WriteAccess *int `json:"write_access,omitempty"`
}
