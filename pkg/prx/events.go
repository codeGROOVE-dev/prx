package prx

import (
	"time"
)

// WriteAccess constants for the Event.WriteAccess field
const (
	WriteAccessNo         = -2 // User confirmed to not have write access
	WriteAccessUnlikely   = -1 // User unlikely to have write access (CONTRIBUTOR, NONE, etc.)
	WriteAccessNA         = 0  // Not applicable/not set (omitted from JSON)
	WriteAccessLikely     = 1  // User likely has write access but unable to confirm (MEMBER with 403 API response)
	WriteAccessDefinitely = 2  // User definitely has write access (OWNER, COLLABORATOR, or confirmed via API)
)

// Event represents a single event that occurred on a pull request.
// Each event captures who did what and when, with additional context depending on the event type.
type Event struct {
	// Kind specifies the type of event (commit, comment, review, etc.)
	Kind string `json:"kind"`

	// Timestamp indicates when this event occurred
	Timestamp time.Time `json:"timestamp"`

	// Actor is the GitHub username who performed this action
	Actor string `json:"actor"`

	// Bot indicates if the actor is an automated bot account
	Bot bool `json:"bot,omitempty"`

	// Target is the user or entity affected by this action
	// - For assigned/unassigned: the assignee username
	// - For review_requested: the reviewer username or team name
	// - For labeled/unlabeled: the label name
	// - For milestoned/demilestoned: the milestone name
	Target string `json:"target,omitempty"`

	// TargetIsBot indicates if the target is an automated bot account
	// Only relevant when target is a user (not for labels, milestones, or teams)
	TargetIsBot bool `json:"target_is_bot,omitempty"`

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
	// - WriteAccessNo (-2): User confirmed to not have write access
	// - WriteAccessUnlikely (-1): User unlikely to have write access
	// - WriteAccessNA (0): Not applicable (omitted from JSON)
	// - WriteAccessLikely (1): User likely has write access but unable to confirm
	// - WriteAccessDefinitely (2): User definitely has write access
	WriteAccess int `json:"write_access,omitempty"`
}
