package prx

import (
	"time"
)

// Event kind constants for PR timeline events.
const (
	EventKindCommit        = "commit"         // EventKindCommit represents a commit event.
	EventKindComment       = "comment"        // EventKindComment represents a comment event.
	EventKindReview        = "review"         // EventKindReview represents a review event.
	EventKindReviewComment = "review_comment" // EventKindReviewComment represents a review comment event.

	EventKindLabeled   = "labeled"   // EventKindLabeled represents a label added event.
	EventKindUnlabeled = "unlabeled" // EventKindUnlabeled represents a label removed event.

	EventKindAssigned   = "assigned"   // EventKindAssigned represents an assignment event.
	EventKindUnassigned = "unassigned" // EventKindUnassigned represents an unassignment event.

	EventKindMilestoned   = "milestoned"   // EventKindMilestoned represents a milestone added event.
	EventKindDemilestoned = "demilestoned" // EventKindDemilestoned represents a milestone removed event.

	EventKindReviewRequested      = "review_requested"       // EventKindReviewRequested represents a review request event.
	EventKindReviewRequestRemoved = "review_request_removed" // EventKindReviewRequestRemoved represents a review request removed event.

	EventKindPROpened       = "pr_opened"        // EventKindPROpened represents a PR opened event.
	EventKindPRClosed       = "pr_closed"        // EventKindPRClosed represents a PR closed event.
	EventKindPRMerged       = "pr_merged"        // EventKindPRMerged represents a PR merge event.
	EventKindMerged         = "merged"           // EventKindMerged represents a merge event from timeline.
	EventKindReadyForReview = "ready_for_review" // EventKindReadyForReview represents a ready for review event.
	EventKindConvertToDraft = "convert_to_draft" // EventKindConvertToDraft represents a convert to draft event.
	EventKindClosed         = "closed"           // EventKindClosed represents a PR closed event.
	EventKindReopened       = "reopened"         // EventKindReopened represents a PR reopened event.
	EventKindRenamedTitle   = "renamed_title"    // EventKindRenamedTitle represents a title rename event.

	EventKindMentioned       = "mentioned"        // EventKindMentioned represents a mention event.
	EventKindReferenced      = "referenced"       // EventKindReferenced represents a reference event.
	EventKindCrossReferenced = "cross_referenced" // EventKindCrossReferenced represents a cross-reference event.

	EventKindPinned      = "pinned"      // EventKindPinned represents a pin event.
	EventKindUnpinned    = "unpinned"    // EventKindUnpinned represents an unpin event.
	EventKindTransferred = "transferred" // EventKindTransferred represents a transfer event.

	EventKindSubscribed   = "subscribed"   // EventKindSubscribed represents a subscription event.
	EventKindUnsubscribed = "unsubscribed" // EventKindUnsubscribed represents an unsubscription event.

	EventKindHeadRefDeleted     = "head_ref_deleted"      // EventKindHeadRefDeleted represents a head ref deletion event.
	EventKindHeadRefRestored    = "head_ref_restored"     // EventKindHeadRefRestored represents a head ref restoration event.
	EventKindHeadRefForcePushed = "head_ref_force_pushed" // EventKindHeadRefForcePushed represents a head ref force push event.

	EventKindBaseRefChanged     = "base_ref_changed"      // EventKindBaseRefChanged represents a base ref change event.
	EventKindBaseRefForcePushed = "base_ref_force_pushed" // EventKindBaseRefForcePushed represents a base ref force push event.

	EventKindReviewDismissed = "review_dismissed" // EventKindReviewDismissed represents a review dismissed event.

	EventKindLocked   = "locked"   // EventKindLocked represents a lock event.
	EventKindUnlocked = "unlocked" // EventKindUnlocked represents an unlock event.

	EventKindAutoMergeEnabled      = "auto_merge_enabled"       // EventKindAutoMergeEnabled represents an auto merge enabled event.
	EventKindAutoMergeDisabled     = "auto_merge_disabled"      // EventKindAutoMergeDisabled represents an auto merge disabled event.
	EventKindAddedToMergeQueue     = "added_to_merge_queue"     // EventKindAddedToMergeQueue represents an added to merge queue event.
	EventKindRemovedFromMergeQueue = "removed_from_merge_queue" // EventKindRemovedFromMergeQueue represents removal from merge queue.

	// EventKindAutomaticBaseChangeSucceeded represents a successful base change.
	EventKindAutomaticBaseChangeSucceeded = "automatic_base_change_succeeded"
	// EventKindAutomaticBaseChangeFailed represents a failed base change.
	EventKindAutomaticBaseChangeFailed = "automatic_base_change_failed"

	EventKindDeployed = "deployed" // EventKindDeployed represents a deployment event.
	// EventKindDeploymentEnvironmentChanged represents a deployment environment change event.
	EventKindDeploymentEnvironmentChanged = "deployment_environment_changed"

	EventKindConnected    = "connected"    // EventKindConnected represents a connected event.
	EventKindDisconnected = "disconnected" // EventKindDisconnected represents a disconnected event.
	EventKindUserBlocked  = "user_blocked" // EventKindUserBlocked represents a user blocked event.

	EventKindStatusCheck = "status_check" // EventKindStatusCheck represents a status check event (from APIs).
	EventKindCheckRun    = "check_run"    // EventKindCheckRun represents a check run event (from APIs).
)

// WriteAccess constants for the Event.WriteAccess field.
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
	Timestamp   time.Time `json:"timestamp"`
	Kind        string    `json:"kind"`
	Actor       string    `json:"actor"`
	Target      string    `json:"target,omitempty"`
	Outcome     string    `json:"outcome,omitempty"`
	Body        string    `json:"body,omitempty"`
	Description string    `json:"description,omitempty"`
	WriteAccess int       `json:"write_access,omitempty"`
	Bot         bool      `json:"bot,omitempty"`
	TargetIsBot bool      `json:"target_is_bot,omitempty"`
	Question    bool      `json:"question,omitempty"`
	Required    bool      `json:"required,omitempty"`
	Outdated    bool      `json:"outdated,omitempty"` // For review comments: indicates comment is on outdated code
}
