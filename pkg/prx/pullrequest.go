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

// PullRequest represents a GitHub pull request with its essential metadata.
type PullRequest struct {
	CreatedAt                 time.Time        `json:"created_at"`
	UpdatedAt                 time.Time        `json:"updated_at"`
	ClosedAt                  *time.Time       `json:"closed_at,omitempty"`
	Mergeable                 *bool            `json:"mergeable"`
	ApprovalSummary           *ApprovalSummary `json:"approval_summary,omitempty"`
	CheckSummary              *CheckSummary    `json:"check_summary,omitempty"`
	MergedAt                  *time.Time       `json:"merged_at,omitempty"`
	MergeableState            string           `json:"mergeable_state"`
	MergeableStateDescription string           `json:"mergeable_state_description,omitempty"`
	Author                    string           `json:"author"`
	Body                      string           `json:"body"`
	Title                     string           `json:"title"`
	MergedBy                  string           `json:"merged_by,omitempty"`
	State                     string           `json:"state"`
	TestState                 string           `json:"test_state,omitempty"`
	Assignees                 []string         `json:"assignees,omitempty"`
	Labels                    []string         `json:"labels,omitempty"`
	RequestedReviewers        []string         `json:"requested_reviewers,omitempty"`
	AuthorWriteAccess         int              `json:"author_write_access,omitempty"`
	Number                    int              `json:"number"`
	ChangedFiles              int              `json:"changed_files"`
	Deletions                 int              `json:"deletions"`
	Additions                 int              `json:"additions"`
	AuthorBot                 bool             `json:"author_bot"`
	Merged                    bool             `json:"merged"`
	Draft                     bool             `json:"draft"`
	HeadSHA                   string           `json:"head_sha,omitempty"`
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

	// Approvals from users without confirmed write access (contributors, unconfirmed members, etc.)
	ApprovalsWithoutWriteAccess int `json:"approvals_without_write_access"`

	// Outstanding change requests from any reviewer
	ChangesRequested int `json:"changes_requested"`
}

// PullRequestData contains a pull request and all its associated events.
type PullRequestData struct {
	Events      []Event     `json:"events"`
	PullRequest PullRequest `json:"pull_request"`
}
