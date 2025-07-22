package prevents

import (
	"time"
)

// EventType represents the type of event that occurred on a pull request.
type EventType string

// Event types for all possible pull request events.
const (
	EventTypeCommit               EventType = "commit"
	EventTypeComment              EventType = "comment"
	EventTypeReview               EventType = "review"
	EventTypeReviewComment        EventType = "review_comment"
	EventTypeStatusCheck          EventType = "status_check"
	EventTypeCheckRun             EventType = "check_run"
	EventTypeCheckSuite           EventType = "check_suite"
	EventTypePROpened             EventType = "pr_opened"
	EventTypePRClosed             EventType = "pr_closed"
	EventTypePRMerged             EventType = "pr_merged"
	EventTypePRReopened           EventType = "pr_reopened"
	EventTypeAssigned             EventType = "assigned"
	EventTypeUnassigned           EventType = "unassigned"
	EventTypeLabeled              EventType = "labeled"
	EventTypeUnlabeled            EventType = "unlabeled"
	EventTypeMilestoned           EventType = "milestoned"
	EventTypeDemilestoned         EventType = "demilestoned"
	EventTypeReviewRequested      EventType = "review_requested"
	EventTypeReviewRequestRemoved EventType = "review_request_removed"
)

// Event represents a single event that occurred on a pull request.
type Event struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Bot       bool      `json:"bot,omitempty"`     // True if the actor is a bot
	Targets   []string  `json:"targets,omitempty"` // Users affected by the action (assignees, reviewers, etc.)
	Outcome   string    `json:"outcome,omitempty"` // For checks: "success", "failure", "pending", etc.
	Body      string    `json:"body,omitempty"`    // For comments and reviews
}