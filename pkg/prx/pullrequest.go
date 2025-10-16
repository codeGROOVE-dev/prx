package prx

import (
	"time"
)

// TestState represents the overall testing status of a pull request.
const (
	TestStateNone    = ""        // No tests or unknown state
	TestStateQueued  = "queued"  // Tests are queued to run
	TestStateRunning = "running" // Tests are currently executing
	TestStatePassing = "passing" // All tests passed
	TestStateFailing = "failing" // Some tests failed
	TestStatePending = "pending" // Some tests are pending
)

// ReviewState represents the current state of a reviewer's review.
type ReviewState string

// Review state constants.
const (
	ReviewStatePending          ReviewState = "pending"           // Review requested but not yet submitted
	ReviewStateApproved         ReviewState = "approved"          // Approved
	ReviewStateChangesRequested ReviewState = "changes_requested" // Changes requested
	ReviewStateCommented        ReviewState = "commented"         // Reviewed with comments only
)

// PullRequest represents a GitHub pull request with its essential metadata.
//
//nolint:govet // fieldalignment: Struct fields ordered for JSON clarity and API compatibility
type PullRequest struct {
	// 16-byte fields (time.Time)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// 8-byte pointer fields
	ClosedAt        *time.Time       `json:"closed_at,omitempty"`
	MergedAt        *time.Time       `json:"merged_at,omitempty"`
	ApprovalSummary *ApprovalSummary `json:"approval_summary,omitempty"`
	CheckSummary    *CheckSummary    `json:"check_summary,omitempty"`
	Mergeable       *bool            `json:"mergeable"`
	// 24-byte slice/map fields
	Assignees         []string               `json:"assignees"`
	Labels            []string               `json:"labels,omitempty"`
	Reviewers         map[string]ReviewState `json:"reviewers,omitempty"`
	ParticipantAccess map[string]int         `json:"participant_access,omitempty"` // Map of username to WriteAccess level
	// 16-byte string fields
	MergeableState            string `json:"mergeable_state"`
	MergeableStateDescription string `json:"mergeable_state_description,omitempty"`
	Author                    string `json:"author"`
	Body                      string `json:"body"`
	Title                     string `json:"title"`
	MergedBy                  string `json:"merged_by,omitempty"`
	State                     string `json:"state"`
	TestState                 string `json:"test_state,omitempty"`
	HeadSHA                   string `json:"head_sha,omitempty"`
	// 8-byte int fields
	Number            int `json:"number"`
	ChangedFiles      int `json:"changed_files"`
	Deletions         int `json:"deletions"`
	Additions         int `json:"additions"`
	AuthorWriteAccess int `json:"author_write_access,omitempty"`
	// 1-byte bool fields
	AuthorBot bool `json:"author_bot"`
	Merged    bool `json:"merged"`
	Draft     bool `json:"draft"`
}

// CheckSummary aggregates all status checks and check runs.
type CheckSummary struct {
	Success   map[string]string `json:"success"`   // Map of successful check names to their status descriptions
	Failing   map[string]string `json:"failing"`   // Map of failing check names to their status descriptions (excludes cancelled)
	Pending   map[string]string `json:"pending"`   // Map of pending check names to their status descriptions
	Cancelled map[string]string `json:"cancelled"` // Map of cancelled check names to their status descriptions
	Skipped   map[string]string `json:"skipped"`   // Map of skipped check names to their status descriptions
	Stale     map[string]string `json:"stale"`     // Map of stale check names to their status descriptions
	Neutral   map[string]string `json:"neutral"`   // Map of neutral check names to their status descriptions
}

// ApprovalSummary tracks PR review approvals and change requests.
type ApprovalSummary struct {
	// Approvals from users confirmed to have write access (owners, collaborators, members with confirmed access)
	ApprovalsWithWriteAccess int `json:"approvals_with_write_access"`

	// Approvals from users with unknown or likely write access (members, uncertain cases)
	ApprovalsWithUnknownAccess int `json:"approvals_with_unknown_access"`

	// Approvals from users confirmed to not have write access (contributors, outside collaborators)
	ApprovalsWithoutWriteAccess int `json:"approvals_without_write_access"`

	// Outstanding change requests from any reviewer
	ChangesRequested int `json:"changes_requested"`
}

