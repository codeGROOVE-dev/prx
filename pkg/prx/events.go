package prx

import (
	"time"
)

// Event kind constants.
const (
	// Core events.
	EventKindCommit        = "commit"
	EventKindComment       = "comment"
	EventKindReview        = "review"
	EventKindReviewComment = "review_comment"

	// Label events.
	EventKindLabeled   = "labeled"
	EventKindUnlabeled = "unlabeled"

	// Assignment events.
	EventKindAssigned   = "assigned"
	EventKindUnassigned = "unassigned"

	// Milestone events.
	EventKindMilestoned   = "milestoned"
	EventKindDemilestoned = "demilestoned"

	// Review request events.
	EventKindReviewRequested      = "review_requested"
	EventKindReviewRequestRemoved = "review_request_removed"

	// PR state events.
	EventKindPRMerged       = "pr_merged"
	EventKindReadyForReview = "ready_for_review"
	EventKindConvertToDraft = "convert_to_draft"
	EventKindClosed         = "closed"
	EventKindReopened       = "reopened"

	// Reference events.
	EventKindMentioned       = "mentioned"
	EventKindReferenced      = "referenced"
	EventKindCrossReferenced = "cross-referenced"

	// Project events.
	EventKindAddedToProject        = "added_to_project"
	EventKindMovedColumnsInProject = "moved_columns_in_project"
	EventKindRemovedFromProject    = "removed_from_project"
	EventKindConvertedNoteToIssue  = "converted_note_to_issue"

	// Pin events.
	EventKindPinned   = "pinned"
	EventKindUnpinned = "unpinned"

	// Transfer events.
	EventKindTransferred = "transferred"

	// Subscription events.
	EventKindSubscribed   = "subscribed"
	EventKindUnsubscribed = "unsubscribed"

	// Rename events.
	EventKindRenamed = "renamed"

	// Head ref events.
	EventKindHeadRefDeleted     = "head_ref_deleted"
	EventKindHeadRefRestored    = "head_ref_restored"
	EventKindHeadRefForcePushed = "head_ref_force_pushed"

	// Base ref events.
	EventKindBaseRefChanged     = "base_ref_changed"
	EventKindBaseRefForcePushed = "base_ref_force_pushed"

	// Review events.
	EventKindReviewDismissed = "review_dismissed"

	// Duplicate events.
	EventKindMarkedAsDuplicate   = "marked_as_duplicate"
	EventKindUnmarkedAsDuplicate = "unmarked_as_duplicate"

	// Lock events.
	EventKindLocked   = "locked"
	EventKindUnlocked = "unlocked"

	// Auto merge events.
	EventKindAutoMergeEnabled  = "auto_merge_enabled"
	EventKindAutoMergeDisabled = "auto_merge_disabled"

	// Deploy events.
	EventKindDeploymentEnvironmentChanged = "deployment_environment_changed"

	// Connected/Disconnected events.
	EventKindConnected    = "connected"
	EventKindDisconnected = "disconnected"

	// Comment events.
	EventKindCommentDeleted = "comment_deleted"

	// Check/Status events (not from timeline but from other APIs).
	EventKindStatusCheck = "status_check"
	EventKindCheckRun    = "check_run"
)

// WriteAccess constants for the Event.WriteAccess field.
const (
	WriteAccessNo         = -2 // User confirmed to not have write access
	WriteAccessUnlikely   = -1 // User unlikely to have write access (CONTRIBUTOR, NONE, etc.)
	WriteAccessNA         = 0  // Not applicable/not set (omitted from JSON)
	WriteAccessLikely     = 1  // User likely has write access but unable to confirm (MEMBER with 403 API response)
	WriteAccessDefinitely = 2  // User definitely has write access (OWNER, COLLABORATOR, or confirmed via API)
)

// fetchResult represents the result of a concurrent fetch operation.
type fetchResult struct {
	err       error
	name      string
	testState string
	events    []Event
}

// Event represents a single event that occurred on a pull request.
// Each event captures who did what and when, with additional context depending on the event type.
type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	Kind        string    `json:"kind"`
	Actor       string    `json:"actor"`
	Target      string    `json:"target,omitempty"`
	Outcome     string    `json:"outcome,omitempty"`
	Body        string    `json:"body,omitempty"`
	WriteAccess int       `json:"write_access,omitempty"`
	Bot         bool      `json:"bot,omitempty"`
	TargetIsBot bool      `json:"target_is_bot,omitempty"`
	Question    bool      `json:"question,omitempty"`
}

// createEvent is a helper function to create an Event with common fields.
func createEvent(kind string, timestamp time.Time, user *githubUser, body string) Event {
	body = truncate(body)
	event := Event{
		Kind:      kind,
		Timestamp: timestamp,
		Body:      body,
		Question:  containsQuestion(body),
	}
	if user != nil {
		event.Actor = user.Login
		event.Bot = isBot(user)
	}
	return event
}