// PullRequestData contains a pull request and all its associated events.
type PullRequestData struct {
	Events      []Event     `json:"events"`
	PullRequest PullRequest `json:"pull_request"`
}

// finalizePullRequest applies final calculations and consistency fixes.
func finalizePullRequest(pullRequest *PullRequest, events []Event, requiredChecks []string, testStateFromAPI string) {
	pullRequest.TestState = testStateFromAPI
	pullRequest.CheckSummary = calculateCheckSummary(events, requiredChecks)
	pullRequest.ApprovalSummary = calculateApprovalSummary(events)
	pullRequest.ParticipantAccess = calculateParticipantAccess(events, pullRequest)

	fixTestState(pullRequest)
	fixMergeable(pullRequest)
	setMergeableDescription(pullRequest)
}

// fixTestState ensures test_state is consistent with check_summary.
func fixTestState(pullRequest *PullRequest) {
	switch {
	case len(pullRequest.CheckSummary.Failing) > 0 || len(pullRequest.CheckSummary.Cancelled) > 0:
		pullRequest.TestState = TestStateFailing
	case len(pullRequest.CheckSummary.Pending) > 0:
		pullRequest.TestState = TestStatePending
	case len(pullRequest.CheckSummary.Success) > 0:
		pullRequest.TestState = TestStatePassing
	default:
		pullRequest.TestState = TestStateNone
	}
}

// fixMergeable ensures mergeable is consistent with mergeable_state.
func fixMergeable(pullRequest *PullRequest) {
	if pullRequest.MergeableState == "blocked" || pullRequest.MergeableState == "dirty" || pullRequest.MergeableState == "unstable" {
		falseVal := false
		pullRequest.Mergeable = &falseVal
	}
}

// setMergeableDescription adds human-readable description for mergeable state.
func setMergeableDescription(pullRequest *PullRequest) {
	switch pullRequest.MergeableState {
	case "blocked":
		setBlockedDescription(pullRequest)
	case "dirty":
		pullRequest.MergeableStateDescription = "PR has merge conflicts that need to be resolved"
	case "unstable":
		pullRequest.MergeableStateDescription = "PR is mergeable but status checks are failing"
	case "clean":
		pullRequest.MergeableStateDescription = "PR is ready to merge"
	case "unknown":
		pullRequest.MergeableStateDescription = "Merge status is being calculated"
	case "draft":
		pullRequest.MergeableStateDescription = "PR is in draft state"
	default:
		pullRequest.MergeableStateDescription = ""
	}
}

// setBlockedDescription determines what's blocking the PR and sets appropriate description.
func setBlockedDescription(pullRequest *PullRequest) {
	hasApprovals := pullRequest.ApprovalSummary.ApprovalsWithWriteAccess > 0
	hasFailingChecks := len(pullRequest.CheckSummary.Failing) > 0 || len(pullRequest.CheckSummary.Cancelled) > 0
	hasPendingChecks := len(pullRequest.CheckSummary.Pending) > 0

	switch {
	case !hasApprovals && !hasFailingChecks:
		if hasPendingChecks {
			pullRequest.MergeableStateDescription = "PR requires approval and has pending status checks"
		} else {
			pullRequest.MergeableStateDescription = "PR requires approval"
		}
	case hasFailingChecks:
		if !hasApprovals {
			pullRequest.MergeableStateDescription = "PR has failing status checks and requires approval"
		} else {
			pullRequest.MergeableStateDescription = "PR is blocked by failing status checks"
		}
	case hasPendingChecks:
		pullRequest.MergeableStateDescription = "PR is blocked by pending status checks"
	default:
		pullRequest.MergeableStateDescription = "PR is blocked by required status checks, reviews, or branch protection rules"
	}
}
